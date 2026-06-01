# EventPulse — Improvement Roadmap

This document tracks planned improvements to EventPulse, ordered by implementation priority. Each item is scoped to one development session, implemented on a `dev` branch, and merged to `main` via PR.

## Status legend

| Symbol | Meaning |
|--------|---------|
| ⬜ | Planned |
| 🔄 | In progress |
| ✅ | Complete |

---

## Tier 1 — Quick Wins

High signal-to-effort ratio. These close obvious gaps and make the existing system look as good as it actually is.

### 1.1 — Live queue-lag endpoint + dashboard tile ⬜

**Why:** The async pipeline is the core architectural claim of EventPulse, but currently invisible at runtime. Showing real queue depth, pending-message count, and dead-letter count as live metrics turns the static `PipelineCard` diagram into something a visitor can watch.

**Backend:**
- Verify `telemetry.QueueLag` gauge is published by a ticker in the worker
- Add `GET /v1/admin/queue/stats` returning `{pending, consumer_lag, dead_letter_count}`
- Publish `worker_dead_letter_total` counter from existing dead-letter path

**Frontend:**
- Add a "Pipeline Health" card to the dashboard with live queue depth + dead-letter count
- Animate queue depth bar when a batch is sent (drop → drain effect)

### 1.2 — Idempotency-Key header + demo button ⬜

**Why:** The `events` table already has a partial unique index on `idempotency_key` and `Store()` does `ON CONFLICT DO NOTHING`, but the HTTP surface never accepts the header from clients. Closing this gap proves the system handles client retries correctly — a question every senior engineer asks about ingestion pipelines.

**Backend:**
- Accept `Idempotency-Key: <uuid>` request header in `HandleIngest`
- Validate format (UUID v4), map to `event.IdempotencyKey`
- Document in `docs/api.md`

**Frontend:**
- Add "Send duplicate event" button in EventSender that fires the same event twice with the same key
- Show in RequestLog: second request returns `202` but feed shows no duplicate (proof of exactly-once)

### 1.3 — OpenAPI 3.1 spec + interactive docs page ⬜

**Why:** An interactive `/docs` page that talks to the live Railway API lets reviewers try requests without curl. `docs/api.md` already documents every endpoint and error code — this is a structured transcription of that document.

**Backend:**
- Author `api/openapi.yaml` covering all 9 endpoints with request/response schemas and error codes
- Serve at `GET /openapi.yaml` from the ingestion API

**Frontend:**
- Add `/docs` route rendering Scalar or Swagger UI pointing at the live spec
- Pre-configure the demo API key as the default auth token

### 1.4 — README reframe + load-test evidence ⬜

**Why:** Hiring managers spend ~30 seconds on a README. The current README leads with a throughput number but buries the genuinely impressive engineering (Lua atomicity, XAUTOCLAIM recovery, exactly-once storage, dead-letter handling).

**Changes:**
- Add "What makes this interesting" section at the top with 4-5 specific engineering decisions
- Embed the k6 results as a committed PNG/SVG chart instead of a markdown table
- Add a dashboard GIF showing the live feed, rate-limit banner, and batch send
- Add a "Failure modes and recovery" subsection with links to `docs/tradeoffs.md`
- Reconcile `created_at` / `received_at` naming between README and schema

---

## Tier 2 — Product Depth

Analytics features a real PostHog or Segment engineer would ship. These demonstrate SQL skill and product thinking.

### 2.1 — Funnel analysis endpoint + visualization ⬜

**Why:** Funnels are the canonical product-analytics query and are non-trivial in SQL. A correct, indexed, tested implementation demonstrates senior-level data modeling. The `(project_id, user_id, timestamp)` composite index already exists to support it.

**Backend:**
- `POST /v1/projects/{id}/funnels` — accepts ordered event list + time window (ISO 8601 duration)
- Returns per-step `{event, entered, converted, dropped, conversion_rate}` using a windowed self-join on `events` keyed by `user_id`
- Cap window to 90 days; document strict-order semantic in `docs/tradeoffs.md`
- Integration test with `testcontainers-go`

**Frontend:**
- `FunnelChart` — horizontal step visualization with conversion rate labels
- Pre-configured example funnel in the dashboard using the seed event names

### 2.2 — Retention / cohort grid ⬜

**Why:** Retention heatmaps are instantly recognizable as "real analytics product." The first-seen cohorting + return-window bucketing query is a genuinely hard SQL problem.

**Backend:**
- `GET /v1/projects/{id}/retention?period=day&cohorts=8` — returns triangular cohort matrix
- Worker maintains `daily_active_users` rollup (similar to `daily_event_counts`) to keep query bounded
- New migration `000003_daily_active_users.up.sql`

**Frontend:**
- Triangular heatmap rendered with CSS grid (no new charting library)
- Tooltip showing absolute and percentage retention per cell

### 2.3 — OpenTelemetry distributed tracing ⬜

**Why:** Tracing a request across an async queue boundary (HTTP → Redis Stream → worker → DB) is exactly the kind of thing that separates senior from mid-level engineers. The screenshot of a single trace spanning two services is portfolio gold.

**Implementation:**
- Add `go.opentelemetry.io/otel` SDK; instrument ingestion handler with a root span
- Inject trace context into the Redis Stream message headers (W3C `traceparent`)
- Worker extracts and continues the trace span through consume → DB write
- Export to Jaeger in Docker Compose (no paid backend); log `trace_id` in `slog` for production
- Add Jaeger service to `deploy/docker-compose.yml`

### 2.4 — Outbound webhooks with HMAC signing + retry ⬜

**Why:** Webhooks demonstrate outbound reliability engineering — signing, retries, timeouts, and SSRF protection on user-supplied URLs. The existing dead-letter machinery provides the retry model.

**Backend:**
- New migration: `webhook_subscriptions` (url, secret, filter_event, project_id) + `webhook_deliveries` (status, attempts, next_retry)
- Worker delivers matching events via HTTP POST with `X-EventPulse-Signature` (HMAC-SHA256)
- Exponential backoff (1s → 2s → 4s … capped at 1h); after max attempts → `webhook_deliveries` dead state
- SSRF protection: block private/loopback IPs on target URL resolution
- `GET /v1/projects/{id}/webhooks` + `POST` + `DELETE` CRUD endpoints

**Security gate:** This phase requires a security-reviewer pass before merge.

---

## Tier 3 — Operational Maturity

Infrastructure that makes the system feel production-grade.

### 3.1 — Prometheus alert rules + SLO dashboard ⬜

**Why:** Defining SLOs and error budgets — not just collecting metrics — demonstrates you operate systems, not just build them.

**Changes:**
- `deploy/prometheus/alerts.yml` — rules for queue lag > threshold, dead-letter rate > 0, 5xx ratio, p99 latency burn rate
- Grafana SLO panel: availability target (99.9%), latency target (p99 < 200ms), error-budget remaining gauge
- `docs/slo.md` — define the objectives and rationale

### 3.2 — Connection-pool + Redis telemetry ⬜

**Why:** Turns hand-wavy performance claims ("Redis single-thread serialization bottleneck") into instrumented, graphed conclusions.

**Changes:**
- Export `pgxpool.Stat()` fields (acquired, idle, wait duration) as Prometheus gauges
- Export Redis pool stats as gauges
- Add "Resource Saturation" Grafana row with pool utilization graphs

### 3.3 — Rate-limiter fail-open mode + circuit breaker ⬜

**Why:** `docs/tradeoffs.md` explicitly notes the limiter fails *closed* on Redis errors. Adding a configurable fail-open mode and a circuit breaker closes a documented known limitation — showing you read and act on your own analysis.

**Changes:**
- `internal/ratelimit`: add `FailMode` config option (`closed` default, `open` for high-availability preference)
- Wrap Redis calls in a simple half-open circuit breaker (3 failures → open, 10s reset)
- Expose breaker state as a Prometheus gauge
- Update `docs/tradeoffs.md`

---

## Tier 4 — Big Bets

Highest ceiling, most effort. Do after Tier 1-2 are complete.

### 4.1 — Real-time dashboard via SSE ⬜

**Why:** Replacing 3s polling with server-push makes the demo noticeably more impressive and demonstrates SSE + Redis pub/sub fan-out.

**Backend:**
- `GET /v1/projects/{id}/stream` — SSE endpoint; subscribes to a Redis pub/sub channel the ingestion path publishes to per event
- Graceful backpressure: close connection if buffer exceeds threshold

**Frontend:**
- Replace `usePolledResource` for EventFeed with `useEventSource`
- Feed updates in < 500ms from ingestion

### 4.2 — TypeScript client SDK ⬜

**Why:** Shipping the consumer side (an SDK with batching, idempotency keys, retry-with-backoff) shows full product lifecycle thinking.

**Deliverable:**
- `sdk/` directory: `@eventpulse/client` with `identify`, `track`, `trackBatch` methods
- Auto-batching with configurable flush interval and queue size
- Idempotency key generation (UUID v4) on every event
- Exponential backoff retry on 5xx/network errors
- The dashboard itself refactored to use the SDK as its API client

### 4.3 — Event schema registry ⬜

**Why:** JSON Schema enforcement on a schemaless store closes the limitation explicitly documented in `docs/tradeoffs.md` ("JSONB cannot enforce property types"). This is PostHog's "Tracking Plans" / Segment's "Protocols" feature.

**Backend:**
- `POST /v1/projects/{id}/schemas/{event}` — register a JSON Schema for an event name
- Ingestion path validates `properties` against the registered schema: configurable `enforce` (reject) vs `warn` (accept + emit metric) mode
- New migration: `event_schemas` table

---

## Implementation order

```
1.1 Queue stats   →  1.2 Idempotency  →  1.3 OpenAPI  →  1.4 README
       ↓
2.1 Funnels  →  2.2 Retention
       ↓
2.3 OTel tracing  +  3.1 Alerts/SLO  +  3.2 Pool telemetry  (observability cluster)
       ↓
2.4 Webhooks  (security-gated)
       ↓
4.x  Big bets
```

Each phase is implemented on a `dev` branch and merged to `main` via PR.
