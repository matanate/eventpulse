//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/matanate/eventpulse/internal/queue"
)

// fakeInspector is a test stub that returns a fixed pending count.
type fakeInspector struct{ count int64 }

func (f *fakeInspector) PendingCount(_ context.Context) (int64, error) {
	return f.count, nil
}

var statsPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runStats(m))
}

func runStats(m *testing.M) int {
	ctx := context.Background()

	pgCtr, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		return 1
	}
	defer pgCtr.Terminate(ctx) //nolint:errcheck

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		return 1
	}

	statsPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect pool: %v\n", err)
		return 1
	}
	defer statsPool.Close()

	if err := runMigrationsStats(ctx, statsPool); err != nil {
		fmt.Fprintf(os.Stderr, "migrations: %v\n", err)
		return 1
	}

	return m.Run()
}

func TestHandleQueueStats_Empty(t *testing.T) {
	handler := queue.NewStatsHandler(&fakeInspector{count: 0}, statsPool)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/queue/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleQueueStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		PendingMessages int64 `json:"pending_messages"`
		DeadLetterCount int64 `json:"dead_letter_count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.PendingMessages != 0 {
		t.Errorf("want pending_messages=0, got %d", body.PendingMessages)
	}
	if body.DeadLetterCount != 0 {
		t.Errorf("want dead_letter_count=0, got %d", body.DeadLetterCount)
	}
}

func TestHandleQueueStats_WithPending(t *testing.T) {
	handler := queue.NewStatsHandler(&fakeInspector{count: 7}, statsPool)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/queue/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleQueueStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		PendingMessages int64 `json:"pending_messages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.PendingMessages != 7 {
		t.Errorf("want pending_messages=7, got %d", body.PendingMessages)
	}
}

func TestHandleQueueStats_WithDeadLetters(t *testing.T) {
	ctx := context.Background()

	// Insert a dead-letter row.
	_, err := statsPool.Exec(ctx,
		`INSERT INTO dead_letter_events (raw_payload, error, attempt_count)
		 VALUES ('{"event":"test"}'::jsonb, 'test error', 3)`,
	)
	if err != nil {
		t.Fatalf("insert dead letter: %v", err)
	}
	t.Cleanup(func() {
		statsPool.Exec(ctx, `DELETE FROM dead_letter_events`) //nolint:errcheck
	})

	handler := queue.NewStatsHandler(&fakeInspector{count: 0}, statsPool)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/queue/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleQueueStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		DeadLetterCount int64 `json:"dead_letter_count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.DeadLetterCount != 1 {
		t.Errorf("want dead_letter_count=1, got %d", body.DeadLetterCount)
	}
}

func runMigrationsStats(ctx context.Context, pool *pgxpool.Pool) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}
	migrationsDir := filepath.Join(wd, "..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		sql, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec %s: %w", entry.Name(), err)
		}
	}
	return nil
}
