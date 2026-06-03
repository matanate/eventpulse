package webhooks

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/matanate/eventpulse/internal/telemetry"
)

// Dispatcher polls webhook_deliveries for due rows and delivers them via HTTP.
// It runs as a long-lived goroutine inside the worker binary.
type Dispatcher struct {
	pool         *pgxpool.Pool
	client       *Client
	pollInterval time.Duration
	batchSize    int
	claimWindow  time.Duration
	minInterval  time.Duration
	secretKey    []byte // AES-256 key for decrypting secrets at delivery time
	limiter      *subLimiter
}

// DispatcherConfig holds tunable parameters for the Dispatcher.
type DispatcherConfig struct {
	PollInterval time.Duration
	BatchSize    int
	HTTPTimeout  time.Duration
	MinInterval  time.Duration // minimum time between delivery attempts per subscription
	AllowHTTP    bool
	SecretKey    []byte // AES-256 key for decrypting webhook secrets at delivery time
}

// NewDispatcher creates a Dispatcher with the given pool and config.
// cfg.SecretKey must be exactly 32 bytes (AES-256); panics otherwise.
func NewDispatcher(pool *pgxpool.Pool, cfg DispatcherConfig) *Dispatcher {
	if len(cfg.SecretKey) != 32 {
		panic("webhooks.NewDispatcher: SecretKey must be exactly 32 bytes")
	}
	client := NewClient(cfg.HTTPTimeout, cfg.AllowHTTP)
	// claimWindow gives enough time for an HTTP delivery plus a buffer before a
	// claimed row is considered abandoned and becomes reclaimable.
	claimWindow := cfg.HTTPTimeout*2 + 30*time.Second
	return &Dispatcher{
		pool:         pool,
		client:       client,
		pollInterval: cfg.PollInterval,
		batchSize:    cfg.BatchSize,
		claimWindow:  claimWindow,
		minInterval:  cfg.MinInterval,
		secretKey:    cfg.SecretKey,
		limiter:      &subLimiter{},
	}
}

// Run polls for due deliveries on each tick until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.processBatch(ctx)
		}
	}
}

func (d *Dispatcher) processBatch(ctx context.Context) {
	deliveries, err := ClaimDueDeliveries(ctx, d.pool, d.batchSize, d.claimWindow)
	if err != nil {
		slog.Error("dispatcher: claim deliveries", "err", err)
		return
	}

	// Update the queue-depth gauge from the real pending count, not the
	// claimed batch size (which is bounded by batchSize and would hide backlogs).
	if n, err := pendingCount(ctx, d.pool); err == nil {
		telemetry.WebhookPendingDeliveries.Set(float64(n))
	}

	for _, del := range deliveries {
		d.attempt(ctx, del)
	}
}

func (d *Dispatcher) attempt(ctx context.Context, del DeliveryWithSub) {
	// Per-subscription rate limit: skip (leave claimed) if delivered too recently.
	// The claim window ensures the row resurfaces after claimWindow elapses.
	if !d.limiter.allow(del.SubscriptionID, d.minInterval) {
		telemetry.WebhookDeliveriesTotal.WithLabelValues("skipped").Inc()
		slog.Info("dispatcher: rate-limited, skipping", "sub_id", del.SubscriptionID, "delivery_id", del.ID)
		return
	}

	// Decrypt the HMAC signing secret stored encrypted in the database.
	plainSecret, err := DecryptSecret(del.Secret, d.secretKey)
	if err != nil {
		slog.Error("dispatcher: decrypt secret", "err", err, "sub_id", del.SubscriptionID)
		errMsg := "secret decryption failed"
		if del.Attempts+1 >= MaxAttempts {
			_ = RescheduleOrFail(ctx, d.pool, del.ID, del.Attempts+1, errMsg, time.Now(), true)
		} else {
			_ = RescheduleOrFail(ctx, d.pool, del.ID, del.Attempts+1, errMsg,
				time.Now().Add(nextBackoff(del.Attempts+1)), false)
		}
		return
	}
	// Zero the plaintext slice after use to limit the window of exposure in heap memory.
	defer func() {
		for i := range plainSecret {
			plainSecret[i] = 0
		}
	}()
	del.Secret = string(plainSecret)

	start := time.Now()
	result := d.client.Deliver(ctx, del)
	duration := time.Since(start)

	newAttempts := del.Attempts + 1

	if result.Delivered {
		telemetry.WebhookDeliveriesTotal.WithLabelValues("delivered").Inc()
		telemetry.WebhookDeliveryDurationSeconds.Observe(duration.Seconds())
		if err := MarkDelivered(ctx, d.pool, del.ID); err != nil {
			slog.Error("dispatcher: mark delivered", "err", err, "delivery_id", del.ID)
		}
		slog.Info("dispatcher: delivered", "sub_id", del.SubscriptionID, "delivery_id", del.ID, "status", result.StatusCode)
		return
	}

	// Sanitize the raw error before storing so internal IP addresses and
	// network topology are never persisted or surfaced via the database.
	errMsg := sanitizeDeliveryError(result.Err)

	if newAttempts >= MaxAttempts {
		telemetry.WebhookDeliveriesTotal.WithLabelValues("failed").Inc()
		if err := RescheduleOrFail(ctx, d.pool, del.ID, newAttempts, errMsg, time.Now(), true); err != nil {
			slog.Error("dispatcher: mark failed", "err", err, "delivery_id", del.ID)
		}
		slog.Warn("dispatcher: permanently failed", "sub_id", del.SubscriptionID, "delivery_id", del.ID, "err", errMsg, "attempts", newAttempts)
		return
	}

	nextRetry := time.Now().Add(nextBackoff(newAttempts))
	telemetry.WebhookDeliveriesTotal.WithLabelValues("retried").Inc()
	if err := RescheduleOrFail(ctx, d.pool, del.ID, newAttempts, errMsg, nextRetry, false); err != nil {
		slog.Error("dispatcher: reschedule", "err", err, "delivery_id", del.ID)
	}
	slog.Warn("dispatcher: delivery failed, rescheduled",
		"sub_id", del.SubscriptionID, "delivery_id", del.ID,
		"err", errMsg, "attempts", newAttempts, "next_retry", nextRetry,
	)
}

// sanitizeDeliveryError maps raw Go network/SSRF error strings to safe generic
// messages so internal IP addresses and topology are never stored in last_error.
func sanitizeDeliveryError(raw string) string {
	if raw == "" {
		return "non-2xx response"
	}
	if strings.Contains(raw, "SSRF protection") {
		return "webhook URL resolves to a blocked address"
	}
	if strings.Contains(raw, "URL validation") {
		return "invalid webhook URL"
	}
	if strings.Contains(raw, "context deadline exceeded") || strings.Contains(raw, "timeout") {
		return "delivery timed out"
	}
	return "network error"
}

// pendingCount returns the total number of pending delivery rows.
func pendingCount(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM webhook_deliveries WHERE status = 'pending'`,
	).Scan(&n)
	return n, err
}

// subLimiter is a simple in-process rate limiter keyed on subscription ID.
type subLimiter struct {
	mu          sync.Mutex
	lastAttempt map[string]time.Time
}

const subLimiterTTL = 10 * time.Minute

func (l *subLimiter) allow(subID string, minInterval time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastAttempt == nil {
		l.lastAttempt = make(map[string]time.Time)
	}
	now := time.Now()
	if last, ok := l.lastAttempt[subID]; ok && now.Sub(last) < minInterval {
		return false
	}
	l.lastAttempt[subID] = now
	// Evict stale entries to prevent unbounded growth across long-running processes.
	for k, v := range l.lastAttempt {
		if now.Sub(v) > subLimiterTTL {
			delete(l.lastAttempt, k)
		}
	}
	return true
}

