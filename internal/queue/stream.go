package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/matanate/eventpulse/internal/events"
	"github.com/matanate/eventpulse/internal/tracing"
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
	values := injectTrace(ctx, map[string]any{"payload": payload})
	if err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.streamName,
		Values: values,
	}).Err(); err != nil {
		return fmt.Errorf("xadd: %w", err)
	}
	return nil
}

func (p *StreamPublisher) PublishBatch(ctx context.Context, evts []*events.Event) error {
	pipe := p.rdb.Pipeline()
	// Extract trace headers once; each event entry gets its own values map.
	traceHeaders := make(map[string]string, 1)
	tracing.InjectMap(ctx, traceHeaders)

	for _, e := range evts {
		payload, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		values := map[string]any{"payload": payload}
		for k, v := range traceHeaders {
			values[k] = v
		}
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: p.streamName,
			Values: values,
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("xadd batch: %w", err)
	}
	return nil
}

// injectTrace returns a new map with all entries from base plus any W3C trace
// headers extracted from ctx. The base map is never modified.
func injectTrace(ctx context.Context, base map[string]any) map[string]any {
	out := make(map[string]any, len(base)+2)
	for k, v := range base {
		out[k] = v
	}
	headers := make(map[string]string, 1)
	tracing.InjectMap(ctx, headers)
	for k, v := range headers {
		out[k] = v
	}
	return out
}
