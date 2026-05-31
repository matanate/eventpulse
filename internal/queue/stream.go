package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/matangi/eventpulse/internal/events"
)

// StreamPublisher implements events.Publisher using Redis Streams (XADD).
type StreamPublisher struct {
	rdb        *redis.Client
	streamName string
}

func NewStreamPublisher(rdb *redis.Client) *StreamPublisher {
	return &StreamPublisher{rdb: rdb, streamName: StreamName}
}

// NewStreamPublisherWithStream creates a publisher targeting a custom stream name.
// Intended for testing; production code should use NewStreamPublisher.
func NewStreamPublisherWithStream(rdb *redis.Client, streamName string) *StreamPublisher {
	return &StreamPublisher{rdb: rdb, streamName: streamName}
}

func (p *StreamPublisher) Publish(ctx context.Context, e *events.Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.streamName,
		Values: map[string]any{"payload": payload},
	}).Err(); err != nil {
		return fmt.Errorf("xadd: %w", err)
	}
	return nil
}

func (p *StreamPublisher) PublishBatch(ctx context.Context, evts []*events.Event) error {
	pipe := p.rdb.Pipeline()
	for _, e := range evts {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: p.streamName,
			Values: map[string]any{"payload": payload},
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("xadd batch: %w", err)
	}
	return nil
}
