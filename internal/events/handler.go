package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/matangi/eventpulse/internal/api"
	"github.com/matangi/eventpulse/internal/auth"
	"github.com/matangi/eventpulse/internal/telemetry"
)

type Handler struct {
	pub Publisher
}

func NewHandler(pub Publisher) *Handler {
	return &Handler{pub: pub}
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

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid JSON payload")
		return
	}

	now := time.Now().UTC()
	e := buildEvent(req, projectID, now)

	if errs := e.Validate(); len(errs) > 0 {
		api.WriteValidationError(w, toFieldErrors(errs))
		return
	}

	if err := h.pub.Publish(r.Context(), e); err != nil {
		telemetry.EventsIngestedTotal.WithLabelValues("error").Inc()
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
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

	var req batchIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
