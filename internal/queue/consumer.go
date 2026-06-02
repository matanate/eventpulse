package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// StreamConsumer implements Consumer using Redis Streams (XREADGROUP / XAUTOCLAIM / XACK).
type StreamConsumer struct {
	rdb          *redis.Client
	consumerName string
	streamName   string
}

func NewStreamConsumer(rdb *redis.Client, consumerName string) (*StreamConsumer, error) {
	return NewStreamConsumerWithStream(rdb, consumerName, StreamName)
}

// NewStreamConsumerWithStream creates a consumer targeting a custom stream name.
// Intended for testing; production code should use NewStreamConsumer.
func NewStreamConsumerWithStream(rdb *redis.Client, consumerName, streamName string) (*StreamConsumer, error) {
	c := &StreamConsumer{rdb: rdb, consumerName: consumerName, streamName: streamName}
	if err := c.ensureGroup(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

// ensureGroup creates the consumer group if it does not exist.
// MKSTREAM creates the stream itself if it also does not exist.
// "0" starts from the beginning of the stream so a fresh consumer group
// processes any backlog rather than silently skipping it ("$" would
// start at the tail and lose all messages published before the group existed).
func (c *StreamConsumer) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.streamName, ConsumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

// Read returns up to 10 new (undelivered) messages, blocking up to 2 seconds.
func (c *StreamConsumer) Read(ctx context.Context) ([]Message, error) {
	res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ConsumerGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.streamName, ">"},
		Count:    10,
		Block:    2 * time.Second,
	}).Result()
	if err != nil {
		// redis.Nil means the blocking call timed out with no messages — not an error.
		if err == redis.Nil {
			return nil, nil
		}
		// context cancelled during block
		if ctx.Err() != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("xreadgroup: %w", err)
	}
	if len(res) == 0 {
		return nil, nil
	}
	return toMessages(res[0].Messages, 0), nil
}

// Reclaim picks up pending messages idle longer than MinIdleTime and populates
// DeliveryCount for each by querying XPENDINGEXT.
func (c *StreamConsumer) Reclaim(ctx context.Context) ([]Message, error) {
	xmsgs, _, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   c.streamName,
		Group:    ConsumerGroup,
		Consumer: c.consumerName,
		MinIdle:  MinIdleTime,
		Start:    "0-0",
		Count:    10,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("xautoclaim: %w", err)
	}
	if len(xmsgs) == 0 {
		return nil, nil
	}

	// Collect IDs to query delivery counts in one call.
	ids := make([]string, len(xmsgs))
	for i, m := range xmsgs {
		ids[i] = m.ID
	}

	counts, err := c.deliveryCounts(ctx, ids)
	if err != nil {
		// Non-fatal: process without accurate counts.
		counts = make(map[string]int64)
	}

	msgs := make([]Message, 0, len(xmsgs))
	for _, m := range xmsgs {
		payload, _ := extractPayload(m.Values)
		msgs = append(msgs, Message{
			ID:            m.ID,
			Payload:       payload,
			DeliveryCount: counts[m.ID],
			Headers:       extractTraceHeaders(m.Values),
		})
	}
	return msgs, nil
}

// PendingCount returns the total number of unacknowledged messages in the consumer group.
func (c *StreamConsumer) PendingCount(ctx context.Context) (int64, error) {
	info, err := c.rdb.XPending(ctx, c.streamName, ConsumerGroup).Result()
	if err != nil {
		return 0, fmt.Errorf("xpending: %w", err)
	}
	return info.Count, nil
}

// Ack acknowledges one or more message IDs.
func (c *StreamConsumer) Ack(ctx context.Context, ids ...string) error {
	if err := c.rdb.XAck(ctx, c.streamName, ConsumerGroup, ids...).Err(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	return nil
}

// deliveryCounts queries XPENDINGEXT for the given message IDs and returns a map of id → count.
func (c *StreamConsumer) deliveryCounts(ctx context.Context, ids []string) (map[string]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.streamName,
		Group:  ConsumerGroup,
		Start:  ids[0],
		End:    ids[len(ids)-1],
		Count:  int64(len(ids)),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("xpendingext: %w", err)
	}
	out := make(map[string]int64, len(pending))
	for _, p := range pending {
		out[p.ID] = p.RetryCount
	}
	return out, nil
}

func toMessages(xmsgs []redis.XMessage, deliveryCount int64) []Message {
	out := make([]Message, 0, len(xmsgs))
	for _, m := range xmsgs {
		payload, _ := extractPayload(m.Values)
		out = append(out, Message{
			ID:            m.ID,
			Payload:       payload,
			DeliveryCount: deliveryCount,
			Headers:       extractTraceHeaders(m.Values),
		})
	}
	return out
}

func extractPayload(values map[string]any) ([]byte, bool) {
	v, ok := values["payload"]
	if !ok {
		return nil, false
	}
	switch t := v.(type) {
	case string:
		return []byte(t), true
	case []byte:
		return t, true
	}
	return nil, false
}

// extractTraceHeaders copies W3C trace headers (traceparent, tracestate) from
// stream message values into a string map for use with tracing.ExtractMap.
func extractTraceHeaders(values map[string]any) map[string]string {
	headers := make(map[string]string, 2)
	for _, key := range []string{"traceparent", "tracestate"} {
		if v, ok := values[key]; ok {
			switch s := v.(type) {
			case string:
				headers[key] = s
			case []byte:
				headers[key] = string(s)
			}
		}
	}
	return headers
}
