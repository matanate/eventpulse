package queue

import (
	"context"
	"time"
)

const (
	StreamName    = "events"
	ConsumerGroup = "workers"
	MaxRetries    = 3
	MinIdleTime   = 30 * time.Second
)

// Message is a single entry read from the Redis Stream.
type Message struct {
	ID            string
	Payload       []byte
	DeliveryCount int64
}

// Consumer reads messages from the stream.
// Read returns new (undelivered) messages; Reclaim returns pending messages
// idle longer than MinIdleTime with delivery counts populated.
type Consumer interface {
	Read(ctx context.Context) ([]Message, error)
	Reclaim(ctx context.Context) ([]Message, error)
	Ack(ctx context.Context, ids ...string) error
}
