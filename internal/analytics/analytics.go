package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EventCount pairs an event name with its aggregate count.
type EventCount struct {
	Event string `json:"event"`
	Count int64  `json:"count"`
}

// EventRow is a single event returned by list queries.
type EventRow struct {
	ID         string         `json:"id"`
	Event      string         `json:"event"`
	UserID     string         `json:"user_id,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	ReceivedAt time.Time      `json:"received_at"`
}

// StatsResult is the response for the stats endpoint.
type StatsResult struct {
	TotalEvents int64        `json:"total_events"`
	TodayCount  int64        `json:"today_count"`
	TopEvents   []EventCount `json:"top_events"`
}

// ListParams controls pagination and filtering for list queries.
type ListParams struct {
	Limit  int
	Offset int
	Event  string    // optional filter
	UserID string    // optional filter
	From   time.Time // zero = unset
	To     time.Time // zero = unset
}

// TopParams controls the top-events query.
type TopParams struct {
	N    int
	From time.Time
	To   time.Time
}

// Stats returns aggregate statistics for a project.
func Stats(ctx context.Context, pool *pgxpool.Pool, projectID string) (StatsResult, error) {
	var res StatsResult

	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE project_id = $1`, projectID,
	).Scan(&res.TotalEvents); err != nil {
		return StatsResult{}, fmt.Errorf("stats total: %w", err)
	}

	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM daily_event_counts
		 WHERE project_id = $1 AND date = CURRENT_DATE`, projectID,
	).Scan(&res.TodayCount); err != nil {
		return StatsResult{}, fmt.Errorf("stats today: %w", err)
	}

	rows, err := pool.Query(ctx,
		`SELECT event, SUM(count) AS total FROM daily_event_counts
		 WHERE project_id = $1
		 GROUP BY event ORDER BY total DESC LIMIT 5`, projectID,
	)
	if err != nil {
		return StatsResult{}, fmt.Errorf("stats top events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ec EventCount
		if err := rows.Scan(&ec.Event, &ec.Count); err != nil {
			return StatsResult{}, fmt.Errorf("stats top events scan: %w", err)
		}
		res.TopEvents = append(res.TopEvents, ec)
	}
	if res.TopEvents == nil {
		res.TopEvents = []EventCount{}
	}
	return res, rows.Err()
}

// ListEvents returns a paginated, filtered list of events and the total matching count.
func ListEvents(ctx context.Context, pool *pgxpool.Pool, projectID string, p ListParams) ([]EventRow, int64, error) {
	args := []any{projectID}
	where := "WHERE project_id = $1"
	idx := 2

	if p.Event != "" {
		where += fmt.Sprintf(" AND event = $%d", idx)
		args = append(args, p.Event)
		idx++
	}
	if p.UserID != "" {
		where += fmt.Sprintf(" AND user_id = $%d", idx)
		args = append(args, p.UserID)
		idx++
	}
	if !p.From.IsZero() {
		where += fmt.Sprintf(" AND timestamp >= $%d", idx)
		args = append(args, p.From)
		idx++
	}
	if !p.To.IsZero() {
		where += fmt.Sprintf(" AND timestamp <= $%d", idx)
		args = append(args, p.To)
		idx++
	}

	var total int64
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM events "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("list events count: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, event, COALESCE(user_id, ''), properties, timestamp, received_at
		 FROM events %s ORDER BY timestamp DESC, id DESC LIMIT $%d OFFSET $%d`,
		where, idx, idx+1,
	)
	args = append(args, p.Limit, p.Offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list events query: %w", err)
	}
	defer rows.Close()

	var evts []EventRow
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.ID, &e.Event, &e.UserID, &e.Properties, &e.Timestamp, &e.ReceivedAt); err != nil {
			return nil, 0, fmt.Errorf("list events scan: %w", err)
		}
		evts = append(evts, e)
	}
	if evts == nil {
		evts = []EventRow{}
	}
	return evts, total, rows.Err()
}

// TopEvents returns the top N event names by aggregate count, optionally filtered by date range.
func TopEvents(ctx context.Context, pool *pgxpool.Pool, projectID string, p TopParams) ([]EventCount, error) {
	args := []any{projectID}
	where := "WHERE project_id = $1"
	idx := 2

	if !p.From.IsZero() {
		where += fmt.Sprintf(" AND date >= $%d", idx)
		args = append(args, p.From.UTC().Truncate(24*time.Hour))
		idx++
	}
	if !p.To.IsZero() {
		where += fmt.Sprintf(" AND date <= $%d", idx)
		args = append(args, p.To.UTC().Truncate(24*time.Hour))
		idx++
	}

	query := fmt.Sprintf(
		`SELECT event, SUM(count) AS total FROM daily_event_counts
		 %s GROUP BY event ORDER BY total DESC LIMIT $%d`,
		where, idx,
	)
	args = append(args, p.N)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("top events query: %w", err)
	}
	defer rows.Close()

	var out []EventCount
	for rows.Next() {
		var ec EventCount
		if err := rows.Scan(&ec.Event, &ec.Count); err != nil {
			return nil, fmt.Errorf("top events scan: %w", err)
		}
		out = append(out, ec)
	}
	if out == nil {
		out = []EventCount{}
	}
	return out, rows.Err()
}

// UserEvents returns a paginated list of events for a specific user.
func UserEvents(ctx context.Context, pool *pgxpool.Pool, projectID, userID string, p ListParams) ([]EventRow, int64, error) {
	p.UserID = userID
	return ListEvents(ctx, pool, projectID, p)
}
