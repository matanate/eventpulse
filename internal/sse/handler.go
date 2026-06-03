package sse

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/matanate/eventpulse/internal/auth"
)

const (
	channelPrefix = "events:"
	// pubSubBufSize is the size of the Go channel returned by pubsub.Channel().
	// Messages that arrive when the buffer is full are dropped by go-redis (backpressure).
	pubSubBufSize = 64
)

// Handler streams Server-Sent Events for a project's event feed.
type Handler struct {
	redis *redis.Client
}

// NewHandler creates an SSE handler backed by the given Redis client.
func NewHandler(client *redis.Client) *Handler {
	return &Handler{redis: client}
}

// Handle serves GET /v1/projects/{projectID}/stream.
// The client must authenticate via Bearer header or ?api_key= query param
// (EventSource in browsers cannot set custom headers).
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Ensure the URL project matches the authenticated project.
	if chi.URLParam(r, "projectID") != projectID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Clear the server-level WriteTimeout for this long-lived connection.
	// The global timeout is appropriate for normal request/response pairs but
	// would sever SSE connections after every timeout interval.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		slog.Debug("sse: could not clear write deadline", "err", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Disable proxy buffering so bytes reach the browser immediately.
	w.Header().Set("X-Accel-Buffering", "no")

	pubsub := h.redis.Subscribe(r.Context(), channelPrefix+projectID)
	defer func() {
		if err := pubsub.Close(); err != nil {
			slog.Warn("sse: pubsub close error", "err", err)
		}
	}()

	// Send an initial comment to confirm the connection is open.
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ch := pubsub.Channel(redis.WithChannelSize(pubSubBufSize))

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			flusher.Flush()
		}
	}
}
