//go:build integration

package analytics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/matanate/eventpulse/internal/analytics"
	"github.com/matanate/eventpulse/internal/auth"
)

func newRetentionTestServer(projectID string) *httptest.Server {
	h := analytics.NewHandler(testPool)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithProjectID(req.Context(), projectID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Route("/v1/projects/{projectID}", func(r chi.Router) {
		r.Get("/retention", h.HandleRetention)
	})

	return httptest.NewServer(r)
}

// seedRetentionUsers inserts daily_active_users rows across 4 days for a controlled
// retention scenario. All user IDs use a "ret_" prefix to avoid collisions with
// existing seed data.
//
// Cohort layout (cohorts=4, period=day):
//
//	cohort today-3: ret_u1..ret_u4 (size=4)
//	  D+1 retained: ret_u1, ret_u2, ret_u3  ג†’ rate=0.75
//	  D+2 retained: ret_u1, ret_u2          ג†’ rate=0.50
//	  D+3 retained: ret_u1                  ג†’ rate=0.25
//
//	cohort today-2: ret_u5, ret_u6 (size=2, first seen this day)
//	  D+1 retained: ret_u5                  ג†’ rate=0.50
//	  D+2 retained: ret_u5                  ג†’ rate=0.50
//
//	cohort today-1: ret_u7 (size=1, first seen this day)
//	  D+1 retained: ret_u7                  ג†’ rate=1.0
//
//	cohort today:   ret_u8 (size=1, first seen this day)
//	  D+0 only
func seedRetentionUsers(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	today := time.Now().UTC().Truncate(24 * time.Hour)
	d := func(daysBack int) time.Time { return today.AddDate(0, 0, -daysBack) }

	rows := []struct {
		user string
		date time.Time
	}{
		// Cohort today-3
		{"ret_u1", d(3)}, {"ret_u2", d(3)}, {"ret_u3", d(3)}, {"ret_u4", d(3)},
		// D+1 for cohort today-3
		{"ret_u1", d(2)}, {"ret_u2", d(2)}, {"ret_u3", d(2)},
		// D+2 for cohort today-3; also cohort today-2 first seen
		{"ret_u1", d(1)}, {"ret_u2", d(1)},
		{"ret_u5", d(2)}, {"ret_u6", d(2)},
		// D+1 for cohort today-2
		{"ret_u5", d(1)},
		// D+3 for cohort today-3; D+2 for today-2; cohort today-1 first seen
		{"ret_u1", today}, {"ret_u5", today},
		{"ret_u7", d(1)},
		// D+1 for today-1; cohort today first seen
		{"ret_u7", today},
		{"ret_u8", today},
	}

	for _, row := range rows {
		_, err := testPool.Exec(ctx,
			`INSERT INTO daily_active_users (project_id, date, user_id)
			 VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`,
			testProjectID, row.date, row.user,
		)
		if err != nil {
			t.Fatalf("seed retention user %s/%s: %v", row.user, row.date.Format("2006-01-02"), err)
		}
	}
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func TestHandleRetention_HappyPath(t *testing.T) {
	seedRetentionUsers(t)
	srv := newRetentionTestServer(testProjectID)
	defer srv.Close()

	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/retention?period=day&cohorts=4")

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result analytics.RetentionResult
	must(t, json.NewDecoder(rec.Body).Decode(&result))

	if result.Period != "day" {
		t.Errorf("period: want day, got %s", result.Period)
	}
	if result.Cohorts != 4 {
		t.Errorf("cohorts: want 4, got %d", result.Cohorts)
	}

	// Find cohort today-3 (oldest, last in DESC-ordered result).
	today := time.Now().UTC().Truncate(24 * time.Hour)
	oldestKey := today.AddDate(0, 0, -3).Format("2006-01-02")

	var oldestRow *analytics.RetentionRow
	for i := range result.Rows {
		if result.Rows[i].CohortDate == oldestKey {
			oldestRow = &result.Rows[i]
			break
		}
	}
	if oldestRow == nil {
		t.Fatalf("cohort %s not found in response; rows: %+v", oldestKey, result.Rows)
	}
	if oldestRow.CohortSize != 4 {
		t.Errorf("cohort %s size: want 4, got %d", oldestKey, oldestRow.CohortSize)
	}

	// D+0 must always be rate=1.0 and count=cohort_size.
	d0 := bucketByOffset(oldestRow.Buckets, 0)
	if d0 == nil {
		t.Fatal("D+0 bucket missing from oldest cohort")
	}
	if d0.Count != 4 {
		t.Errorf("D+0 count: want 4, got %d", d0.Count)
	}
	if d0.Rate < 0.999 {
		t.Errorf("D+0 rate: want 1.0, got %.4f", d0.Rate)
	}

	// D+1: 3 users ג†’ 0.75
	d1 := bucketByOffset(oldestRow.Buckets, 1)
	if d1 == nil {
		t.Fatal("D+1 bucket missing")
	}
	if d1.Count != 3 {
		t.Errorf("D+1 count: want 3, got %d", d1.Count)
	}
	if d1.Rate < 0.74 || d1.Rate > 0.76 {
		t.Errorf("D+1 rate: want ~0.75, got %.4f", d1.Rate)
	}

	// D+2: 2 users ג†’ 0.50
	d2 := bucketByOffset(oldestRow.Buckets, 2)
	if d2 == nil {
		t.Fatal("D+2 bucket missing")
	}
	if d2.Count != 2 {
		t.Errorf("D+2 count: want 2, got %d", d2.Count)
	}
	if d2.Rate < 0.49 || d2.Rate > 0.51 {
		t.Errorf("D+2 rate: want ~0.50, got %.4f", d2.Rate)
	}
}

func TestHandleRetention_EmptyResult(t *testing.T) {
	// Use a second project that has no daily_active_users rows.
	ctx := context.Background()
	var accountID string
	must(t, testPool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('Empty Retention Account') RETURNING id`,
	).Scan(&accountID))

	var emptyProjectID string
	must(t, testPool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, 'Empty Retention Project') RETURNING id`,
		accountID,
	).Scan(&emptyProjectID))

	srv := newRetentionTestServer(emptyProjectID)
	defer srv.Close()

	rec := doGet(t, srv, "/v1/projects/"+emptyProjectID+"/retention?period=day&cohorts=4")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result analytics.RetentionResult
	must(t, json.NewDecoder(rec.Body).Decode(&result))

	if len(result.Rows) != 0 {
		t.Errorf("want empty rows, got %d rows", len(result.Rows))
	}
}

func TestHandleRetention_ScopeCheck(t *testing.T) {
	srv := newRetentionTestServer(testProjectID)
	defer srv.Close()

	rec := doGet(t, srv, "/v1/projects/00000000-0000-0000-0000-000000000000/retention")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestHandleRetention_InvalidPeriod(t *testing.T) {
	srv := newRetentionTestServer(testProjectID)
	defer srv.Close()

	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/retention?period=week")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRetention_DefaultParams(t *testing.T) {
	srv := newRetentionTestServer(testProjectID)
	defer srv.Close()

	// No query params ג€” should use defaults (period=day, cohorts=8).
	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/retention")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result analytics.RetentionResult
	must(t, json.NewDecoder(rec.Body).Decode(&result))

	if result.Period != "day" {
		t.Errorf("default period: want day, got %s", result.Period)
	}
	if result.Cohorts != 8 {
		t.Errorf("default cohorts: want 8, got %d", result.Cohorts)
	}
}

func TestHandleRetention_CohortsClamped(t *testing.T) {
	srv := newRetentionTestServer(testProjectID)
	defer srv.Close()

	// cohorts=50 should clamp to maxCohorts (12).
	rec := doGet(t, srv, "/v1/projects/"+testProjectID+"/retention?cohorts=50")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result analytics.RetentionResult
	must(t, json.NewDecoder(rec.Body).Decode(&result))

	if result.Cohorts != 12 {
		t.Errorf("clamped cohorts: want 12, got %d", result.Cohorts)
	}
}

// ג”€ג”€ Helpers ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func bucketByOffset(buckets []analytics.RetentionBucket, offset int) *analytics.RetentionBucket {
	for i := range buckets {
		if buckets[i].Offset == offset {
			return &buckets[i]
		}
	}
	return nil
}
