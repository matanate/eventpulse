package ratelimit

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/matanate/eventpulse/internal/api"
	"github.com/matanate/eventpulse/internal/auth"
	"github.com/matanate/eventpulse/internal/telemetry"
)

// sliding window via a Redis sorted set; atomic via Lua so no races.
var slidingWindowScript = redis.NewScript(`
local key      = KEYS[1]
local now_ms   = tonumber(ARGV[1])
local window   = tonumber(ARGV[2])
local limit    = tonumber(ARGV[3])
local member   = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms - window)

local count = tonumber(redis.call('ZCARD', key))
if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local oldest_score = 0
    if #oldest > 0 then oldest_score = tonumber(oldest[2]) end
    return {1, oldest_score}
end

redis.call('ZADD', key, now_ms, member)
redis.call('PEXPIRE', key, window)
return {0, 0}
`)

// FailMode controls what happens when Redis is unavailable.
type FailMode int

const (
	// FailClosed rejects requests when Redis is unavailable (default, safe).
	FailClosed FailMode = iota
	// FailOpen allows requests through when Redis is unavailable (high-availability preference).
	FailOpen
)

// Config holds rate limit parameters.
type Config struct {
	Limit    int
	Window   time.Duration
	FailMode FailMode // default FailClosed
}

// Limiter is a Redis-backed sliding-window rate limiter with a circuit breaker
// that trips after 3 consecutive Redis failures and resets after 10 seconds.
type Limiter struct {
	rdb      *redis.Client
	limit    int
	window   time.Duration
	failMode FailMode
	breaker  *circuitBreaker
}

// NewLimiter creates a Limiter with the given config.
func NewLimiter(rdb *redis.Client, cfg Config) *Limiter {
	l := &Limiter{
		rdb:      rdb,
		limit:    cfg.Limit,
		window:   cfg.Window,
		failMode: cfg.FailMode,
	}
	l.breaker = newCircuitBreaker(func(state int32) {
		telemetry.RateLimiterCircuitBreakerState.Set(float64(state))
	})
	return l
}

// Allow checks whether the given keyID is within the rate limit.
// Returns allowed=false and a retryAfter duration when the limit is exceeded.
// On Redis errors, behaviour depends on FailMode:
//   - FailClosed (default): returns an error, causing the middleware to 500.
//   - FailOpen: returns allowed=true so requests pass through unthrottled.
func (l *Limiter) Allow(ctx context.Context, keyID string) (allowed bool, retryAfter time.Duration, err error) {
	if !l.breaker.allow() {
		// Circuit is open ג€” Redis is (likely) down.
		if l.failMode == FailOpen {
			return true, 0, nil
		}
		return false, 0, fmt.Errorf("rate limiter circuit open")
	}

	now := time.Now()
	nowMS := now.UnixMilli()
	windowMS := l.window.Milliseconds()
	// Use nanoseconds as the member so concurrent requests within the same
	// millisecond each get a distinct ZSET entry; the score (milliseconds)
	// is still what the window range query operates on.
	member := fmt.Sprintf("%d", now.UnixNano())

	res, err := slidingWindowScript.Run(ctx, l.rdb,
		[]string{"rl:" + keyID},
		nowMS, windowMS, l.limit, member,
	).Int64Slice()
	if err != nil {
		l.breaker.recordFailure()
		if l.failMode == FailOpen {
			return true, 0, nil
		}
		return false, 0, fmt.Errorf("rate limit script: %w", err)
	}

	l.breaker.recordSuccess()

	if res[0] == 1 {
		// res[1] is the oldest entry's score in ms
		oldestMS := res[1]
		resetMS := oldestMS + windowMS
		diffMS := resetMS - nowMS
		if diffMS < 0 {
			diffMS = 0
		}
		secs := int64(math.Ceil(float64(diffMS) / 1000))
		return false, time.Duration(secs) * time.Second, nil
	}

	return true, 0, nil
}

// Middleware returns a Chi-compatible middleware that enforces the rate limit
// using the api_key_id injected by the auth middleware.
func (l *Limiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyID, ok := auth.APIKeyIDFromContext(r.Context())
			if !ok {
				// auth middleware didn't run ג€” pass through; handler will reject
				next.ServeHTTP(w, r)
				return
			}

			allowed, retryAfter, err := l.Allow(r.Context(), keyID)
			if err != nil {
				api.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				return
			}
			if !allowed {
				telemetry.RateLimitedRequestsTotal.Inc()
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				api.WriteError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
