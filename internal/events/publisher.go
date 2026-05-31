package events

import "context"

// Publisher enqueues events for async processing by the worker.
// Defined here (where it is consumed) so queue package can implement it without circular imports.
type Publisher interface {
	Publish(ctx context.Context, e *Event) error
	PublishBatch(ctx context.Context, events []*Event) error
}
