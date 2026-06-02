# EventPulse

A production-style Go event ingestion and analytics backend — inspired by Segment and PostHog.

[![CI](https://github.com/matanate/eventpulse/actions/workflows/ci.yml/badge.svg)](https://github.com/matanate/eventpulse/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)
[![Live demo](https://img.shields.io/badge/live%20demo-eventpulse.atedgimatan.com-6366f1)](https://eventpulse.atedgimatan.com)
[![API docs](https://img.shields.io/badge/API%20docs-%2Fdocs-22c55e)](https://eventpulse.atedgimatan.com/docs)

Client applications POST events to the ingestion API. Events are validated, rate-limited per API key, and enqueued to Redis Streams. A separate worker service consumes the queue, persists events to PostgreSQL, and updates aggregate counters. An analytics API surfaces the stored data.

---

## What this demonstrates

Each design decision answers a specific engineering question:

**How do you prevent duplicate events from client retries?**
`POST /v1/events` accepts an `Idempotency-Key: <uuid>` header. The key maps to a partial unique index on the `events` table — Postgres `ON CONFLICT DO NOTHING` means a retried request returns 202 but stores nothing. Both the HTTP layer and the storage layer are idempotent.

**How do you guarantee events aren't lost if the worker crashes?**
Redis Streams consumer groups keep messages un-ACK'd until a worker calls `XACK`. A 30-second ticker runs `XAUTOCLAIM` to redeliver any messages idle past the visibility timeout. After 3 delivery failures the message moves to `dead_letter_events` in Postgres for operator inspection and replay.

**How do you make rate-limiting race-free under concurrent requests?**
A single Redis Lua script — `ZREMRANGEBYSCORE + ZCARD + ZADD` — checks and increments the sliding-window counter atomically. No read-modify-write race between goroutines. Returns `Retry-After` on 429.

**How do you test a distributed system without slow or flaky mocks?**
Every integration test spins up real Postgres and Redis instances via `testcontainers-go`. The test hits the actual driver, the actual schema, the actual constraints — the same code path as production. Zero mock drift.

**How do you keep the HTTP path fast when storage writes are variable-latency?**
`POST /v1/events` calls `XADD` on a Redis Stream and returns 202 immediately. The worker `XREADGROUP`s at its own pace as a separate process. Ingestion latency is bounded by Redis (~1 ms), not Postgres.

---

## Try It Live

**[→ Open live dashboard](https://eventpulse.atedgimatan.com)** — send events and watch them appear in real time.

**[→ API Reference](https://eventpulse.atedgimatan.com/docs)** — interactive Scalar UI pointing at the live Railway API.

The API is live on Railway. Use the demo key below — rate-limited to 100 req/min.

```bash
export DEMO_URL=https://ingestion-api-production-137c.up.railway.app
export DEMO_KEY=epk_dd66ec26c39427de2e72e4badf89c968
export PROJECT_ID=06736aa0-911e-483f-8b54-659b16379984

# Ingest an event
curl -X POST $DEMO_URL/v1/events \
  -H "Authorization: Bearer $DEMO_KEY" \
  -H "Content-Type: application/json" \
  -d '{"event":"page_viewed","user_id":"user_42","properties":{"page":"/pricing"}}'
# → 202 {"status":"queued"}

# Run this command twice — idempotency in action
curl -X POST $DEMO_URL/v1/events \
  -H "Authorization: Bearer $DEMO_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{"event":"checkout_started","user_id":"user_42","properties":{"plan":"pro"}}'
# → 202 on both runs; one row stored in Postgres

# Query project stats
curl $DEMO_URL/v1/projects/$PROJECT_ID/stats \
  -H "Authorization: Bearer $DEMO_KEY"
```

---

## Architecture

```mermaid
flowchart TD
    Client["Client App\nPOST /v1/events\nBearer epk_..."] -->|HTTP| API

    subgraph API["ingestion-api :8080"]
        Auth["Auth middleware\nSHA-256 → api_keys lookup"] --> RL["Rate limiter\n100 req/min per key · Lua ZSET"]
        RL --> EH["Event handler\nvalidate → enqueue → 202"]
        EH --> AH["Analytics handlers\n/stats · /events · /events/top"]
    end

    EH -->|XADD| Redis[("Redis\nStream: events\nZSETs: rl:{api_key_id}")]
    Redis -->|XREADGROUP| Worker

    subgraph Worker["worker :8081"]
        WLoop["5 goroutines\nXREADGROUP → validate\n→ INSERT → XACK"]
        Reclaim["30 s ticker\nXAUTOCLAIM idle messages\n→ dead-letter after 3 retries"]
    end

    Worker -->|pgx/v5 pool| PG[("PostgreSQL\nevents · received_at\ndaily_event_counts\ndead_letter_events")]
    AH -->|SELECT| PG

    Prom["Prometheus :9090"] -->|scrape /metrics| API
    Prom -->|scrape :8081/metrics| Worker
    Prom --> Grafana["Grafana :3000\n7-panel dashboard"]
```

---

## Performance

Tested on a single Windows 11 host (all services co-located — no network round-trip). See [docs/performance.md](docs/performance.md) for full methodology and bottleneck analysis.

![k6 load test results](docs/img/k6-results.svg)

| Scenario | Total RPS | Ingested (202) | p95 latency | 5xx rate |
|---|---|---|---|---|
| Smoke test (1 VU) | ~254 | ~10 req/s | **10 ms** | 0% |
| 50-key · 1,000 VU | ~1,297 | ~127 req/s | 657 ms | 0.008% |
| 100-key · 1,000 VU | ~1,976 | ~282 req/s | 547 ms | **0%** |

The p95 rise at 1,000 VUs is Redis single-thread serialization under extreme concurrency on a single host — not an application bottleneck. Zero 5xx errors across all load levels.

---

## Failure modes & recovery

| Scenario | What happens | How to inspect |
|---|---|---|
| Worker crashes mid-batch | Message stays un-ACK'd in the consumer group. `XAUTOCLAIM` redelivers it to the next available consumer after 30 s. | `GET /v1/admin/queue/stats` — `consumer_lag > 0` while recovering |
| Client retries same event (same `Idempotency-Key`) | Both requests return 202. Postgres `ON CONFLICT DO NOTHING` stores exactly one row. | Event appears once in the feed; no error logged |
| Event fails worker validation 3× | Written to `dead_letter_events` with full payload and last error. Stream message is ACK'd to prevent infinite redelivery. | Query `dead_letter_events` table directly; `GET /v1/admin/queue/stats` shows `dead_letter_count` |
| Redis unavailable | Rate limiter fails open — event is allowed through. `ratelimit_errors_total` counter increments. | Prometheus alert on `ratelimit_errors_total > 0`; see [docs/tradeoffs.md](docs/tradeoffs.md) |

---

## Security Hardening

Production-grade protections applied to the ingestion API:

| Protection | Detail |
|---|---|
| **Slow-loris mitigation** | `ReadHeaderTimeout: 5s` on the HTTP server — slow header attacks time out before consuming a goroutine |
| **Request body size cap** | `http.MaxBytesReader` limits every ingest request to 1 MiB; oversized payloads return `413 PAYLOAD_TOO_LARGE` |
| **Properties size cap** | `properties` JSONB field validated to ≤ 50 top-level keys and ≤ 4 KiB encoded; rejects bloated payloads before they hit the DB |
| **Timestamp bounds** | Event `timestamp` must be within 24 h past / 1 min future; rejects pre-dated or far-future events |
| **Health endpoint safety** | `/readyz` returns `"unavailable"` for failed dependencies — raw driver error strings never reach the public response; logged server-side via `slog.Warn` |
| **CORS allowlist** | Exact origin allowlist (`eventpulse.pages.dev`, `eventpulse.atedgimatan.com`, `localhost:5173`) — no wildcard subdomains |

---

## Quick Start

**Prerequisites:** Docker, Go 1.22+, [golang-migrate](https://github.com/golang-migrate/migrate)

```bash
# 1. Start infrastructure (Postgres + Redis)
make infra-up

# 2. Copy env and apply migrations
cp .env.example .env
make migrate

# 3. Generate an API key (prints the raw epk_... value)
make seed
```

Then in two terminals:

```bash
make run-api     # ingestion API on :8080
make run-worker  # worker consumer
```

Verify:

```bash
curl http://localhost:8080/readyz
# → {"status":"ok"}
```

To see live metrics in Grafana:

```bash
make infra-obs   # Prometheus :9090 + Grafana :3000
```

---

## API Endpoints

All authenticated endpoints require `Authorization: Bearer epk_...`. See [docs/api.md](docs/api.md) for full request/response examples and error codes, or open the [interactive API reference](https://eventpulse.atedgimatan.com/docs).

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/healthz` | — | Liveness check |
| `GET` | `/readyz` | — | Readiness: DB + Redis ping |
| `GET` | `/metrics` | — | Prometheus metrics |
| `GET` | `/openapi.json` | — | OpenAPI 3.1 machine-readable spec |
| `POST` | `/v1/events` | ✓ | Ingest a single event → 202; accepts `Idempotency-Key` header |
| `POST` | `/v1/events/batch` | ✓ | Ingest up to 100 events → 202 |
| `GET` | `/v1/projects/{id}/stats` | ✓ | Total events, today's count, top 5 names |
| `GET` | `/v1/projects/{id}/events` | ✓ | Paginated event list (filter by name, user, date) |
| `GET` | `/v1/projects/{id}/events/top` | ✓ | Top N event names by count |
| `GET` | `/v1/projects/{id}/users/{uid}/events` | ✓ | All events for a user |
| `GET` | `/v1/admin/queue/stats` | ✓ | Live queue depth, consumer lag, dead-letter count |

---

## Local Development

```bash
make test                    # unit + integration tests
make test-cover              # generate coverage.html
make lint                    # golangci-lint
make build                   # compile both binaries to bin/
make seed SEED_COUNT=50      # generate 50 API keys for load testing
make loadtest                # k6 load test (requires EVENTPULSE_API_KEYS)
```

---

## Documentation

- [Architecture](docs/architecture.md) — system diagram, component reference, data flow sequences
- [API Reference](docs/api.md) — all endpoints with curl examples and error codes
- [Performance](docs/performance.md) — load test methodology and results
- [Trade-offs](docs/tradeoffs.md) — async vs sync, Redis rate limiting, JSONB, crash recovery, scaling paths
