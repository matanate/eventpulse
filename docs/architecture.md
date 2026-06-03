# EventPulse — Architecture

## Overview

EventPulse is a two-service Go backend for event ingestion and analytics, inspired by Segment and PostHog. Client applications send events to the ingestion API, which validates, authenticates, and enqueues them. A separate worker service consumes the queue, persists events to PostgreSQL, and updates aggregate counters. The analytics API reads from PostgreSQL to answer queries.

---

## System Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                            Clients                                  │
│   POST /v1/events  (Bearer epk_...)   EventSource /stream           │
└──────────────────────┬──────────────────────┬───────────────────────┘
                       │ HTTP                 │ SSE
                       ▼                      ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ingestion-api  (:8080)                           │
│                                                                     │
│   RequestID ──► RequestLogger ──► RequestDuration (histogram)       │
│        │                                                            │
│        ▼                                                            │
│   Auth Middleware                                                   │
│   SHA-256(token) → api_keys lookup → project_id into context        │
│        │                            (401 if missing/invalid)        │
│        ▼                                                            │
│   Rate Limiter                                                      │
│   Redis ZSET sliding window  100 req/min per api_key_id             │
│        │                            (429 + Retry-After if over)     │
│        ▼                                                            │
│   Schema Validator  ◄── schemas table (compile cache, LRU)         │
│   enforce mode → 422; warn mode → metric + accept                  │
│        │                                                            │
│        ▼                                                            │
│   Handler: validate payload ──► XADD "events" stream               │
│                              └─► PUBLISH "events:{projectID}"       │
│                                          │  (202 Accepted)          │
│                                          │                          │
│   SSE Handler ◄── Redis SUBSCRIBE "events:{projectID}"             │
│   GET /v1/projects/{id}/stream  (long-lived, rate-limit exempt)    │
│                                          │                          │
│   Analytics Handlers ◄── PostgreSQL ◄────┘  (via worker, async)    │
│   /stats | /events | /events/top | /funnels | /retention            │
│                                                                     │
│   Webhook CRUD   POST/GET /v1/webhooks   DELETE /v1/webhooks/{id}  │
│   Schema CRUD    POST/GET/DELETE /v1/projects/{id}/schemas/{event}  │
│                                                                     │
│   GET /healthz   GET /readyz   GET /metrics                         │
└────────────┬──────────────────────────────────────┬────────────────┘
             │ XADD (Redis Streams)                 │ SSRF-guarded
             ▼                                      ▼ HTTPS delivery
┌────────────────────────────┐         ┌────────────────────────────┐
│          Redis             │         │   External webhook          │
│                            │         │   endpoints                 │
│   Stream:  "events"        │         └────────────────────────────┘
│   Group:   "workers"       │
│   RL keys: "rl:{key_id}"   │
│   PubSub:  "events:{id}"   │
└────────────┬───────────────┘
             │ XREADGROUP BLOCK 2000ms COUNT 10
             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      worker  (:8081/metrics)                        │
│                                                                     │
│   concurrency=5 goroutines, each loop:                              │
│   ┌────────────────────────────────────────────────────────────┐   │
│   │  XREADGROUP → decode → validate → INSERT events            │   │
│   │                             → UPSERT daily_event_counts     │   │
│   │                             → UPSERT daily_active_users     │   │
│   │                             → XACK                          │   │
│   │  on transient error: no XACK (message redelivered)          │   │
│   │  on format error:   immediate dead-letter + XACK            │   │
│   │  after 3 deliveries: INSERT dead_letter_events + XACK       │   │
│   └────────────────────────────────────────────────────────────┘   │
│                                                                     │
│   30s ticker → XAUTOCLAIM (reclaim idle messages from crashed       │
│                            workers, check delivery count)           │
│                                                                     │
│   webhook dispatcher → poll webhook_subscriptions every 1s         │
│     → deliver pending events → retry with exponential back-off     │
└────────────┬────────────────────────────────────────────────────────┘
             │ pgx/v5 pool
             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                       PostgreSQL (:5433)                            │
│                                                                     │
│   accounts, projects, api_keys                                      │
│   events              (composite indexes on project+event+time,     │
│                                          project+user+time)         │
│   daily_event_counts  (upserted by worker; powers stats/top)        │
│   daily_active_users  (upserted by worker; powers retention query)  │
│   webhook_subscriptions  (url, encrypted_secret, filter_event)      │
│   schemas             (per project+event JSON Schema + mode)        │
│   dead_letter_events  (unrecoverable messages for operator review)  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Reference

### `cmd/ingestion-api`

Entry point for the HTTP API. Wires config, DB pool, Redis client, auth middleware, rate limiter, event handler, analytics handler, and the Chi router. Starts the HTTP server and shuts down gracefully on SIGINT/SIGTERM.

### `cmd/worker`

Entry point for the consumer loop. Wires config, DB pool, Redis client, `StreamConsumer`, and `Worker`. Also starts a dedicated metrics HTTP server on `:8081` and a queue-lag polling goroutine.

### `internal/config`

Loads all configuration from environment variables at startup. Fails fast with a list of missing required vars. Consumed by both binaries.

### `internal/auth`

Chi middleware that reads the `Authorization: Bearer epk_...` header, hashes the token with SHA-256, and looks up the hash in `api_keys`. On success, injects `project_id` and `api_key_id` into `context.Context`. Handlers extract these values via `auth.ProjectIDFromContext`.

### `internal/ratelimit`

Atomic sliding-window rate limiter backed by a Redis sorted set. A Lua script atomically trims old entries, counts the window, and adds a new entry. Returns `(allowed bool, retryAfter time.Duration)`. The middleware writes `Retry-After` on 429.

### `internal/events`

`Event` struct and `Validate()`, `Store()`, `StoreBatch()`, `UpsertDailyCount()`. The `Publisher` interface (implemented by `queue.StreamPublisher`) allows the handler to publish without depending on the queue package.

### `internal/queue`

`Publisher` and `Consumer` interfaces with Redis Streams implementations. `StreamPublisher.Publish` → XADD; `PublishBatch` → pipeline of XADDs. `StreamConsumer.Read` → XREADGROUP BLOCK; `Reclaim` → XAUTOCLAIM + XPENDINGEXT. `PendingCount` → XPENDING summary (used for the `queue_lag` metric).

### `internal/worker`

`Worker` struct runs `concurrency` goroutines. Each goroutine loops: read messages, process via `handleMessage`, reclaim idle messages every 30s. Graceful shutdown via `sync.WaitGroup` and `context.Context` cancellation — each goroutine finishes its current message before exiting.

### `internal/analytics`

Query functions (`Stats`, `ListEvents`, `TopEvents`, `UserEvents`, `Funnels`, `Retention`) and HTTP handlers. Dynamic SQL with `$N` placeholders for optional filters. Scope check on every handler verifies URL `{projectID}` equals the auth context `project_id`. Funnel analysis uses dynamically generated CTEs (one per step); retention queries use the `daily_active_users` rollup.

### `internal/sse`

`RedisBroadcaster` publishes JSON-encoded event payloads to `events:{projectID}` on Redis pub/sub after each successful XADD. `Handler` subscribes to the same channel and writes each message as a `data:` SSE frame. The connection clears the server-level write deadline (necessary for long-lived streaming) and is exempt from per-key rate limiting.

### `internal/webhooks`

CRUD handlers for webhook subscriptions stored in `webhook_subscriptions`. Secrets are encrypted with AES-256-GCM before storage. Outgoing deliveries are signed with HMAC-SHA256. SSRF is prevented via `ssrf.go` (`ValidateURL` + `safeDial` — resolves hostname, checks all IPs, connects directly to avoid DNS-rebinding TOCTOU). Redirects are disabled.

### `internal/schemas`

Schema registry with `Store` (Postgres-backed CRUD), `SchemaValidator` (in-memory compile cache, LRU eviction), and HTTP handlers. `Compile` validates a submitted JSON Schema document with a 3-second goroutine timeout to guard against pathological inputs. On each event ingest, the validator is consulted; `enforce` mode returns 422, `warn` mode increments a Prometheus counter.

### `internal/telemetry`

Seven Prometheus metrics defined via `promauto` (auto-registers with default registry). `RequestLogger` middleware (slog INFO per request) and `RequestDuration` middleware (histogram using Chi's route pattern, not actual URL, to avoid label cardinality explosion).

### `internal/health`

`/healthz` always returns 200. `/readyz` pings PostgreSQL and Redis; returns 503 with details if either is unavailable.

---

## Data Flow: Single Event Ingestion

```
1.  Client sends POST /v1/events with Bearer token and JSON payload.

2.  Chi middleware chain:
    a. RequestID assigns a unique ID to the request.
    b. Auth middleware hashes the token, queries api_keys,
       and injects project_id + api_key_id into context.
    c. Rate limiter checks the Redis ZSET; increments counter.

3.  Handler decodes and validates the payload.
    Returns 400 VALIDATION_FAILED on invalid fields.

4.  Handler calls publisher.Publish(ctx, event).
    XADD appends the event (JSON-encoded) to the "events" stream.

5.  Handler returns 202 {"status": "queued"}.

6.  Worker's XREADGROUP returns the message (within 2 seconds
    of enqueue for a healthy worker).

7.  Worker decodes the message, calls events.Store (INSERT events),
    then events.UpsertDailyCount (INSERT ... ON CONFLICT DO UPDATE).

8.  Worker calls XACK. Message is removed from the PEL.

9.  Event is now queryable via analytics endpoints.
```

## Data Flow: Crash Recovery

```
1.  Worker reads message M from stream, begins processing.

2.  Worker crashes before XACK.

3.  After MinIdleTime (30s), the next reclaim tick claims M
    via XAUTOCLAIM. DeliveryCount is incremented.

4.  If DeliveryCount < MaxRetries (3): process normally.

5.  If DeliveryCount >= MaxRetries: write to dead_letter_events, XACK.
    Message is out of the stream; operator can inspect and replay.
```

---

## Observability

| Signal | Where | Description |
|---|---|---|
| Metrics | `GET /metrics` (:8080) | Prometheus default registry; ingested, processed, failed, durations, queue lag |
| Metrics (worker) | `GET /metrics` (:8081) | Worker-specific counters and histograms |
| Logs | stdout (slog JSON) | Every request: request_id, method, path, status, duration_ms |
| Health | `GET /readyz` | DB + Redis ping |
| Dashboard | Grafana :3000 | Auto-provisioned 7-panel dashboard |

Start observability stack: `make infra-obs`

---

## Configuration

All configuration is via environment variables. See `.env.example` for defaults.

| Variable | Default | Description |
|---|---|---|
| `APP_PORT` | `8080` | Ingestion API listen port |
| `METRICS_PORT` | `8081` | Worker metrics server port |
| `DATABASE_URL` | — | PostgreSQL connection string (required) |
| `REDIS_URL` | — | Redis connection string (required) |
| `WORKER_CONCURRENCY` | `5` | Number of worker goroutines |
| `DB_MAX_CONNS` | `25` | pgxpool max connections |
| `DB_MIN_CONNS` | `5` | pgxpool min connections |
| `APP_ENV` | `development` | `development` uses text log format; anything else uses JSON |
| `LOG_LEVEL` | `info` | `info` or `debug` |
| `WEBHOOK_SECRET_KEY` | — | AES-256 key for encrypting webhook secrets at rest (required; generate with `make gen-webhook-key`) |
| `WEBHOOK_HTTP_TIMEOUT` | `10s` | Per-delivery HTTP timeout for outgoing webhook calls |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP HTTP endpoint for distributed tracing (omit to disable) |
