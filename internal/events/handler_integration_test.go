package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/matanate/eventpulse/internal/auth"
	"github.com/matanate/eventpulse/internal/events"
)

// fakePublisher captures published events in memory.
type fakePublisher struct {
	published []*events.Event
}

func (f *fakePublisher) Publish(_ context.Context, e *events.Event) error {
	f.published = append(f.published, e)
	return nil
}

func (f *fakePublisher) PublishBatch(_ context.Context, evts []*events.Event) error {
	f.published = append(f.published, evts...)
	return nil
}

const testProjectID = "test-project-id"

func newTestServer(pub events.Publisher) *httptest.Server {
	h := events.NewHandler(pub, nil)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pid := r.Header.Get("X-Project-ID"); pid != "" {
				r = r.WithContext(auth.WithProjectID(r.Context(), pid))
			}
			next.ServeHTTP(w, r)
		})
	})
	r.Post("/v1/events", h.HandleIngest)
	r.Post("/v1/events/batch", h.HandleBatchIngest)
	return httptest.NewServer(r)
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func withProjectID() map[string]string {
	return map[string]string{"X-Project-ID": testProjectID}
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func TestHandleIngest_HappyPath(t *testing.T) {
	pub := &fakePublisher{}
	srv := newTestServer(pub)
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events", map[string]any{
		"event":   "page_view",
		"user_id": "u1",
	}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d, want 202", resp.StatusCode)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].Event != "page_view" {
		t.Errorf("published event name = %q, want page_view", pub.published[0].Event)
	}
}

func TestHandleIngest_MissingProjectID(t *testing.T) {
	srv := newTestServer(&fakePublisher{})
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events", map[string]any{"event": "test"}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestHandleIngest_InvalidJSON(t *testing.T) {
	srv := newTestServer(&fakePublisher{})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/events", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-ID", testProjectID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestHandleIngest_EmptyEventName(t *testing.T) {
	srv := newTestServer(&fakePublisher{})
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events", map[string]any{"event": ""}, withProjectID())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
}

func TestHandleBatchIngest_HappyPath(t *testing.T) {
	pub := &fakePublisher{}
	srv := newTestServer(pub)
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events/batch", map[string]any{
		"events": []map[string]any{
			{"event": "batch_a", "user_id": "ua"},
			{"event": "batch_b", "user_id": "ub"},
			{"event": "batch_c"},
		},
	}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d, want 202", resp.StatusCode)
	}
	if len(pub.published) != 3 {
		t.Fatalf("expected 3 published events, got %d", len(pub.published))
	}

	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if count, _ := body["count"].(float64); count != 3 {
		t.Errorf("expected count=3, got %v", body["count"])
	}
}

func TestHandleBatchIngest_TooLarge(t *testing.T) {
	srv := newTestServer(&fakePublisher{})
	defer srv.Close()

	evts := make([]map[string]any, 101)
	for i := range evts {
		evts[i] = map[string]any{"event": "e"}
	}
	resp := postJSON(t, srv, "/v1/events/batch", map[string]any{"events": evts}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "BATCH_TOO_LARGE" {
		t.Errorf("expected BATCH_TOO_LARGE, got %q", body["code"])
	}
}

func TestHandleBatchIngest_Empty(t *testing.T) {
	srv := newTestServer(&fakePublisher{})
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events/batch", map[string]any{"events": []any{}}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "BATCH_EMPTY" {
		t.Errorf("expected BATCH_EMPTY, got %q", body["code"])
	}
}
