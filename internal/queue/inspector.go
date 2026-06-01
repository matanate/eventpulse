package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Inspector provides read-only visibility into the stream's queue state.
// Used by monitoring endpoints; the Worker uses StreamConsumer.PendingCount directly.
type Inspector struct {
	rdb *redis.Client
}

func NewInspector(rdb *redis.Client) *Inspector {
	return &Inspector{rdb: rdb}
}

// PendingCount returns the number of messages delivered to consumers but not yet ACKed.
// Returns 0 (not an error) when the consumer group does not yet exist.
func (i *Inspector) PendingCount(ctx context.Context) (int64, error) {
	info, err := i.rdb.XPending(ctx, StreamName, ConsumerGroup).Result()
	if err != nil {
		if strings.Contains(err.Error(), "NOGROUP") {
			return 0, nil
		}
		return 0, fmt.Errorf("xpending: %w", err)
	}
	return info.Count, nil
}
