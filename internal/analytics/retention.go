package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultCohorts = 8
	maxCohorts     = 12
)

// RetentionBucket is one cell in the cohort matrix: users who returned offset days after cohort day.
type RetentionBucket struct {
	Offset int     `json:"offset"`
	Count  int64   `json:"count"`
	Rate   float64 `json:"rate"`
}

// RetentionRow is a single cohort (one row in the heatmap).
type RetentionRow struct {
	CohortDate  string            `json:"cohort_date"`
	CohortSize  int64             `json:"cohort_size"`
	Buckets     []RetentionBucket `json:"buckets"`
}

// RetentionResult is the full response for the retention endpoint.
type RetentionResult struct {
	Period  string         `json:"period"`
	Cohorts int            `json:"cohorts"`
	Rows    []RetentionRow `json:"rows"`
}

// RetentionParams controls the retention query.
type RetentionParams struct {
	Period  string // only "day" is supported
	Cohorts int    // 1–maxCohorts
}

// Retention computes a triangular cohort-retention matrix for the given project.
//
// Users are grouped into cohorts by the first day they appear in daily_active_users
// within the observation window [today - (cohorts-1) days, today]. For each cohort,
// the function counts how many of those users returned on each subsequent day.
func Retention(ctx context.Context, pool *pgxpool.Pool, projectID string, p RetentionParams) (RetentionResult, error) {
	startDate := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(p.Cohorts - 1))

	const query = `
		WITH first_seen AS (
			SELECT user_id, MIN(date) AS cohort_date
			FROM daily_active_users
			WHERE project_id = $1
			  AND date >= $2
			GROUP BY user_id
		),
		cohort_sizes AS (
			SELECT cohort_date, COUNT(*) AS size
			FROM first_seen
			GROUP BY cohort_date
		),
		retention_counts AS (
			SELECT
				fs.cohort_date,
				(dau.date - fs.cohort_date)::integer AS period_offset,
				COUNT(*) AS retained
			FROM first_seen fs
			JOIN daily_active_users dau
				ON  dau.project_id = $1
				AND dau.user_id    = fs.user_id
			WHERE dau.date >= fs.cohort_date
			  AND dau.date <= CURRENT_DATE
			GROUP BY fs.cohort_date, period_offset
		)
		SELECT r.cohort_date, cs.size, r.period_offset, r.retained
		FROM retention_counts r
		JOIN cohort_sizes cs ON cs.cohort_date = r.cohort_date
		ORDER BY r.cohort_date DESC, r.period_offset ASC`

	rows, err := pool.Query(ctx, query, projectID, startDate)
	if err != nil {
		return RetentionResult{}, fmt.Errorf("retention query: %w", err)
	}
	defer rows.Close()

	// rowMap preserves insertion order (DESC cohort_date from ORDER BY).
	type rowKey = string
	rowMap := make(map[rowKey]*RetentionRow)
	var order []rowKey

	for rows.Next() {
		var (
			cohortDate  time.Time
			cohortSize  int64
			offset      int
			retained    int64
		)
		if err := rows.Scan(&cohortDate, &cohortSize, &offset, &retained); err != nil {
			return RetentionResult{}, fmt.Errorf("retention scan: %w", err)
		}

		key := cohortDate.Format("2006-01-02")
		if _, exists := rowMap[key]; !exists {
			rowMap[key] = &RetentionRow{
				CohortDate: key,
				CohortSize: cohortSize,
				Buckets:    []RetentionBucket{},
			}
			order = append(order, key)
		}

		rate := 0.0
		if cohortSize > 0 {
			rate = float64(retained) / float64(cohortSize)
		}
		rowMap[key].Buckets = append(rowMap[key].Buckets, RetentionBucket{
			Offset: offset,
			Count:  retained,
			Rate:   rate,
		})
	}
	if err := rows.Err(); err != nil {
		return RetentionResult{}, fmt.Errorf("retention rows: %w", err)
	}

	result := RetentionResult{
		Period:  p.Period,
		Cohorts: p.Cohorts,
		Rows:    make([]RetentionRow, 0, len(order)),
	}
	for _, key := range order {
		result.Rows = append(result.Rows, *rowMap[key])
	}
	return result, nil
}
