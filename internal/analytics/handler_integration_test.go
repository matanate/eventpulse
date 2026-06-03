//go:build integration

package analytics_test

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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/matanate/eventpulse/internal/analytics"
	"github.com/matanate/eventpulse/internal/auth"
)

var (
	testPool      *pgxpool.Pool
	testProjectID string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
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

	testProjectID, err = seedData(ctx, testPool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		return 1
	}

	return m.Run()
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func TestHandleStats(t *testing.T) {
	srv := newTestServer(testProjectID)

	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/stats")

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body analytics.StatsResult
	must(t, json.NewDecoder(rec.Body).Decode(&body))

	if body.TotalEvents == 0 {
		t.Error("expected total_events > 0")
	}
	if body.TopEvents == nil {
		t.Error("expected top_events to be non-nil")
	}
}

func TestHandleStats_WrongProject(t *testing.T) {
	// Auth context has testProjectID but URL has a different project ג€” expect 403.
	srv := newTestServer(testProjectID)
	rec := doGet(t, srv, "/v1/projects/00000000-0000-0000-0000-000000000000/stats")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestHandleListEvents(t *testing.T) {
	srv := newTestServer(testProjectID)
	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/events?limit=10&offset=0")

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]json.RawMessage
	must(t, json.NewDecoder(rec.Body).Decode(&body))

	if _, ok := body["events"]; !ok {
		t.Error("missing 'events' key")
	}
	if _, ok := body["total"]; !ok {
		t.Error("missing 'total' key")
	}
}

func TestHandleListEvents_FilterByEventName(t *testing.T) {
	srv := newTestServer(testProjectID)
	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/events?event=page_view")

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Events []analytics.EventRow `json:"events"`
		Total  int64                `json:"total"`
	}
	must(t, json.NewDecoder(rec.Body).Decode(&body))

	for _, e := range body.Events {
		if e.Event != "page_view" {
			t.Errorf("expected event=page_view, got %s", e.Event)
		}
	}
}

func TestHandleListEvents_DateRange(t *testing.T) {
	srv := newTestServer(testProjectID)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")

	rec := doGet(t, srv, fmt.Sprintf(
		"/v1/projects/%s/events?from=%s&to=%s", testProjectID, yesterday, tomorrow,
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTopEvents(t *testing.T) {
	srv := newTestServer(testProjectID)
	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/events/top?n=5")

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Events []analytics.EventCount `json:"events"`
		N      int                    `json:"n"`
	}
	must(t, json.NewDecoder(rec.Body).Decode(&body))

	if body.N != 5 {
		t.Errorf("want n=5, got %d", body.N)
	}
	if body.Events == nil {
		t.Error("expected events to be non-nil")
	}
}

func TestHandleUserEvents(t *testing.T) {
	srv := newTestServer(testProjectID)
	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/users/user1/events")

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Events []analytics.EventRow `json:"events"`
		Total  int64                `json:"total"`
	}
	must(t, json.NewDecoder(rec.Body).Decode(&body))

	for _, e := range body.Events {
		if e.UserID != "user1" {
			t.Errorf("expected user_id=user1, got %s", e.UserID)
		}
	}
}

// ג”€ג”€ Helpers ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

// newTestServer returns an httptest.Server with analytics routes mounted and
// an auth middleware that injects projectID into every request context.
func newTestServer(projectID string) *httptest.Server {
	h := analytics.NewHandler(testPool)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithProjectID(req.Context(), projectID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	r.Route("/v1/projects/{projectID}", func(r chi.Router) {
		r.Get("/stats", h.HandleStats)
		r.Get("/events", h.HandleListEvents)
		r.Get("/events/top", h.HandleTopEvents)
		r.Route("/users/{userID}", func(r chi.Router) {
			r.Get("/events", h.HandleUserEvents)
		})
	})

	return httptest.NewServer(r)
}

func doGet(t *testing.T, srv *httptest.Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, srv.URL+path, nil)
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, req)
	return rec
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// seedData inserts a project + events and daily counts, returns the project ID.
func seedData(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var accountID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('Analytics Test Account') RETURNING id`,
	).Scan(&accountID); err != nil {
		return "", fmt.Errorf("insert account: %w", err)
	}

	var projectID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, 'Analytics Test Project') RETURNING id`,
		accountID,
	).Scan(&projectID); err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}

	now := time.Now().UTC()
	eventData := []struct {
		event  string
		userID string
	}{
		{"page_view", "user1"},
		{"page_view", "user1"},
		{"page_view", "user2"},
		{"button_click", "user1"},
		{"button_click", "user2"},
		{"sign_up", "user3"},
	}

	for i, d := range eventData {
		_, err := pool.Exec(ctx,
			`INSERT INTO events (id, project_id, event, user_id, timestamp, received_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $4)`,
			projectID, d.event, d.userID, now.Add(time.Duration(i)*time.Second),
		)
		if err != nil {
			return "", fmt.Errorf("insert event %d: %w", i, err)
		}
	}

	// Seed daily_event_counts for top-events and stats queries.
	counts := []struct {
		event string
		count int
	}{
		{"page_view", 3},
		{"button_click", 2},
		{"sign_up", 1},
	}
	today := now.Truncate(24 * time.Hour)
	for _, c := range counts {
		_, err := pool.Exec(ctx,
			`INSERT INTO daily_event_counts (project_id, event, date, count)
			 VALUES ($1, $2, $3, $4)`,
			projectID, c.event, today, c.count,
		)
		if err != nil {
			return "", fmt.Errorf("insert daily count: %w", err)
		}
	}

	return projectID, nil
}

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
