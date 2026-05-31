package ratelimit

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/matangi/eventpulse/internal/api"
	"github.com/matangi/eventpulse/internal/auth"
	"github.com/matangi/eventpulse/internal/telemetry"
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

// Config holds rate limit parameters.
type Config struct {
	Limit  int
	Window time.Duration
}

// Limiter is a Redis-backed sliding-window rate limiter.
type Limiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

// NewLimiter creates a Limiter with the given config.
func NewLimiter(rdb *redis.Client, cfg Config) *Limiter {
	return &Limiter{
		rdb:    rdb,
		limit:  cfg.Limit,
		window: cfg.Window,
	}
}

// Allow checks whether the given keyID is within the rate limit.
// Returns allowed=false and a retryAfter duration when the limit is exceeded.
func (l *Limiter) Allow(ctx context.Context, keyID string) (allowed bool, retryAfter time.Duration, err error) {
	now := time.Now()
	nowMS := now.UnixMilli()
	windowMS := l.window.Milliseconds()
	member := fmt.Sprintf("%d", nowMS)

	res, err := slidingWindowScript.Run(ctx, l.rdb,
		[]string{"rl:" + keyID},
		nowMS, windowMS, l.limit, member,
	).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit script: %w", err)
	}

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
				// auth middleware didn't run — pass through; handler will reject
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
