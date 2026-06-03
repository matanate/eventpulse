package webhooks

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/matanate/eventpulse/internal/api"
	"github.com/matanate/eventpulse/internal/auth"
)

const (
	minSecretLen = 16
	maxSecretLen = 256
	maxEventLen  = 100
)

// Handler exposes CRUD endpoints for webhook subscriptions.
type Handler struct {
	pool      *pgxpool.Pool
	allowHTTP bool
	secretKey []byte // AES-256 key for at-rest secret encryption
}

// NewHandler creates a Handler. allowHTTP must only be true in development.
// secretKey must be exactly 32 bytes (AES-256); panics otherwise.
func NewHandler(pool *pgxpool.Pool, allowHTTP bool, secretKey []byte) *Handler {
	if len(secretKey) != 32 {
		panic("webhooks.NewHandler: secretKey must be exactly 32 bytes")
	}
	return &Handler{pool: pool, allowHTTP: allowHTTP, secretKey: secretKey}
}

// subscriptionResponse is the public DTO ג€” secret is intentionally absent.
type subscriptionResponse struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	FilterEvent *string   `json:"filter_event,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

func toResponse(s Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:          s.ID,
		URL:         s.URL,
		FilterEvent: s.FilterEvent,
		Active:      s.Active,
		CreatedAt:   s.CreatedAt,
	}
}

type createRequest struct {
	URL         string  `json:"url"`
	Secret      string  `json:"secret"`
	FilterEvent *string `json:"filter_event,omitempty"`
}

// HandleCreate handles POST /v1/webhooks.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			api.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
			return
		}
		api.WriteError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid JSON")
		return
	}

	var fieldErrs []api.FieldError
	if err := ValidateURL(req.URL, h.allowHTTP); err != nil {
		fieldErrs = append(fieldErrs, api.FieldError{Field: "url", Message: err.Error()})
	}
	if len(req.Secret) < minSecretLen {
		fieldErrs = append(fieldErrs, api.FieldError{Field: "secret", Message: "must be at least 16 characters"})
	}
	if len(req.Secret) > maxSecretLen {
		fieldErrs = append(fieldErrs, api.FieldError{Field: "secret", Message: "must be at most 256 characters"})
	}
	if req.FilterEvent != nil && len(*req.FilterEvent) > maxEventLen {
		fieldErrs = append(fieldErrs, api.FieldError{Field: "filter_event", Message: "must be at most 100 characters"})
	}
	if len(fieldErrs) > 0 {
		api.WriteValidationError(w, fieldErrs)
		return
	}

	encryptedSecret, err := EncryptSecret([]byte(req.Secret), h.secretKey)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	sub, err := CreateSubscription(r.Context(), h.pool, projectID, req.URL, encryptedSecret, req.FilterEvent)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusCreated, toResponse(sub))
}

// HandleList handles GET /v1/webhooks.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return
	}

	subs, err := ListSubscriptions(r.Context(), h.pool, projectID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	resp := make([]subscriptionResponse, len(subs))
	for i, s := range subs {
		resp[i] = toResponse(s)
	}
	api.WriteJSON(w, http.StatusOK, resp)
}

// HandleDelete handles DELETE /v1/webhooks/{id}.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return
	}

	id := chi.URLParam(r, "id")
	if !isValidUUID(id) {
		api.WriteError(w, http.StatusNotFound, "NOT_FOUND", "subscription not found")
		return
	}

	deleted, err := DeleteSubscription(r.Context(), h.pool, projectID, id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if !deleted {
		// Return 404 for both "not found" and "belongs to another project" ג€” avoids
		// enumeration of other projects' subscription IDs.
		api.WriteError(w, http.StatusNotFound, "NOT_FOUND", "subscription not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// isValidUUID does a lightweight structural check on a UUID string.
func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}
