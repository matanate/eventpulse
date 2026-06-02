//go:build integration

package analytics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/matangi/eventpulse/internal/analytics"
	"github.com/matangi/eventpulse/internal/auth"
)

// newFunnelTestServer returns a test server with only the funnel route mounted.
func newFunnelTestServer(projectID string) *httptest.Server {
	h := analytics.NewHandler(testPool)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithProjectID(req.Context(), projectID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Route("/v1/projects/{projectID}", func(r chi.Router) {
		r.Post("/funnels", h.HandleFunnel)
	})

	return httptest.NewServer(r)
}

// doPost sends a POST request with a JSON body to the test server.
func doPost(t *testing.T, srv *httptest.Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	must(t, err)
	req := httptest.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, req)
	return rec
}

// seedFunnelEvents inserts ordered events for a funnel test into testProjectID.
// Returns the funnel step names used.
func seedFunnelEvents(t *testing.T) (step0, step1, step2 string) {
	t.Helper()
	step0, step1, step2 = "funnel_enter", "funnel_engage", "funnel_convert"

	ctx := context.Background()
	now := time.Now().UTC()

	// User journeys:
	//   user_f1..f2: complete all 3 steps  → step0=7, step1=5, step2=2
	//   user_f3..f5: complete step0+step1
	//   user_f6..f7: complete step0 only
	journeys := []struct {
		user   string
		events []string
	}{
		{"user_f1", []string{step0, step1, step2}},
		{"user_f2", []string{step0, step1, step2}},
		{"user_f3", []string{step0, step1}},
		{"user_f4", []string{step0, step1}},
		{"user_f5", []string{step0, step1}},
		{"user_f6", []string{step0}},
		{"user_f7", []string{step0}},
	}

	for _, j := range journeys {
		for k, evt := range j.events {
			ts := now.Add(time.Duration(k) * time.Second)
			_, err := testPool.Exec(ctx,
				`INSERT INTO events (id, project_id, event, user_id, timestamp, received_at)
				 VALUES (gen_random_uuid(), $1, $2, $3, $4, $4)`,
				testProjectID, evt, j.user, ts,
			)
			if err != nil {
				t.Fatalf("seed funnel event %s/%s: %v", j.user, evt, err)
			}
		}
	}
	return
}

// ── Tests ──────────────────────────────────────────────────────────────────

func TestHandleFunnel_HappyPath(t *testing.T) {
	step0, step1, step2 := seedFunnelEvents(t)
	srv := newFunnelTestServer(testProjectID)
	defer srv.Close()

	rec := doPost(t, srv, "/v1/projects/"+testProjectID+"/funnels", map[string]any{
		"steps":  []string{step0, step1, step2},
		"window": "P7D",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result analytics.FunnelResult
	must(t, json.NewDecoder(rec.Body).Decode(&result))

	if len(result.Steps) != 3 {
		t.Fatalf("want 3 steps, got %d", len(result.Steps))
	}

	s0, s1, s2 := result.Steps[0], result.Steps[1], result.Steps[2]

	if s0.Entered != 7 {
		t.Errorf("step0 entered: want 7, got %d", s0.Entered)
	}
	if s1.Entered != 5 {
		t.Errorf("step1 entered: want 5, got %d", s1.Entered)
	}
	if s2.Entered != 2 {
		t.Errorf("step2 entered: want 2, got %d", s2.Entered)
	}
	if s0.Converted != 5 {
		t.Errorf("step0 converted: want 5, got %d", s0.Converted)
	}
	if s0.Dropped != 2 {
		t.Errorf("step0 dropped: want 2, got %d", s0.Dropped)
	}

	// Verify overall conversion rate ≈ 2/7
	want := float64(2) / float64(7)
	if result.OverallConversionRate < want-0.001 || result.OverallConversionRate > want+0.001 {
		t.Errorf("overall rate: want ~%.4f, got %.4f", want, result.OverallConversionRate)
	}

	// Last step has zero conversion metrics.
	if s2.Converted != 0 || s2.Dropped != 0 || s2.ConversionRate != 0 {
		t.Errorf("last step should have zero conversion fields, got %+v", s2)
	}

	if result.Window != "P7D" {
		t.Errorf("window echo: want P7D, got %s", result.Window)
	}
}

func TestHandleFunnel_EmptyResult(t *testing.T) {
	srv := newFunnelTestServer(testProjectID)
	defer srv.Close()

	rec := doPost(t, srv, "/v1/projects/"+testProjectID+"/funnels", map[string]any{
		"steps":  []string{"no_such_event_a", "no_such_event_b"},
		"window": "P30D",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result analytics.FunnelResult
	must(t, json.NewDecoder(rec.Body).Decode(&result))

	if len(result.Steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[0].Entered != 0 {
		t.Errorf("want entered=0, got %d", result.Steps[0].Entered)
	}
	if result.OverallConversionRate != 0 {
		t.Errorf("want overall_conversion_rate=0, got %f", result.OverallConversionRate)
	}
}

func TestHandleFunnel_ScopeCheck(t *testing.T) {
	srv := newFunnelTestServer(testProjectID)
	defer srv.Close()

	rec := doPost(t, srv, "/v1/projects/00000000-0000-0000-0000-000000000000/funnels", map[string]any{
		"steps":  []string{"page_view", "sign_up"},
		"window": "P7D",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestHandleFunnel_InvalidRequest(t *testing.T) {
	srv := newFunnelTestServer(testProjectID)
	defer srv.Close()
	base := "/v1/projects/" + testProjectID + "/funnels"

	cases := []struct {
		name string
		body any
		want int
	}{
		{"too few steps", map[string]any{"steps": []string{"only_one"}, "window": "P7D"}, 400},
		{"too many steps", map[string]any{"steps": makeSteps(9), "window": "P7D"}, 400},
		{"empty step name", map[string]any{"steps": []string{"valid", ""}, "window": "P7D"}, 400},
		{"duplicate step names", map[string]any{"steps": []string{"a", "a"}, "window": "P7D"}, 400},
		{"invalid window format", map[string]any{"steps": []string{"a", "b"}, "window": "7days"}, 400},
		{"window too long days", map[string]any{"steps": []string{"a", "b"}, "window": "P100D"}, 400},
		{"window too long weeks P13W", map[string]any{"steps": []string{"a", "b"}, "window": "P13W"}, 400},
		{"bad json", "not json", 400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body []byte
			if s, ok := tc.body.(string); ok {
				body = []byte(s)
			} else {
				b, err := json.Marshal(tc.body)
				must(t, err)
				body = b
			}
			req := httptest.NewRequest(http.MethodPost, srv.URL+base, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Config.Handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("want %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func makeSteps(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("step_%d", i)
	}
	return out
}
