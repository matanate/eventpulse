package queue

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/matanate/eventpulse/internal/api"
	"github.com/matanate/eventpulse/internal/auth"
)

// QueueStatsSource is the minimal interface for reading live queue state.
type QueueStatsSource interface {
	PendingCount(ctx context.Context) (int64, error)
}

// StatsHandler exposes a read-only monitoring endpoint for queue state.
type StatsHandler struct {
	source QueueStatsSource
	pool   *pgxpool.Pool
}

func NewStatsHandler(source QueueStatsSource, pool *pgxpool.Pool) *StatsHandler {
	return &StatsHandler{source: source, pool: pool}
}

type queueStatsResponse struct {
	PendingMessages int64 `json:"pending_messages"`
	DeadLetterCount int64 `json:"dead_letter_count"`
}

// HandleQueueStats returns the current queue depth and dead-letter event count
// scoped to the caller's project.
// pending_messages reflects global queue depth (the stream is not partitioned by project).
func (h *StatsHandler) HandleQueueStats(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authorization required")
		return
	}

	pending, err := h.source.PendingCount(r.Context())
	if err != nil {
		slog.Warn("queue stats: pending count unavailable", "err", err)
		api.WriteError(w, http.StatusServiceUnavailable, "QUEUE_UNAVAILABLE", "queue unavailable")
		return
	}

	var deadLetterCount int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM dead_letter_events WHERE project_id = $1`,
		projectID,
	).Scan(&deadLetterCount); err != nil {
		slog.Warn("queue stats: dead letter count unavailable", "err", err)
		api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	api.WriteJSON(w, http.StatusOK, queueStatsResponse{
		PendingMessages: pending,
		DeadLetterCount: deadLetterCount,
	})
}
