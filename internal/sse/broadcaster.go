package sse

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// RedisBroadcaster implements events.Broadcaster using Redis pub/sub.
type RedisBroadcaster struct {
	client *redis.Client
}

// NewBroadcaster returns a RedisBroadcaster for the given client.
func NewBroadcaster(client *redis.Client) *RedisBroadcaster {
	return &RedisBroadcaster{client: client}
}

// Broadcast publishes payload to channel. The call is fire-and-forget; errors
// are logged at debug level so failures are visible in traces without noise.
func (b *RedisBroadcaster) Broadcast(ctx context.Context, channel, payload string) {
	if err := b.client.Publish(ctx, channel, payload).Err(); err != nil {
		slog.Debug("sse: broadcast failed", "channel", channel, "err", err)
	}
}
