package telemetry

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const poolCollectInterval = 15 * time.Second

// StartDBPoolCollector launches a goroutine that publishes pgxpool statistics
// to Prometheus gauges every 15 seconds. It stops when ctx is cancelled.
func StartDBPoolCollector(ctx context.Context, pool *pgxpool.Pool) {
	go func() {
		tick := time.NewTicker(poolCollectInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				collectDBPool(pool)
			}
		}
	}()
}

func collectDBPool(pool *pgxpool.Pool) {
	s := pool.Stat()
	DBPoolAcquiredConnections.Set(float64(s.AcquiredConns()))
	DBPoolIdleConnections.Set(float64(s.IdleConns()))
	// EmptyAcquireCount and AcquireDuration are cumulative since pool creation.
	// Use increase() or irate() in Grafana/PromQL to derive per-interval rates.
	DBPoolWaitCount.Set(float64(s.EmptyAcquireCount()))
	DBPoolWaitDurationSeconds.Set(s.AcquireDuration().Seconds())
}

// StartRedisPoolCollector launches a goroutine that publishes go-redis pool
// statistics to Prometheus gauges every 15 seconds. It stops when ctx is cancelled.
func StartRedisPoolCollector(ctx context.Context, rdb *redis.Client) {
	go func() {
		tick := time.NewTicker(poolCollectInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				collectRedisPool(rdb)
			}
		}
	}()
}

func collectRedisPool(rdb *redis.Client) {
	s := rdb.PoolStats()
	// TotalConns and IdleConns can diverge briefly between internal pool updates,
	// so clamp to zero rather than emitting a negative active count.
	active := int32(s.TotalConns) - int32(s.IdleConns)
	if active < 0 {
		active = 0
	}
	RedisPoolActiveConnections.Set(float64(active))
	RedisPoolIdleConnections.Set(float64(s.IdleConns))
}
