package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/matangi/eventpulse/internal/api"
	"github.com/matangi/eventpulse/internal/auth"
)

const (
	defaultLimit = 50
	maxLimit     = 1000
	defaultN     = 10
	maxN         = 50
)

type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// scopeCheck verifies the URL project_id matches the authenticated project_id.
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

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}

	result, err := Stats(r.Context(), h.pool, projectID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}

	p := parseListParams(r)

	evts, total, err := ListEvents(r.Context(), h.pool, projectID, p)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"events": evts,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

func (h *Handler) HandleTopEvents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}

	n := queryInt(r, "n", defaultN, maxN)
	from := queryDate(r, "from")
	to := queryDate(r, "to")

	evts, err := TopEvents(r.Context(), h.pool, projectID, TopParams{N: n, From: from, To: to})
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"events": evts,
		"n":      n,
	})
}

func (h *Handler) HandleUserEvents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "userID")
	if userID == "" {
		api.WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "user_id is required")
		return
	}

	p := parseListParams(r)

	evts, total, err := UserEvents(r.Context(), h.pool, projectID, userID, p)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"events": evts,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

func (h *Handler) HandleFunnel(w http.ResponseWriter, r *http.Request) {
	projectID, ok := scopeCheck(w, r)
	if !ok {
		return
	}

	// 32 KB is generous for up to 8 step names of 100 chars each.
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)

	var req struct {
		Steps  []string `json:"steps"`
		Window string   `json:"window"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			api.WriteError(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "request body too large")
			return
		}
		api.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}

	if len(req.Steps) < minFunnelSteps || len(req.Steps) > maxFunnelSteps {
		api.WriteError(w, http.StatusBadRequest, "INVALID_PARAM",
			fmt.Sprintf("steps must have %d–%d entries", minFunnelSteps, maxFunnelSteps))
		return
	}
	seen := make(map[string]struct{}, len(req.Steps))
	for _, s := range req.Steps {
		if s == "" || len(s) > 100 {
			api.WriteError(w, http.StatusBadRequest, "INVALID_PARAM",
				"each step must be 1–100 characters")
			return
		}
		if _, dup := seen[s]; dup {
			api.WriteError(w, http.StatusBadRequest, "INVALID_PARAM", "duplicate step names are not allowed")
			return
		}
		seen[s] = struct{}{}
	}

	window, err := ParseFunnelWindow(req.Window)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "INVALID_PARAM", err.Error())
		return
	}

	queryCtx, queryCancel := contextWithFunnelTimeout(r.Context())
	defer queryCancel()

	result, err := Funnel(queryCtx, h.pool, projectID, FunnelParams{
		Steps:  req.Steps,
		Window: window,
	})
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, result)
}

// contextWithFunnelTimeout wraps ctx with a 10-second deadline for the funnel query.
// Funnel queries involve N-1 self-joins and can be significantly heavier than other analytics queries.
func contextWithFunnelTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 10*time.Second)
}

func parseListParams(r *http.Request) ListParams {
	return ListParams{
		Limit:  queryInt(r, "limit", defaultLimit, maxLimit),
		Offset: queryInt(r, "offset", 0, -1),
		Event:  r.URL.Query().Get("event"),
		UserID: r.URL.Query().Get("user_id"),
		From:   queryDate(r, "from"),
		To:     queryDate(r, "to"),
	}
}

// queryInt parses an integer query param. Returns def if missing or invalid.
// max <= 0 means no upper bound.
func queryInt(r *http.Request, key string, def, max int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

// queryDate parses a time query param. Accepts RFC3339 timestamps or YYYY-MM-DD dates.
// Returns zero time if missing or unparseable.
func queryDate(r *http.Request, key string) time.Time {
	s := r.URL.Query().Get(key)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
