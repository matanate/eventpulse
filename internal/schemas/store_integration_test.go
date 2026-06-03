//go:build integration

package schemas_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/matanate/eventpulse/internal/schemas"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runIntegration(m))
}

func runIntegration(m *testing.M) int {
	ctx := context.Background()

	pgCtr, err := tcpostgres.Run(ctx,
		"postgres:18.4-alpine",
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

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect pool: %v\n", err)
		return 1
	}
	defer testPool.Close()

	if err := runMigrations(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "migrations: %v\n", err)
		return 1
	}

	if _, err := seedProject(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "seed project: %v\n", err)
		return 1
	}

	return m.Run()
}

var seededProjectID string

func seedProject(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO projects (name, slug)
		VALUES ('test', 'test')
		RETURNING id`).Scan(&id)
	if err != nil {
		return "", err
	}
	seededProjectID = id
	return id, nil
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func TestStore_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	store := schemas.NewStore(testPool)

	raw := json.RawMessage(`{"type":"object","properties":{"price":{"type":"number"}}}`)
	sc, err := store.Upsert(ctx, seededProjectID, "purchase", raw, schemas.ModeEnforce)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if sc.EventName != "purchase" {
		t.Errorf("event_name = %q, want purchase", sc.EventName)
	}
	if sc.Mode != schemas.ModeEnforce {
		t.Errorf("mode = %q, want enforce", sc.Mode)
	}

	got, err := store.Get(ctx, seededProjectID, "purchase")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != sc.ID {
		t.Errorf("id mismatch after get")
	}
}

func TestStore_Upsert_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	store := schemas.NewStore(testPool)

	raw1 := json.RawMessage(`{}`)
	sc1, err := store.Upsert(ctx, seededProjectID, "click", raw1, schemas.ModeWarn)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	raw2 := json.RawMessage(`{"type":"object"}`)
	sc2, err := store.Upsert(ctx, seededProjectID, "click", raw2, schemas.ModeEnforce)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// Same row ג€” IDs match, mode updated.
	if sc1.ID != sc2.ID {
		t.Errorf("expected same row id after conflict update, got different ids")
	}
	if sc2.Mode != schemas.ModeEnforce {
		t.Errorf("mode not updated: got %q", sc2.Mode)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := schemas.NewStore(testPool)

	_, err := store.Get(ctx, seededProjectID, "nonexistent_event")
	if !errors.Is(err, schemas.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := schemas.NewStore(testPool)

	_, err := store.Upsert(ctx, seededProjectID, "delete_me", json.RawMessage(`{}`), schemas.ModeWarn)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := store.Delete(ctx, seededProjectID, "delete_me"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = store.Get(ctx, seededProjectID, "delete_me")
	if !errors.Is(err, schemas.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_Delete_Nonexistent(t *testing.T) {
	ctx := context.Background()
	store := schemas.NewStore(testPool)
	// Deleting a non-existent schema should not return an error.
	if err := store.Delete(ctx, seededProjectID, "does_not_exist"); err != nil {
		t.Errorf("expected nil error for missing delete, got %v", err)
	}
}

func TestStore_List(t *testing.T) {
	ctx := context.Background()
	store := schemas.NewStore(testPool)

	events := []string{"list_a", "list_b", "list_c"}
	for _, ev := range events {
		if _, err := store.Upsert(ctx, seededProjectID, ev, json.RawMessage(`{}`), schemas.ModeWarn); err != nil {
			t.Fatalf("upsert %s: %v", ev, err)
		}
	}

	list, err := store.List(ctx, seededProjectID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	found := make(map[string]bool)
	for _, sc := range list {
		found[sc.EventName] = true
	}
	for _, ev := range events {
		if !found[ev] {
			t.Errorf("event %q missing from list", ev)
		}
	}
}

// ג”€ג”€ Helpers ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
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
