package events_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/matangi/eventpulse/internal/auth"
	"github.com/matangi/eventpulse/internal/events"
)

// fakeSchemaValidator is a controllable SchemaValidator for handler tests.
type fakeSchemaValidator struct {
	violations []string
	enforce    bool
	err        error
}

func (f *fakeSchemaValidator) Validate(_ context.Context, _ string, _ string, _ map[string]any) ([]string, bool, error) {
	return f.violations, f.enforce, f.err
}

func newTestServerWithSchema(pub events.Publisher, v *fakeSchemaValidator) *httptest.Server {
	h := events.NewHandler(pub, nil).WithSchemaValidator(v)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if pid := req.Header.Get("X-Project-ID"); pid != "" {
				req = req.WithContext(auth.WithProjectID(req.Context(), pid))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Post("/v1/events", h.HandleIngest)
	r.Post("/v1/events/batch", h.HandleBatchIngest)
	return httptest.NewServer(r)
}

// ── Tests ──────────────────────────────────────────────────────────────────

func TestHandleIngest_SchemaEnforce_Rejects(t *testing.T) {
	pub := &fakePublisher{}
	validator := &fakeSchemaValidator{
		violations: []string{"price: expected number, got string"},
		enforce:    true,
	}
	srv := newTestServerWithSchema(pub, validator)
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events", map[string]any{
		"event":      "purchase",
		"user_id":    "u1",
		"properties": map[string]any{"price": "not-a-number"},
	}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "SCHEMA_VIOLATION" {
		t.Errorf("expected SCHEMA_VIOLATION code, got %q", body["code"])
	}
	if len(pub.published) != 0 {
		t.Errorf("expected 0 published events, got %d", len(pub.published))
	}
}

func TestHandleIngest_SchemaWarn_Accepts(t *testing.T) {
	pub := &fakePublisher{}
	validator := &fakeSchemaValidator{
		violations: []string{"price: expected number, got string"},
		enforce:    false,
	}
	srv := newTestServerWithSchema(pub, validator)
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events", map[string]any{
		"event":      "purchase",
		"user_id":    "u1",
		"properties": map[string]any{"price": "not-a-number"},
	}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d, want 202", resp.StatusCode)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected event published in warn mode, got %d", len(pub.published))
	}
}

func TestHandleIngest_SchemaNoViolations_Accepts(t *testing.T) {
	pub := &fakePublisher{}
	srv := newTestServerWithSchema(pub, &fakeSchemaValidator{violations: nil})
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events", map[string]any{
		"event": "purchase", "user_id": "u1",
	}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d, want 202", resp.StatusCode)
	}
}

func TestHandleBatchIngest_SchemaEnforce_Rejects(t *testing.T) {
	pub := &fakePublisher{}
	validator := &fakeSchemaValidator{
		violations: []string{"required: price"},
		enforce:    true,
	}
	srv := newTestServerWithSchema(pub, validator)
	defer srv.Close()

	resp := postJSON(t, srv, "/v1/events/batch", map[string]any{
		"events": []map[string]any{
			{"event": "purchase", "user_id": "u1"},
		},
	}, withProjectID())
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", resp.StatusCode)
	}
	if len(pub.published) != 0 {
		t.Errorf("expected 0 published events, got %d", len(pub.published))
	}
}
