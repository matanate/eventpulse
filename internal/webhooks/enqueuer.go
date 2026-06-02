package webhooks

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Enqueuer implements worker.WebhookEnqueuer using a Postgres pool.
// It is constructed in cmd/worker/main.go and injected into the Worker.
type Enqueuer struct {
	pool *pgxpool.Pool
}

// NewEnqueuer creates an Enqueuer backed by pool.
func NewEnqueuer(pool *pgxpool.Pool) *Enqueuer {
	return &Enqueuer{pool: pool}
}

func (e *Enqueuer) EnqueueDeliveries(ctx context.Context, projectID, eventName, eventID string, payload []byte) (int, error) {
	return EnqueueDeliveries(ctx, e.pool, projectID, eventName, eventID, payload)
}
