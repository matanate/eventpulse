//go:build integration

package webhooks_test

import (
	"bytes"
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

	"github.com/matanate/eventpulse/internal/auth"
	"github.com/matanate/eventpulse/internal/webhooks"
)

var (
	testPool      *pgxpool.Pool
	testProjectID string
	// Fixed 32-byte AES-256 key for integration tests.
	testSecretKey = bytes.Repeat([]byte("t"), 32)
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

	testProjectID, err = seedProject(ctx, testPool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed project: %v\n", err)
		return 1
	}

	return m.Run()
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func TestWebhookHandler_CreateAndList(t *testing.T) {
	handler := newTestHandler()
	srv := newTestServer(handler)
	defer srv.Close()

	body := `{"url":"http://example.com/hook","secret":"this-is-a-valid-secret-16chars"}`
	resp := doRequest(t, srv, http.MethodPost, "/v1/webhooks", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created) //nolint:errcheck
	resp.Body.Close()

	// Secret must not appear in the response.
	if _, ok := created["secret"]; ok {
		t.Error("create response must not contain secret")
	}
	if created["id"] == nil {
		t.Error("create response must contain id")
	}
	if created["url"] != "http://example.com/hook" {
		t.Errorf("unexpected url: %v", created["url"])
	}

	// List must include the created subscription.
	resp2 := doRequest(t, srv, http.MethodGet, "/v1/webhooks", "")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp2.StatusCode)
	}
	var list []map[string]any
	json.NewDecoder(resp2.Body).Decode(&list) //nolint:errcheck
	resp2.Body.Close()

	if len(list) == 0 {
		t.Fatal("expected at least one subscription in list")
	}
	for _, item := range list {
		if _, ok := item["secret"]; ok {
			t.Error("list response must not contain secret")
		}
	}
}

func TestWebhookHandler_CreateValidation(t *testing.T) {
	handler := webhooks.NewHandler(testPool, false, testSecretKey) // https-only
	srv := newTestServer(handler)
	defer srv.Close()

	tests := []struct {
		name string
		body string
		code int
	}{
		{"http url rejected", `{"url":"http://example.com/hook","secret":"valid-secret-16chars"}`, http.StatusUnprocessableEntity},
		{"private ip rejected", `{"url":"https://192.168.1.1/hook","secret":"valid-secret-16chars"}`, http.StatusUnprocessableEntity},
		{"secret too short", `{"url":"https://example.com/hook","secret":"short"}`, http.StatusUnprocessableEntity},
		{"invalid json", `{not json}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, srv, http.MethodPost, "/v1/webhooks", tt.body)
			if resp.StatusCode != tt.code {
				t.Errorf("expected %d, got %d", tt.code, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func TestWebhookHandler_Delete(t *testing.T) {
	handler := newTestHandler()
	srv := newTestServer(handler)
	defer srv.Close()

	// Create a subscription to delete.
	resp := doRequest(t, srv, http.MethodPost, "/v1/webhooks",
		`{"url":"http://example.com/deleteme","secret":"valid-secret-16chars"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d", resp.StatusCode)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created) //nolint:errcheck
	resp.Body.Close()
	id := created["id"].(string)

	// Delete it.
	resp2 := doRequest(t, srv, http.MethodDelete, "/v1/webhooks/"+id, "")
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Deleting again returns 404.
	resp3 := doRequest(t, srv, http.MethodDelete, "/v1/webhooks/"+id, "")
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("second delete: expected 404, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()
}

func TestWebhookHandler_Delete_IDOR(t *testing.T) {
	// Create a subscription under testProjectID.
	handler := newTestHandler()
	srv := newTestServer(handler)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodPost, "/v1/webhooks",
		`{"url":"http://example.com/idor","secret":"valid-secret-16chars"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d", resp.StatusCode)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created) //nolint:errcheck
	resp.Body.Close()
	id := created["id"].(string)

	// Attempt to delete using a different project's auth token.
	otherProjectID, err := seedProject(context.Background(), testPool)
	if err != nil {
		t.Fatalf("seed other project: %v", err)
	}
	otherSrv := newTestServerForProject(handler, otherProjectID)
	defer otherSrv.Close()

	resp2 := doRequest(t, otherSrv, http.MethodDelete, "/v1/webhooks/"+id, "")
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("IDOR: expected 404 for cross-project delete, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

func TestWebhookHandler_Unauthenticated(t *testing.T) {
	h := newTestHandler()

	r := chi.NewRouter()
	r.Post("/v1/webhooks", h.HandleCreate)
	r.Get("/v1/webhooks", h.HandleList)
	r.Delete("/v1/webhooks/{id}", h.HandleDelete)
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodGet} {
		resp, _ := http.DefaultClient.Do(mustNewRequest(method, srv.URL+"/v1/webhooks", "{}"))
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without auth: expected 401, got %d", method, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestEnqueueDeliveries(t *testing.T) {
	ctx := context.Background()

	// Use a fresh project so subscriptions created by other tests don't bleed in.
	projectID, err := seedProject(ctx, testPool)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Seed a subscription that matches any event.
	sub, err := webhooks.CreateSubscription(ctx, testPool, projectID,
		"http://example.com/hook", "secret-at-least-16-chars", nil)
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	defer testPool.Exec(ctx, "DELETE FROM webhook_subscriptions WHERE id = $1", sub.ID) //nolint:errcheck

	eventID := "00000000-0000-0000-0000-000000000001"
	payload := []byte(`{"event":"test","user_id":"u1"}`)
	n, err := webhooks.EnqueueDeliveries(ctx, testPool, projectID, "test", eventID, payload)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 delivery row, got %d", n)
	}

	// Second call with same event_id must be idempotent (dedupe index).
	n2, err := webhooks.EnqueueDeliveries(ctx, testPool, projectID, "test", eventID, payload)
	if err != nil {
		t.Fatalf("enqueue (2nd): %v", err)
	}
	if n2 != 0 {
		t.Errorf("expected 0 on duplicate, got %d", n2)
	}
}

func TestEnqueueDeliveries_FilterEvent(t *testing.T) {
	ctx := context.Background()

	// Use a fresh project so subscriptions created by other tests don't bleed in.
	projectID, err := seedProject(ctx, testPool)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	filterEvent := "purchase"
	sub, err := webhooks.CreateSubscription(ctx, testPool, projectID,
		"http://example.com/filter", "secret-at-least-16-chars", &filterEvent)
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	defer testPool.Exec(ctx, "DELETE FROM webhook_subscriptions WHERE id = $1", sub.ID) //nolint:errcheck

	payload := []byte(`{"event":"page_view"}`)

	// Non-matching event should not produce a delivery row.
	n, err := webhooks.EnqueueDeliveries(ctx, testPool, projectID, "page_view",
		"00000000-0000-0000-0000-000000000002", payload)
	if err != nil {
		t.Fatalf("enqueue non-matching: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows for non-matching event, got %d", n)
	}

	// Matching event should produce a row.
	n2, err := webhooks.EnqueueDeliveries(ctx, testPool, projectID, "purchase",
		"00000000-0000-0000-0000-000000000003", []byte(`{"event":"purchase"}`))
	if err != nil {
		t.Fatalf("enqueue matching: %v", err)
	}
	if n2 != 1 {
		t.Errorf("expected 1 row for matching event, got %d", n2)
	}
}

// ג”€ג”€ Helpers ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func newTestHandler() *webhooks.Handler {
	return webhooks.NewHandler(testPool, true, testSecretKey)
}

func newTestServer(h *webhooks.Handler) *httptest.Server {
	return newTestServerForProject(h, testProjectID)
}

func newTestServerForProject(h *webhooks.Handler, projectID string) *httptest.Server {
	r := chi.NewRouter()
	// Inject project_id the same way the auth middleware does.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithProjectID(r.Context(), projectID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Post("/v1/webhooks", h.HandleCreate)
	r.Get("/v1/webhooks", h.HandleList)
	r.Delete("/v1/webhooks/{id}", h.HandleDelete)
	return httptest.NewServer(r)
}

func doRequest(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	req := mustNewRequest(method, srv.URL+path, body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func mustNewRequest(method, url, body string) *http.Request {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	var req *http.Request
	if bodyReader != nil {
		req, _ = http.NewRequest(method, url, bodyReader)
	} else {
		req, _ = http.NewRequest(method, url, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
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

func seedProject(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	name := fmt.Sprintf("Webhook Test Account %d", time.Now().UnixNano())
	var accountID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ($1) RETURNING id`, name,
	).Scan(&accountID); err != nil {
		return "", fmt.Errorf("insert account: %w", err)
	}
	var projectID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, 'Webhook Test Project') RETURNING id`, accountID,
	).Scan(&projectID); err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}
	return projectID, nil
}
