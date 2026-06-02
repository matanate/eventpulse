package webhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateSubscription inserts a new webhook subscription and returns the created row.
// The secret is stored as plaintext in this portfolio implementation. In a
// production system, encrypt it with AES-GCM before writing and decrypt after reading.
func CreateSubscription(ctx context.Context, pool *pgxpool.Pool, projectID, rawURL, secret string, filterEvent *string) (Subscription, error) {
	var s Subscription
	err := pool.QueryRow(ctx, `
		INSERT INTO webhook_subscriptions (project_id, url, secret, filter_event)
		VALUES ($1, $2, $3, $4)
		RETURNING id, project_id, url, secret, filter_event, active, created_at`,
		projectID, rawURL, secret, filterEvent,
	).Scan(&s.ID, &s.ProjectID, &s.URL, &s.Secret, &s.FilterEvent, &s.Active, &s.CreatedAt)
	if err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return s, nil
}

// ListSubscriptions returns all active and inactive subscriptions for the project.
func ListSubscriptions(ctx context.Context, pool *pgxpool.Pool, projectID string) ([]Subscription, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, project_id, url, secret, filter_event, active, created_at
		FROM webhook_subscriptions
		WHERE project_id = $1
		ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.URL, &s.Secret, &s.FilterEvent, &s.Active, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// DeleteSubscription deletes a subscription by id scoped to projectID.
// Returns (true, nil) when a row was deleted, (false, nil) when not found or
// owned by a different project (prevents IDOR enumeration).
func DeleteSubscription(ctx context.Context, pool *pgxpool.Pool, projectID, id string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`DELETE FROM webhook_subscriptions WHERE id = $1 AND project_id = $2`,
		id, projectID,
	)
	if err != nil {
		return false, fmt.Errorf("delete subscription: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// EnqueueDeliveries inserts a webhook_deliveries row for every active subscription
// that matches (projectID, eventName). The INSERT uses ON CONFLICT DO NOTHING so
// reprocessed stream messages are idempotent.
// Returns the number of rows inserted.
func EnqueueDeliveries(ctx context.Context, pool *pgxpool.Pool, projectID, eventName, eventID string, payload []byte) (int, error) {
	tag, err := pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (subscription_id, event_id, payload)
		SELECT id, $3, $4::jsonb
		FROM webhook_subscriptions
		WHERE project_id = $1
		  AND active
		  AND (filter_event IS NULL OR filter_event = $2)
		ON CONFLICT (subscription_id, event_id) DO NOTHING`,
		projectID, eventName, eventID, payload,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue deliveries: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ClaimDueDeliveries claims up to limit pending delivery rows that are due for
// processing. Claimed rows have their next_retry_at bumped by claimWindow so that
// concurrent dispatcher instances or a crash mid-delivery don't double-send.
// The subscription URL and secret are joined in the same query to avoid N+1.
func ClaimDueDeliveries(ctx context.Context, pool *pgxpool.Pool, limit int, claimWindow time.Duration) ([]DeliveryWithSub, error) {
	claimUntil := time.Now().Add(claimWindow)

	rows, err := pool.Query(ctx, `
		WITH claimed AS (
			UPDATE webhook_deliveries
			SET next_retry_at = $2
			WHERE id IN (
				SELECT id FROM webhook_deliveries
				WHERE status = 'pending' AND next_retry_at <= now()
				ORDER BY next_retry_at
				FOR UPDATE SKIP LOCKED
				LIMIT $1
			)
			RETURNING id, subscription_id, event_id, status, attempts, last_error, payload, created_at
		)
		SELECT c.id, c.subscription_id, c.event_id, c.status, c.attempts,
		       c.last_error, c.payload, c.created_at,
		       s.url, s.secret
		FROM claimed c
		JOIN webhook_subscriptions s ON s.id = c.subscription_id`,
		limit, claimUntil,
	)
	if err != nil {
		return nil, fmt.Errorf("claim due deliveries: %w", err)
	}
	defer rows.Close()

	var results []DeliveryWithSub
	for rows.Next() {
		var d DeliveryWithSub
		if err := rows.Scan(
			&d.ID, &d.SubscriptionID, &d.EventID, &d.Status, &d.Attempts,
			&d.LastError, &d.Payload, &d.CreatedAt,
			&d.URL, &d.Secret,
		); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// MarkDelivered sets a delivery row to 'delivered'.
func MarkDelivered(ctx context.Context, pool *pgxpool.Pool, id string) error {
	_, err := pool.Exec(ctx,
		`UPDATE webhook_deliveries SET status = $1 WHERE id = $2`,
		StatusDelivered, id,
	)
	if err != nil {
		return fmt.Errorf("mark delivered: %w", err)
	}
	return nil
}

// RescheduleOrFail increments the attempt counter and either reschedules the
// delivery for the next retry or marks it as permanently failed.
func RescheduleOrFail(ctx context.Context, pool *pgxpool.Pool, id string, attempts int, lastErr string, nextRetryAt time.Time, markFailed bool) error {
	status := StatusPending
	if markFailed {
		status = StatusFailed
	}
	_, err := pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET attempts = $2, last_error = $3, next_retry_at = $4, status = $5
		WHERE id = $1`,
		id, attempts, lastErr, nextRetryAt, status,
	)
	if err != nil {
		return fmt.Errorf("reschedule delivery: %w", err)
	}
	return nil
}
