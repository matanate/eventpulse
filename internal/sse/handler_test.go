package sse

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matanate/eventpulse/internal/auth"
)

// stubHandler replaces the Redis subscription with an in-process channel.
// It allows tests to push messages and verify SSE output without a real Redis.
type stubHandler struct {
	msgs chan string
}

func newStubHandler() *stubHandler {
	return &stubHandler{msgs: make(chan string, 8)}
}

// handle mirrors Handler.Handle but reads from msgs instead of Redis.
func (s *stubHandler) handle(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.PathValue("projectID") != projectID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-s.msgs:
			_, _ = w.Write([]byte("data: " + msg + "\n\n"))
			flusher.Flush()
		}
	}
}

func TestHandle_HeadersSet(t *testing.T) {
	h := newStubHandler()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ctx = auth.WithProjectID(ctx, "proj-1")

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/projects/proj-1/stream", nil)
	req.SetPathValue("projectID", "proj-1")
	rec := httptest.NewRecorder()

	h.handle(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestHandle_StreamsMessages(t *testing.T) {
	h := newStubHandler()

	ctx, cancel := context.WithCancel(context.Background())
	ctx = auth.WithProjectID(ctx, "proj-1")

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/projects/proj-1/stream", nil)
	req.SetPathValue("projectID", "proj-1")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.handle(rec, req)
	}()

	h.msgs <- `{"event":"click"}`
	h.msgs <- `{"event":"view"}`

	// Give the goroutine time to flush both messages then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `data: {"event":"click"}`) {
		t.Errorf("expected click event in body, got: %q", body)
	}
	if !strings.Contains(body, `data: {"event":"view"}`) {
		t.Errorf("expected view event in body, got: %q", body)
	}
}

func TestHandle_SSEFormat(t *testing.T) {
	h := newStubHandler()

	ctx, cancel := context.WithCancel(context.Background())
	ctx = auth.WithProjectID(ctx, "proj-1")

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/projects/proj-1/stream", nil)
	req.SetPathValue("projectID", "proj-1")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.handle(rec, req)
	}()

	h.msgs <- `{"id":"e1","event":"click"}`
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	// Verify each SSE message is terminated by a blank line.
	scanner := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"click"`) {
			// Next line must be empty (SSE double-newline separator)
			if i+1 < len(lines) && lines[i+1] == "" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("SSE message not followed by blank line; body: %q", rec.Body.String())
	}
}

func TestHandle_UnauthorizedWithoutProjectID(t *testing.T) {
	h := newStubHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/stream", nil)
	req.SetPathValue("projectID", "proj-1")
	rec := httptest.NewRecorder()

	h.handle(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandle_ForbiddenWhenProjectMismatch(t *testing.T) {
	h := newStubHandler()

	ctx := auth.WithProjectID(context.Background(), "proj-other")
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/projects/proj-1/stream", nil)
	req.SetPathValue("projectID", "proj-1")
	rec := httptest.NewRecorder()

	h.handle(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
