package analytics

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	minFunnelSteps  = 2
	maxFunnelSteps  = 8
	maxFunnelWindow = 90 * 24 * time.Hour
)

var (
	reFunnelDays  = regexp.MustCompile(`^P(\d+)D$`)
	reFunnelWeeks = regexp.MustCompile(`^P(\d+)W$`)
)

// FunnelParams controls a funnel query.
type FunnelParams struct {
	Steps  []string      // ordered event names, 2–8 entries
	Window time.Duration // 1–90 days; each step must occur within this duration of the previous
}

// FunnelStep is one step in the funnel response.
type FunnelStep struct {
	Event          string  `json:"event"`
	Entered        int64   `json:"entered"`
	Converted      int64   `json:"converted"`
	Dropped        int64   `json:"dropped"`
	ConversionRate float64 `json:"conversion_rate"` // converted/entered; 0 for last step
}

// FunnelResult is the response for a funnel query.
type FunnelResult struct {
	Steps                 []FunnelStep `json:"steps"`
	Window                string       `json:"window"`
	OverallConversionRate float64      `json:"overall_conversion_rate"`
}

// ParseFunnelWindow parses an ISO 8601 duration string (PXD or PXW) into a time.Duration.
// Valid range: 1–90 days.
func ParseFunnelWindow(s string) (time.Duration, error) {
	if m := reFunnelDays.FindStringSubmatch(s); m != nil {
		days, _ := strconv.Atoi(m[1])
		return validateFunnelWindow(days)
	}
	if m := reFunnelWeeks.FindStringSubmatch(s); m != nil {
		weeks, _ := strconv.Atoi(m[1])
		return validateFunnelWindow(weeks * 7)
	}
	return 0, fmt.Errorf("invalid window %q: use PXD or PXW (e.g. P7D, P4W)", s)
}

func validateFunnelWindow(days int) (time.Duration, error) {
	if days < 1 || days > 90 {
		return 0, fmt.Errorf("window must be 1–90 days, got %d", days)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// Funnel runs a strict-order, step-to-step windowed funnel query.
// Each step must be performed by the same user_id within p.Window after the previous step.
// Events with a NULL user_id are excluded from all steps.
//
// The query dynamically builds N CTEs — one per step — using the existing
// (project_id, user_id, timestamp) composite index for efficient joins.
func Funnel(ctx context.Context, pool *pgxpool.Pool, projectID string, p FunnelParams) (FunnelResult, error) {
	n := len(p.Steps)
	// param layout: $1=project_id, $2..$(n+1)=event names, $(n+2)=window interval
	windowIdx := n + 2

	args := make([]any, 0, n+2)
	args = append(args, projectID)
	for _, s := range p.Steps {
		args = append(args, s)
	}
	args = append(args, p.Window)

	var b strings.Builder

	// step_0: entry cohort — all users who triggered the first event.
	b.WriteString("WITH step_0 AS (\n")
	b.WriteString("    SELECT user_id, MIN(timestamp) AS ts FROM events\n")
	b.WriteString("    WHERE project_id = $1 AND event = $2 AND user_id IS NOT NULL\n")
	b.WriteString("    GROUP BY user_id\n)")

	// step_1 .. step_{n-1}: each chains from the previous step.
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b,
			",\nstep_%d AS (\n    SELECT s.user_id, MIN(e.timestamp) AS ts\n"+
				"    FROM step_%d s\n"+
				"    JOIN events e ON e.project_id = $1 AND e.event = $%d\n"+
				"        AND e.user_id = s.user_id\n"+
				"        AND e.timestamp > s.ts AND e.timestamp <= s.ts + $%d\n"+
				"    GROUP BY s.user_id\n)",
			i, i-1, i+2, windowIdx)
	}

	// Final SELECT: one COUNT per step.
	b.WriteString("\nSELECT\n    ")
	cols := make([]string, n)
	for i := range cols {
		cols[i] = fmt.Sprintf("COUNT(s%d.user_id) AS c%d", i, i)
	}
	b.WriteString(strings.Join(cols, ",\n    "))
	b.WriteString("\nFROM step_0 s0")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "\nLEFT JOIN step_%d s%d ON s%d.user_id = s0.user_id", i, i, i)
	}

	counts := make([]int64, n)
	dest := make([]any, n)
	for i := range counts {
		dest[i] = &counts[i]
	}
	if err := pool.QueryRow(ctx, b.String(), args...).Scan(dest...); err != nil {
		return FunnelResult{}, fmt.Errorf("funnel query: %w", err)
	}

	steps := make([]FunnelStep, n)
	for i, entered := range counts {
		fs := FunnelStep{
			Event:   p.Steps[i],
			Entered: entered,
		}
		if i < n-1 {
			converted := counts[i+1]
			fs.Converted = converted
			fs.Dropped = entered - converted
			if entered > 0 {
				fs.ConversionRate = float64(converted) / float64(entered)
			}
		}
		steps[i] = fs
	}

	var overall float64
	if counts[0] > 0 {
		overall = float64(counts[n-1]) / float64(counts[0])
	}

	return FunnelResult{
		Steps:                 steps,
		Window:                durationToISO(p.Window),
		OverallConversionRate: overall,
	}, nil
}

// durationToISO renders a day-aligned duration as ISO 8601 (e.g. P7D).
// Callers must ensure d is an exact multiple of 24h — guaranteed by ParseFunnelWindow.
func durationToISO(d time.Duration) string {
	return fmt.Sprintf("P%dD", int(d.Hours()/24))
}
