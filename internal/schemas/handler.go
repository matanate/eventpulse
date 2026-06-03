package schemas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/matanate/eventpulse/internal/api"
	"github.com/matanate/eventpulse/internal/auth"
)

// Handler provides HTTP endpoints for managing event schemas.
type Handler struct {
	store     *Store
	validator *SchemaValidator
}

// NewHandler creates a Handler backed by the given store and validator.
func NewHandler(store *Store, validator *SchemaValidator) *Handler {
	return &Handler{store: store, validator: validator}
}

type upsertRequest struct {
	Schema json.RawMessage `json:"schema"`
	Mode   Mode            `json:"mode"`
}

type schemaResponse struct {
	ID        string          `json:"id"`
	EventName string          `json:"event_name"`
	Schema    json.RawMessage `json:"schema"`
	Mode      Mode            `json:"mode"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func toResponse(sc *Schema) schemaResponse {
	return schemaResponse{
		ID:        sc.ID,
		EventName: sc.EventName,
		Schema:    sc.Schema,
		Mode:      sc.Mode,
		CreatedAt: sc.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: sc.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// HandleUpsert handles POST /v1/projects/{projectID}/schemas/{event}.
func (h *Handler) HandleUpsert(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}
	eventName := chi.URLParam(r, "event")
	if eventName == "" || len(eventName) > 255 {
		api.WriteError(w, http.StatusBadRequest, "INVALID_EVENT_NAME", "event name must be between 1 and 255 characters")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var req upsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid JSON payload")
		return
	}

	if len(req.Schema) == 0 {
		api.WriteError(w, http.StatusBadRequest, "MISSING_SCHEMA", "schema is required")
		return
	}
	if req.Mode == "" {
		req.Mode = ModeWarn
	}
	if req.Mode != ModeEnforce && req.Mode != ModeWarn {
		api.WriteError(w, http.StatusBadRequest, "INVALID_MODE", "mode must be 'enforce' or 'warn'")
		return
	}

	// Guard against pathological schemas ($ref cycles) that could block indefinitely.
	compileCtx, compileCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer compileCancel()
	compileCh := make(chan error, 1)
	go func() { compileCh <- Compile(req.Schema) }()
	select {
	case err := <-compileCh:
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "INVALID_SCHEMA", err.Error())
			return
		}
	case <-compileCtx.Done():
		api.WriteError(w, http.StatusRequestTimeout, "SCHEMA_TIMEOUT", "schema compilation timed out")
		return
	}

	sc, err := h.store.Upsert(r.Context(), projectID, eventName, req.Schema, req.Mode)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	// Bust the compile cache so subsequent events use the updated schema.
	h.validator.Invalidate(projectID, eventName)

	api.WriteJSON(w, http.StatusCreated, toResponse(sc))
}

// HandleGet handles GET /v1/projects/{projectID}/schemas/{event}.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}
	eventName := chi.URLParam(r, "event")

	sc, err := h.store.Get(r.Context(), projectID, eventName)
	if errors.Is(err, ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, "NOT_FOUND", "no schema registered for this event")
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, toResponse(sc))
}

// HandleDelete handles DELETE /v1/projects/{projectID}/schemas/{event}.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}
	eventName := chi.URLParam(r, "event")

	if err := h.store.Delete(r.Context(), projectID, eventName); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	h.validator.Invalidate(projectID, eventName)
	w.WriteHeader(http.StatusNoContent)
}

// HandleList handles GET /v1/projects/{projectID}/schemas.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}

	list, err := h.store.List(r.Context(), projectID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	out := make([]schemaResponse, len(list))
	for i, sc := range list {
		out[i] = toResponse(sc)
	}
	api.WriteJSON(w, http.StatusOK, out)
}

// scopeCheck verifies the URL projectID matches the authenticated project.
func scopeCheck(w http.ResponseWriter, r *http.Request) (string, bool) {
	ctxID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return "", false
	}
	urlID := chi.URLParam(r, "projectID")
	if ctxID != urlID {
		api.WriteError(w, http.StatusForbidden, "FORBIDDEN", "access denied")
		return "", false
	}
	return urlID, true
}
