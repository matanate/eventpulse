package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// flusherRecorder is a ResponseWriter that also implements http.Flusher.
type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flusherRecorder) Flush() {
	f.flushed = true
}

func TestStatusRecorder_ImplementsFlusher(t *testing.T) {
	// Verify the interface is satisfied at compile time.
	var _ http.Flusher = (*statusRecorder)(nil)
}

func TestStatusRecorder_Flush_DelegatesToUnderlying(t *testing.T) {
	underlying := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	sr := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	sr.Flush()

	if !underlying.flushed {
		t.Fatal("expected Flush() to be forwarded to the underlying ResponseWriter")
	}
}

func TestStatusRecorder_Flush_NoopWhenUnderlyingNotFlusher(t *testing.T) {
	// Plain httptest.ResponseRecorder does not implement http.Flusher in the way
	// we check (it does via a separate method set — but we test the delegation
	// path explicitly here using a non-flusher wrapper).
	type nonFlusher struct{ http.ResponseWriter }
	sr := &statusRecorder{ResponseWriter: nonFlusher{httptest.NewRecorder()}, status: http.StatusOK}

	// Should not panic.
	sr.Flush()
}
