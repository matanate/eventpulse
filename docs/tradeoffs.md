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

## 5. Funnel Analysis

**Context**

The funnel query must identify users who performed an ordered sequence of events. The schema has no explicit session concept, only `(project_id, user_id, event, timestamp)` rows. Two design choices significantly affect query performance and semantics: window measurement and SQL shape.

**Window semantics**

| Option | Meaning | Tradeoff |
|---|---|---|
| Session window (from step 1) | All steps must occur within N days of the first event | Stricter; fairer cross-funnel comparison |
| Step-to-step window (chosen) | Each step must occur within N days of the previous step | Simpler SQL; allows slow funnels to still convert |

**Choice: step-to-step window**

Each CTE starts from the previous step's `MIN(timestamp)`, so the window resets at each step. A user who takes 6 days between step 1→2 and another 6 days between step 2→3 satisfies a P7D window. A session window would require carrying `step_0.ts` through every CTE join, adding complexity with no material benefit at the scale this system targets.

**SQL shape**

The query builds N CTEs dynamically — one per step. Each CTE is a self-join on the `events` table keyed by `(project_id, user_id, timestamp)`:

```sql
step_1 AS (
    SELECT s.user_id, MIN(e.timestamp) AS ts
    FROM step_0 s
    JOIN events e ON e.project_id = $1 AND e.event = $step1
        AND e.user_id = s.user_id
        AND e.timestamp > s.ts AND e.timestamp <= s.ts + $window
    GROUP BY s.user_id
)
```

The `(project_id, user_id, timestamp DESC) WHERE user_id IS NOT NULL` index supports every step join. For a 3-step funnel, the planner performs 2 nested-loop index scans over the user cohort from the previous step — typically O(users × log(events_per_user)).

**Caps and exclusions**

- 2–8 steps: prevents quadratic query growth.
- 90-day maximum window: keeps the cohort from expanding to all-time data.
- `NULL user_id` events are excluded: funnels are user-journey analysis; anonymous events have no identity to chain across steps.

---

## 6. Retention / Cohort Analysis

**Context**

A retention heatmap groups users into cohorts by the first day they appear, then tracks how many of those users returned on each subsequent day. The challenge is doing this efficiently without full table scans on `events`.

**Rollup strategy: `daily_active_users`**

Rather than computing "first seen" and "returned" directly from the `events` table (which grows unboundedly), the worker upserts one row per `(project_id, date, user_id)` into `daily_active_users` after each event. This is an `INSERT ON CONFLICT DO NOTHING` — idempotent and O(1) per event.

The retention query then runs against this rollup table, which has O(users × days) rows — dramatically smaller than `events`. At 1,000 daily active users and 90 days, that is ≤ 90,000 rows. The same query against `events` at 100k events/day would scan millions of rows.

**SQL shape**

The query uses three CTEs:

1. `first_seen` — for each `user_id`, the minimum date within the observation window. This is the cohort assignment.
2. `cohort_sizes` — count of users per cohort date.
3. `retention_counts` — for each user, join back to `daily_active_users` for every day they were active at or after their cohort date.

The `(project_id, user_id, date)` secondary index on `daily_active_users` makes the `first_seen` aggregation and the return-day join efficient. The primary key `(project_id, date, user_id)` supports the cohort-size count.

**Triangular output**

The matrix is intentionally triangular: a cohort from 5 days ago can have at most 5 day-buckets (D+0 through D+4). The response only includes buckets for days that have elapsed — there are no placeholder entries for future periods. Consumers must account for the varying bucket count per row when rendering.

**Cohort definition: "first seen within window"**

A user is assigned to the cohort of their first appearance within the observation window `[today - (cohorts-1), today]`. A user who was active before the window opened will appear in the earliest cohort of the window — this is a standard product analytics convention. It means "new user cohorts" in the strictest sense require a full-history lookback, which this endpoint deliberately avoids to keep queries bounded.

**Caps and exclusions**

- Maximum 12 cohorts (days) per request: keeps the query bounded; the `retention_counts` JOIN processes at most `12 × cohort_size` row pairs.
- Anonymous events (`NULL user_id`) are excluded: retention is a user-centric metric; there is no identity to track across days.
- `period=week` is not yet implemented: daily cohorts cover most dashboard use cases; weekly would require date-truncation arithmetic changes but the rollup table already supports it.

---

## 7. Scaling Paths

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
