package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

// fakeRedis implements just enough of the Redis interface to drive Allow().
// It returns a fixed error on every call when errMode is true.
type fakeRedis struct {
	errMode bool
}

// We can't inject fakeRedis through the public API because Limiter holds a
// concrete *redis.Client. Instead we test the circuit breaker directly and
// test FailMode semantics via the internalLimiter wrapper below.
// NOTE: FailMode behavior in the production Limiter.Allow is NOT directly
// covered by these unit tests — only the internalLimiter proxy is exercised.
// Integration tests against a real Redis would be needed for full coverage.

// ─── Circuit breaker unit tests ───────────────────────────────────────────────

func TestCircuitBreaker_StartsClose(t *testing.T) {
	cb := newCircuitBreaker(nil)
	if !cb.allow() {
		t.Fatal("new breaker should allow requests")
	}
	if cb.state.Load() != stateClosed {
		t.Fatalf("expected state=closed, got %d", cb.state.Load())
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	var notified int32
	cb := newCircuitBreaker(func(s int32) { notified = s })

	for i := 0; i < failureThreshold; i++ {
		cb.recordFailure()
	}

	if cb.state.Load() != stateOpen {
		t.Fatalf("expected state=open after %d failures, got %d", failureThreshold, cb.state.Load())
	}
	if notified != stateOpen {
		t.Fatalf("expected notification=open, got %d", notified)
	}
}

func TestCircuitBreaker_BlocksWhenOpen(t *testing.T) {
	cb := newCircuitBreaker(nil)
	for i := 0; i < failureThreshold; i++ {
		cb.recordFailure()
	}
	if cb.allow() {
		t.Fatal("open breaker should not allow requests immediately")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterReset(t *testing.T) {
	cb := newCircuitBreaker(nil)
	for i := 0; i < failureThreshold; i++ {
		cb.recordFailure()
	}
	// Wind the clock back so openedAt looks old enough.
	past := time.Now().Add(-(resetAfter + time.Millisecond)).UnixNano()
	cb.openedAt.Store(past)

	if !cb.allow() {
		t.Fatal("breaker should transition to half-open and allow a probe")
	}
	if cb.state.Load() != stateHalfOpen {
		t.Fatalf("expected state=half-open, got %d", cb.state.Load())
	}
}

func TestCircuitBreaker_ClosesOnSuccessFromHalfOpen(t *testing.T) {
	cb := newCircuitBreaker(nil)
	for i := 0; i < failureThreshold; i++ {
		cb.recordFailure()
	}
	past := time.Now().Add(-(resetAfter + time.Millisecond)).UnixNano()
	cb.openedAt.Store(past)
	cb.allow() // transition to half-open

	cb.recordSuccess()

	if cb.state.Load() != stateClosed {
		t.Fatalf("expected state=closed after success from half-open, got %d", cb.state.Load())
	}
}

func TestCircuitBreaker_ReOpensOnFailureFromHalfOpen(t *testing.T) {
	cb := newCircuitBreaker(nil)
	for i := 0; i < failureThreshold; i++ {
		cb.recordFailure()
	}
	past := time.Now().Add(-(resetAfter + time.Millisecond)).UnixNano()
	cb.openedAt.Store(past)
	cb.allow() // transition to half-open

	cb.recordFailure()

	if cb.state.Load() != stateOpen {
		t.Fatalf("expected state=open after failure from half-open, got %d", cb.state.Load())
	}
}

// ─── FailMode tests via an injectable script runner ───────────────────────────

// scriptRunner is the narrow interface we need to call the Lua script.
type scriptRunner interface {
	run(ctx context.Context, key, nowMS, windowMS, limit, member string) ([]int64, error)
}

// failingRunner always returns an error, simulating Redis unavailability.
type failingRunner struct{}

func (failingRunner) run(_ context.Context, _, _, _, _, _ string) ([]int64, error) {
	return nil, errors.New("redis: connection refused")
}

// internalLimiter mirrors Limiter but accepts a scriptRunner for testing.
type internalLimiter struct {
	limit    int
	window   time.Duration
	failMode FailMode
	breaker  *circuitBreaker
	runner   scriptRunner
}

func newInternalLimiter(runner scriptRunner, cfg Config) *internalLimiter {
	l := &internalLimiter{
		limit:    cfg.Limit,
		window:   cfg.Window,
		failMode: cfg.FailMode,
		runner:   runner,
	}
	l.breaker = newCircuitBreaker(nil)
	return l
}

func (l *internalLimiter) allow(ctx context.Context, keyID string) (bool, time.Duration, error) {
	if !l.breaker.allow() {
		if l.failMode == FailOpen {
			return true, 0, nil
		}
		return false, 0, errors.New("circuit open")
	}
	now := time.Now()
	_, err := l.runner.run(ctx,
		"rl:"+keyID,
		formatMS(now.UnixMilli()),
		formatMS(l.window.Milliseconds()),
		formatMS(int64(l.limit)),
		formatMS(now.UnixMilli()),
	)
	if err != nil {
		l.breaker.recordFailure()
		if l.failMode == FailOpen {
			return true, 0, nil
		}
		return false, 0, err
	}
	l.breaker.recordSuccess()
	return true, 0, nil
}

func formatMS(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestFailClosed_ReturnsErrorOnRedisFailure(t *testing.T) {
	l := newInternalLimiter(failingRunner{}, Config{Limit: 100, Window: time.Minute, FailMode: FailClosed})
	_, _, err := l.allow(context.Background(), "key1")
	if err == nil {
		t.Fatal("FailClosed: expected error when Redis is down")
	}
}

func TestFailOpen_AllowsRequestsOnRedisFailure(t *testing.T) {
	l := newInternalLimiter(failingRunner{}, Config{Limit: 100, Window: time.Minute, FailMode: FailOpen})
	allowed, _, err := l.allow(context.Background(), "key1")
	if err != nil {
		t.Fatalf("FailOpen: unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("FailOpen: expected request to be allowed when Redis is down")
	}
}

func TestFailOpen_AllowsWhenBreakerIsOpen(t *testing.T) {
	l := newInternalLimiter(failingRunner{}, Config{Limit: 100, Window: time.Minute, FailMode: FailOpen})
	// Trip the breaker.
	for i := 0; i < failureThreshold; i++ {
		l.breaker.recordFailure()
	}
	allowed, _, err := l.allow(context.Background(), "key1")
	if err != nil {
		t.Fatalf("FailOpen: unexpected error with open breaker: %v", err)
	}
	if !allowed {
		t.Fatal("FailOpen: expected request to be allowed when breaker is open")
	}
}

func TestFailClosed_BlocksWhenBreakerIsOpen(t *testing.T) {
	l := newInternalLimiter(failingRunner{}, Config{Limit: 100, Window: time.Minute, FailMode: FailClosed})
	// Trip the breaker.
	for i := 0; i < failureThreshold; i++ {
		l.breaker.recordFailure()
	}
	_, _, err := l.allow(context.Background(), "key1")
	if err == nil {
		t.Fatal("FailClosed: expected error when breaker is open")
	}
}
