package webhooks

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{0, time.Second},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{10, 512 * time.Second},
		{100, time.Hour}, // capped
	}
	for _, tt := range tests {
		got := nextBackoff(tt.attempts)
		if got != tt.want {
			t.Errorf("nextBackoff(%d) = %v, want %v", tt.attempts, got, tt.want)
		}
	}
}

func TestSubLimiter_AllowsFirst(t *testing.T) {
	var l subLimiter
	if !l.allow("sub1", 100*time.Millisecond) {
		t.Error("first call should be allowed")
	}
}

func TestSubLimiter_BlocksWithinInterval(t *testing.T) {
	var l subLimiter
	l.allow("sub1", time.Second) // first call marks the time
	if l.allow("sub1", time.Second) {
		t.Error("second call within interval should be blocked")
	}
}

func TestSubLimiter_AllowsAfterInterval(t *testing.T) {
	var l subLimiter
	l.allow("sub1", time.Millisecond) // first call
	time.Sleep(5 * time.Millisecond)
	if !l.allow("sub1", time.Millisecond) {
		t.Error("call after interval should be allowed")
	}
}

func TestSubLimiter_IndependentPerSub(t *testing.T) {
	var l subLimiter
	l.allow("sub1", time.Hour) // block sub1 for 1h
	if !l.allow("sub2", time.Hour) {
		t.Error("sub2 should be independent from sub1")
	}
}
