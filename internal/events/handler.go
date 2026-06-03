package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matanate/eventpulse/internal/api"
	"github.com/matanate/eventpulse/internal/auth"
	"github.com/matanate/eventpulse/internal/telemetry"
)

// SchemaValidator validates event properties against a registered JSON Schema.
// It mirrors schemas.Validator to avoid an import cycle.
type SchemaValidator interface {
	Validate(ctx context.Context, projectID, eventName string, properties map[string]any) (violations []string, enforce bool, err error)
}

// Broadcaster is an optional hook for broadcasting events to real-time subscribers
// (e.g. SSE connections) after successful ingestion.
type Broadcaster interface {
	Broadcast(ctx context.Context, channel, payload string)
}

type Handler struct {
	pub             Publisher
	pool            *pgxpool.Pool    // optional: direct DB write for immediate feed visibility
	broadcaster     Broadcaster      // optional: SSE pub/sub after ingest
	schemaValidator SchemaValidator  // optional: JSON Schema validation on properties
}

// NewHandler creates an event handler. Pass a non-nil pool to enable direct
// Postgres writes alongside queue publishing (makes events appear immediately
// in analytics queries without waiting for the worker).
func NewHandler(pub Publisher, pool *pgxpool.Pool) *Handler {
	return &Handler{pub: pub, pool: pool}
}

// WithBroadcaster attaches a real-time broadcaster (SSE pub/sub).
func (h *Handler) WithBroadcaster(b Broadcaster) *Handler {
	h.broadcaster = b
	return h
}

// WithSchemaValidator attaches a JSON Schema validator. Properties are checked
// against any registered schema after structural validation passes.
func (h *Handler) WithSchemaValidator(v SchemaValidator) *Handler {
	h.schemaValidator = v
	return h
}

// sseEvent is the minimal JSON shape broadcast to SSE subscribers.
// It mirrors the EventRow TypeScript interface in the frontend.
type sseEvent struct {
	ID         string         `json:"id"`
	Event      string         `json:"event"`
	UserID     string         `json:"user_id,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  string         `json:"timestamp"`
	ReceivedAt string         `json:"received_at"`
}

type ingestRequest struct {
	Event          string         `json:"event"`
	UserID         string         `json:"user_id"`
	Properties     map[string]any `json:"properties"`
	IdempotencyKey string         `json:"idempotency_key"`
	Timestamp      *time.Time     `json:"timestamp"`
}

type ingestResponse struct {
	Status string `json:"status"`
}

type batchIngestRequest struct {
	Events []ingestRequest `json:"events"`
}

type batchIngestResponse struct {
	Count  int    `json:"count"`
	Status string `json:"status"`
}

func (h *Handler) HandleIngest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			api.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body must not exceed 1 MiB")
			return
		}
		api.WriteError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid JSON payload")
		return
	}

	now := time.Now().UTC()
	e := buildEvent(req, projectID, now)

	// Header takes precedence over body field.
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		e.IdempotencyKey = key
	}

	if errs := e.Validate(); len(errs) > 0 {
		api.WriteValidationError(w, toFieldErrors(errs))
		return
	}

	if rejected := h.checkSchema(w, r, projectID, e); rejected {
		return
	}

	if err := h.pub.Publish(r.Context(), e); err != nil {
		telemetry.EventsIngestedTotal.WithLabelValues("error").Inc()
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	// Direct write for immediate feed visibility; non-fatal if it fails.
	// ON CONFLICT DO NOTHING in Store() ensures no duplicates when the worker later processes the queue entry.
	if h.pool != nil {
		if err := Store(r.Context(), h.pool, e); err != nil {
			slog.Warn("handler: direct store failed", "err", err, "event_id", e.ID)
		}
	}

	h.broadcastEvent(r.Context(), e)

	telemetry.EventsIngestedTotal.WithLabelValues("success").Inc()
	api.WriteJSON(w, http.StatusAccepted, ingestResponse{Status: "queued"})
}

func (h *Handler) HandleBatchIngest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req batchIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			api.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body must not exceed 1 MiB")
			return
		}
		api.WriteError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid JSON payload")
		return
	}

	if len(req.Events) == 0 {
		api.WriteError(w, http.StatusBadRequest, "BATCH_EMPTY", "batch must contain at least one event")
		return
	}
	if len(req.Events) > 100 {
		api.WriteError(w, http.StatusBadRequest, "BATCH_TOO_LARGE", "batch must not exceed 100 events")
		return
	}

	now := time.Now().UTC()
	evts := make([]*Event, len(req.Events))
	var fieldErrs []api.FieldError

	for i, item := range req.Events {
		e := buildEvent(item, projectID, now)
		evts[i] = e
		for _, ve := range e.Validate() {
			fieldErrs = append(fieldErrs, api.FieldError{
				Field:   fmt.Sprintf("events[%d].%s", i, ve.Field),
				Message: ve.Message,
			})
		}
	}

	if len(fieldErrs) > 0 {
		api.WriteValidationError(w, fieldErrs)
		return
	}

	for _, e := range evts {
		if rejected := h.checkSchema(w, r, projectID, e); rejected {
			return
		}
	}

	if err := h.pub.PublishBatch(r.Context(), evts); err != nil {
		telemetry.EventsIngestedTotal.WithLabelValues("error").Add(float64(len(evts)))
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	// Direct write for immediate feed visibility; non-fatal if it fails.
	if h.pool != nil {
		if err := BatchStore(r.Context(), h.pool, evts); err != nil {
			slog.Warn("handler: direct batch store failed", "err", err, "count", len(evts))
		}
	}

	for _, e := range evts {
		h.broadcastEvent(r.Context(), e)
	}

	telemetry.EventsIngestedTotal.WithLabelValues("success").Add(float64(len(evts)))
	api.WriteJSON(w, http.StatusAccepted, batchIngestResponse{Count: len(evts), Status: "queued"})
}

func buildEvent(req ingestRequest, projectID string, now time.Time) *Event {
	e := &Event{
		ID:             uuid.New().String(),
		ProjectID:      projectID,
		Event:          strings.TrimSpace(req.Event),
		UserID:         req.UserID,
		Properties:     req.Properties,
		IdempotencyKey: req.IdempotencyKey,
		ReceivedAt:     now,
		Timestamp:      now,
	}
	if req.Timestamp != nil {
		e.Timestamp = req.Timestamp.UTC()
	}
	return e
}

func toFieldErrors(errs []ValidationError) []api.FieldError {
	out := make([]api.FieldError, len(errs))
	for i, ve := range errs {
		out[i] = api.FieldError{Field: ve.Field, Message: ve.Message}
	}
	return out
}

// checkSchema validates e.Properties against any registered JSON Schema.
// Returns true if the request was rejected (response already written).
// Warn-mode violations are logged and metered but do not reject.
func (h *Handler) checkSchema(w http.ResponseWriter, r *http.Request, projectID string, e *Event) (rejected bool) {
	if h.schemaValidator == nil {
		return false
	}
	violations, enforce, err := h.schemaValidator.Validate(r.Context(), projectID, e.Event, e.Properties)
	if err != nil {
		slog.Warn("schema validator error", "err", err, "event", e.Event)
		return false
	}
	if len(violations) == 0 {
		return false
	}
	if enforce {
		telemetry.SchemaViolationsTotal.WithLabelValues(projectID, e.Event, "enforce").Inc()
		details := make([]api.FieldError, len(violations))
		for i, v := range violations {
			details[i] = api.FieldError{Field: "properties", Message: v}
		}
		api.WriteJSON(w, http.StatusUnprocessableEntity, api.ErrorResponse{
			Error:   "event properties failed schema validation",
			Code:    "SCHEMA_VIOLATION",
			Details: details,
		})
		return true
	}
	telemetry.SchemaViolationsTotal.WithLabelValues(projectID, e.Event, "warn").Inc()
	slog.Warn("schema violation (warn mode)", "event", e.Event, "violations", violations)
	return false
}

func (h *Handler) broadcastEvent(ctx context.Context, e *Event) {
	if h.broadcaster == nil {
		return
	}
	evt := sseEvent{
		ID:         e.ID,
		Event:      e.Event,
		UserID:     e.UserID,
		Properties: e.Properties,
		Timestamp:  e.Timestamp.Format(time.RFC3339),
		ReceivedAt: e.ReceivedAt.Format(time.RFC3339),
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	h.broadcaster.Broadcast(ctx, "events:"+e.ProjectID, string(payload))
}
