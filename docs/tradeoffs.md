# EventPulse — Engineering Tradeoffs

Each section follows the format: **Context → Options considered → Choice → Cost / mitigation**.

---

## 1. Async Ingestion (Queue) vs. Synchronous DB Write

**Context**

The ingestion API receives events from client applications. The simplest design writes each event directly to PostgreSQL and returns 200 when committed. We changed this in Phase 3 to return 202 immediately and enqueue into Redis Streams.

**Options considered**

| Option | Latency to client | Throughput ceiling | Failure mode |
|---|---|---|---|
| Synchronous DB write | ~5–20ms (DB round trip) | Bounded by `max_connections` × query rate | Client sees DB errors directly |
| Async via Redis Streams (chosen) | ~1–2ms (XADD) | Bounded by Redis throughput | Client always gets 202; failures visible in worker metrics |
| Async via dedicated message broker (Kafka, SQS) | ~2–5ms | Very high | Operationally heavier |

**Choice: Redis Streams**

Redis was already in the stack for rate limiting, so adding Streams costs zero new infrastructure. XADD is a sub-millisecond append operation — the ingestion API response time is decoupled from PostgreSQL entirely. A traffic spike that would saturate the DB connection pool instead builds a queue that the worker drains at its own pace.

**Cost / mitigation**

- Events are not immediately queryable after the 202 response — analytics has a short lag (typically < 1s on a healthy worker). Documented behaviour; acceptable for an analytics backend.
- Redis is now a mandatory dependency for ingestion. If Redis is unavailable, `Publish` returns an error and the API returns 500. The rate limiter has the same dependency, so Redis going down already breaks the API — no new single point of failure was introduced.
- Message ordering within a stream is preserved per shard; ordering across stream entries is insertion order, which is insertion-time-ordered for our single-stream design.

---

## 2. Redis Sliding Window for Rate Limiting

**Context**

Every API key gets a per-minute request limit (100 req/min in production). We need a counter that resets on a rolling window, not a fixed clock boundary, to avoid the "burst at :00 and :01" problem.

**Options considered**

| Option | State location | Accuracy | Notes |
|---|---|---|---|
| In-memory token bucket | Per process | Exact within process | Breaks with multiple API replicas — each has its own counter |
| Fixed-window counter in Redis | Redis | Approximate (boundary burst) | Simple but allows 2× burst at window boundary |
| Sliding window ZSET in Redis (chosen) | Redis | Exact | Slightly more memory per key; fully atomic via Lua |
| Database row lock | PostgreSQL | Exact | ~5–10ms per check; too slow in the hot path |

**Choice: Redis sliding window via Lua script**

A sorted set keyed on `rl:{api_key_id}` stores request timestamps as scores. The Lua script atomically: removes entries older than the window, checks the count, and adds a new entry if under the limit. The entire check-and-increment is atomic — no TOCTOU race even under high concurrency.

`redis.NewScript()` in go-redis handles the EVALSHA + EVAL fallback automatically; the script is cached server-side after the first call.

**Cost / mitigation**

- Redis is a dependency for every authenticated request. If Redis is down, the middleware returns 500 (fail-closed). A fail-open mode (skip rate limiting on Redis error) could be added but risks abuse during Redis outages.
- Memory per active key: O(requests in window) sorted set entries. At 100 req/min, each ZSET entry is ~20 bytes, so ~2 KB per active key. Negligible.
- `Retry-After` header is set to the exact seconds until the oldest entry expires, giving clients precise backoff timing.

---

## 3. JSONB for Event Properties

**Context**

Events carry a `properties` map of arbitrary key-value pairs. The property schema varies by event type and cannot be predicted at schema design time.

**Options considered**

| Option | Schema flexibility | Query performance | Storage |
|---|---|---|---|
| Fixed columns per property | None — requires schema migration per new property | Excellent (indexed columns) | Optimal |
| JSONB in PostgreSQL (chosen) | Total | Good with GIN index; slow for arbitrary key scans | ~1.2× overhead vs text |
| Separate `event_properties` table | Good | Moderate (join required) | Higher row count |
| ClickHouse columnar store | Good | Excellent for analytics | Requires separate system |

**Choice: JSONB**

JSONB stores and validates JSON natively, supports GIN indexing for key/value containment queries, and requires no schema migrations when new event types are introduced. For v1 at the scale we're targeting, it is the correct tradeoff — the property shapes are unknown at build time.

**Cost / mitigation**

- Ad-hoc queries like `WHERE properties @> '{"plan": "pro"}'` are slow without a GIN index. A GIN index on `properties` was not added in v1 because the query patterns are unknown. Adding it later is a non-blocking `CREATE INDEX CONCURRENTLY`.
- JSONB cannot enforce property types or required keys. Type validation must happen in the client SDK or at ingestion time, not at the DB layer. In v1 we store whatever is sent.
- At very high cardinality (millions of events, hundreds of distinct property keys), a ClickHouse mirror populated via the worker or a CDC pipeline would unlock fast analytical queries without impacting the OLTP Postgres instance.

---

## 4. Worker Crash Recovery

**Context**

The worker processes events from Redis Streams via `XREADGROUP`. If the worker crashes mid-processing (after reading but before `XACK`), those messages must not be lost.

**How Redis Streams handles this**

Redis Streams' Pending Entries List (PEL) tracks unacknowledged messages per consumer. Each message has a delivery count and an idle time (time since last delivery).

**Recovery mechanism in EventPulse**

1. On startup, the worker creates the consumer group with `MKSTREAM` if it doesn't exist (idempotent via BUSYGROUP error suppression).
2. Every 30 seconds, a ticker goroutine calls `XAUTOCLAIM` — any message idle for > 30s is reassigned to the current worker.
3. `XPENDINGEXT` enriches each reclaimed message with its `DeliveryCount`.
4. After `MaxRetries` (3) deliveries, the message is written to `dead_letter_events` and ACK'd — it will never block the queue again.

**What each failure mode looks like**

| Scenario | Outcome |
|---|---|
| Worker panic mid-process | Message stays in PEL; reclaimed by next worker or next reclaim tick |
| Transient DB error | No XACK issued; message redelivered automatically |
| Malformed payload (parse error) | Immediate dead-letter; no retries wasted |
| Redis restart | PEL survives if Redis uses persistence (RDB/AOF); lost if not (acceptable in dev) |

**Cost / mitigation**

- The 30s reclaim window means a crashed worker leaves messages unprocessed for up to 30s before another worker picks them up. This is the latency SLA for crash recovery. Reducing `MinIdleTime` lowers latency at the cost of more false-positive reclaims under normal slow processing.
- `dead_letter_events` has no automatic alerting in v1. In production, a Prometheus alert on `events_failed_total` rate or a periodic count query on `dead_letter_events` would trigger operator attention.

---

## 5. Scaling Paths

**Current architecture is single-instance by design** — one API process, one worker process. Here is how each bottleneck scales horizontally.

### Ingestion API

Stateless — scale by running N replicas behind a load balancer. Each replica shares the same Postgres pool and Redis. Rate limiting works correctly across replicas because all counters are in Redis. No sticky sessions required.

**Bottleneck**: `DB_MAX_CONNS` × replicas must stay under Postgres's `max_connections` (default 100). With 3 replicas at `DB_MAX_CONNS=25`, that's 75 connections — fine. At 10 replicas, use PgBouncer in transaction mode to multiplex connections.

### Worker

Scale by running N worker processes. Each process uses a unique consumer name (hostname-based), so Redis Streams distributes messages across all consumers in the group automatically. No coordination needed.

**Bottleneck**: All workers share the same Postgres pool limit. Raise `DB_MAX_CONNS` per worker instance or add PgBouncer.

### PostgreSQL

The `events` table grows linearly with event volume. Current mitigations:

- Composite indexes on `(project_id, event, created_at)` and `(project_id, user_id, created_at)` keep analytics queries fast.
- When the table exceeds ~100M rows, partition by `(project_id, created_at RANGE)`. PostgreSQL's declarative partitioning allows this without application changes.
- For very high write throughput, TimescaleDB's hypertable or a dedicated time-series store is the upgrade path.

### Analytics at Scale

`daily_event_counts` is the aggregate layer — most dashboard queries read from this table, not `events`. This scales well because it has O(days × projects × event_names) rows regardless of total event volume.

For real-time analytics at > 10k events/s, a ClickHouse mirror populated via Debezium CDC from Postgres would serve analytical queries without competing with OLTP writes.
