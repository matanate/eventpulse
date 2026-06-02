package ratelimit

import (
	"sync/atomic"
	"time"
)

// breakerState encodes the circuit breaker state machine.
// Transitions:
//
//	closed   ──(3 consecutive failures)──► open
//	open     ──(resetAfter elapsed)──────► half-open
//	half-open ──(success)───────────────► closed
//	half-open ──(failure)───────────────► open
//
// Half-open allows all concurrent callers through (not just one probe). This is
// intentional for a rate-limiter context: Redis recovering under light load is
// preferable to a strict single-probe gate that only resets at the next tick.
const (
	stateClosed   int32 = 0
	stateHalfOpen int32 = 1
	stateOpen     int32 = 2
)

const (
	failureThreshold = 3
	resetAfter       = 10 * time.Second
)

type circuitBreaker struct {
	state         atomic.Int32
	failures      atomic.Int32
	openedAt      atomic.Int64 // Unix nanoseconds when the breaker last opened
	onStateChange func(state int32)
}

func newCircuitBreaker(onStateChange func(state int32)) *circuitBreaker {
	return &circuitBreaker{onStateChange: onStateChange}
}

// allow returns true if the call should proceed (breaker is closed or half-open).
func (cb *circuitBreaker) allow() bool {
	switch cb.state.Load() {
	case stateClosed:
		return true
	case stateOpen:
		if time.Duration(time.Now().UnixNano()-cb.openedAt.Load()) >= resetAfter {
			// Transition to half-open: let requests probe recovery.
			if cb.state.CompareAndSwap(stateOpen, stateHalfOpen) {
				cb.notify(stateHalfOpen)
			}
			return true
		}
		return false
	case stateHalfOpen:
		return true
	}
	return true
}

// recordSuccess transitions the breaker back to closed. Uses CAS so a concurrent
// recordFailure from a different in-flight request cannot be silently overwritten.
func (cb *circuitBreaker) recordSuccess() {
	if cb.state.CompareAndSwap(stateHalfOpen, stateClosed) {
		cb.failures.Store(0)
		cb.notify(stateClosed)
	}
	// If already closed, nothing to do. If open (unusual: success raced with a
	// very fast re-open), leave the open state intact so the reset timer governs.
}

// recordFailure increments the failure counter and trips the breaker when the
// threshold is reached, or immediately if the breaker is already half-open.
// The CAS inside the trip block ensures only one goroutine performs the
// transition; failures are reset to 0 atomically inside the winner.
func (cb *circuitBreaker) recordFailure() {
	if cb.state.Load() == stateHalfOpen {
		// Any failure from half-open immediately re-opens.
		if cb.state.CompareAndSwap(stateHalfOpen, stateOpen) {
			cb.failures.Store(0)
			cb.openedAt.Store(time.Now().UnixNano())
			cb.notify(stateOpen)
		}
		return
	}

	if cb.failures.Add(1) >= failureThreshold {
		if cb.state.CompareAndSwap(stateClosed, stateOpen) {
			cb.failures.Store(0)
			cb.openedAt.Store(time.Now().UnixNano())
			cb.notify(stateOpen)
		} else {
			// Another goroutine already opened the breaker; reset our increment
			// so the counter does not accumulate past 0 after the next close.
			cb.failures.Store(0)
		}
	}
}

func (cb *circuitBreaker) notify(s int32) {
	if cb.onStateChange != nil {
		cb.onStateChange(s)
	}
}

