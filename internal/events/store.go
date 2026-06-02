package events

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Store(ctx context.Context, pool *pgxpool.Pool, e *Event) error {
	var props any
	if e.Properties != nil {
		props = e.Properties
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO events (id, project_id, event, user_id, properties, idempotency_key, timestamp, received_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8)
		ON CONFLICT DO NOTHING`,
		e.ID, e.ProjectID, e.Event, e.UserID, props, e.IdempotencyKey, e.Timestamp, e.ReceivedAt,
	)
	if err != nil {
		return fmt.Errorf("store event: %w", err)
	}
	return nil
}

// UpsertDailyCount increments the aggregate counter for the event's date bucket.
// A failure here is non-fatal — the event is already persisted.
func UpsertDailyCount(ctx context.Context, pool *pgxpool.Pool, projectID, event string, date time.Time) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO daily_event_counts (project_id, event, date, count)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (project_id, event, date)
		DO UPDATE SET count = daily_event_counts.count + 1`,
		projectID, event, date.UTC().Truncate(24*time.Hour),
	)
	if err != nil {
		return fmt.Errorf("upsert daily count: %w", err)
	}
	return nil
}

// UpsertDailyActiveUser records that userID was active on the given date.
// Anonymous events (empty userID) are skipped. A failure here is non-fatal.
func UpsertDailyActiveUser(ctx context.Context, pool *pgxpool.Pool, projectID, userID string, date time.Time) error {
	if userID == "" {
		return nil
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO daily_active_users (project_id, date, user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`,
		projectID, date.UTC().Truncate(24*time.Hour), userID,
	)
	if err != nil {
		return fmt.Errorf("upsert daily active user: %w", err)
	}
	return nil
}

func BatchStore(ctx context.Context, pool *pgxpool.Pool, events []*Event) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}
	for _, e := range events {
		var props any
		if e.Properties != nil {
			props = e.Properties
		}
		batch.Queue(`
			INSERT INTO events (id, project_id, event, user_id, properties, idempotency_key, timestamp, received_at)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8)
			ON CONFLICT DO NOTHING`,
			e.ID, e.ProjectID, e.Event, e.UserID, props, e.IdempotencyKey, e.Timestamp, e.ReceivedAt,
		)
	}

	results := tx.SendBatch(ctx, batch)
	for range events {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return fmt.Errorf("batch insert: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("close batch results: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
