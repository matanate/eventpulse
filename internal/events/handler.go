package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matangi/eventpulse/internal/api"
	"github.com/matangi/eventpulse/internal/auth"
	"github.com/matangi/eventpulse/internal/telemetry"
)

type Handler struct {
	pub  Publisher
	pool *pgxpool.Pool // optional: if set, events are written directly to DB for immediate feed visibility
}

// NewHandler creates an event handler. Pass a non-nil pool to enable direct
// Postgres writes alongside queue publishing (makes events appear immediately
// in analytics queries without waiting for the worker).
func NewHandler(pub Publisher, pool *pgxpool.Pool) *Handler {
	return &Handler{pub: pub, pool: pool}
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
