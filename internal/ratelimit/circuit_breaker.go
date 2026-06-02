package ratelimit

import (
	"sync/atomic"
	"time"
)

// breakerState encodes the circuit breaker state machine.
// Transitions:
//
//	closed ──(3 consecutive failures)──► open
//	open   ──(resetAfter elapsed)──────► half-open
//	half-open ──(success)──────────────► closed
//	half-open ──(failure)──────────────► open
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
	state       atomic.Int32
	failures    atomic.Int32
	openedAt    atomic.Int64 // Unix nanoseconds when the breaker last opened
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
			// Transition to half-open: let one probe through.
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

// recordSuccess resets the breaker to closed on a successful call.
func (cb *circuitBreaker) recordSuccess() {
	if cb.state.Load() != stateClosed {
		cb.failures.Store(0)
		cb.state.Store(stateClosed)
		cb.notify(stateClosed)
	}
}

// recordFailure increments the failure counter and trips the breaker when the
// threshold is reached.
func (cb *circuitBreaker) recordFailure() {
	if cb.failures.Add(1) >= failureThreshold || cb.state.Load() == stateHalfOpen {
		if cb.state.CompareAndSwap(stateClosed, stateOpen) ||
			cb.state.CompareAndSwap(stateHalfOpen, stateOpen) {
			cb.openedAt.Store(time.Now().UnixNano())
			cb.failures.Store(0)
			cb.notify(stateOpen)
		}
	}
}

func (cb *circuitBreaker) notify(s int32) {
	if cb.onStateChange != nil {
		cb.onStateChange(s)
	}
}
