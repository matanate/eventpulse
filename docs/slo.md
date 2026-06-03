# EventPulse — Service Level Objectives

This document defines the SLOs for the EventPulse ingestion API and worker, the rationale behind each target, and the alert rules that fire when an SLO is at risk.

---

## SLO 1 — Availability: 99.9% over 30 days

**Definition:** The fraction of ingestion requests that return a non-error response (HTTP 2xx or 429).

> 429 (rate limited) counts as _available_ — the system responded correctly. Only 5xx responses and network timeouts count as _unavailable_.

**Target:** 99.9% (error budget: 43.8 minutes per 30-day window)

**Measurement:**

```promql
1 - (
  sum(rate(events_ingested_total{status="error"}[30d]))
  /
  sum(rate(events_ingested_total[30d]))
)
```

**Alert:** `High5xxErrorRate` fires when the 5-minute rolling error rate exceeds 1% for 5 consecutive minutes — a leading indicator for SLO burn before the 30-day window closes.

**Rationale:** Event ingestion is the critical path. Dropped events cannot be recovered after the fact, so the availability bar is high. 99.9% leaves ~43 minutes of downtime per month — enough for a rolling deploy or Redis failover.

---

## SLO 2 — Latency: p99 < 200ms over any 5-minute window

**Definition:** The 99th-percentile HTTP response time for `POST /v1/events` and `POST /v1/events/batch`.

**Target:** p99 ≤ 200ms

**Measurement:**

```promql
histogram_quantile(0.99,
  sum(rate(api_request_duration_seconds_bucket[5m])) by (le)
)
```

**Alert:** `HighP99Latency` fires when the 5-minute p99 exceeds 200ms for 5 consecutive minutes.

**Rationale:** Client SDKs typically fire-and-forget events on a 1-second flush cycle. A p99 of 200ms keeps the flush well within the SDK timeout budget and ensures that rate-limit headers (`Retry-After`) are visible to clients before their own timeout fires.

The main latency contributors are:
1. Auth middleware — SHA-256 hash + single-row `api_keys` lookup (~2–4ms)
2. Rate limiter — Redis Lua script (ZREMRANGEBYSCORE + ZCARD + ZADD, ~1–3ms)
3. Schema validation — in-process compile cache lookup (~0.1ms cache hit; first compile ~1–10ms)
4. Queue write — `XADD` to Redis Streams (~1–2ms)
5. SSE broadcast — `PUBLISH` to Redis pub/sub (~1ms, fire-and-forget, does not block 202 response)

The ingestion API does **not** write to PostgreSQL synchronously. All DB writes happen in the worker process after `XREADGROUP`.

p99 > 200ms typically indicates Redis saturation (rate limiter or XADD queuing under extreme concurrency) — visible in the `rate_limiter_circuit_breaker_state` gauge and `api_request_duration_seconds` histogram.

**SSE exclusion**: `GET /v1/projects/{id}/stream` is a long-lived Server-Sent Events connection that may last minutes or hours. The `RequestDuration` histogram records this as an extremely high-latency "request," which would invalidate the p99 target. The latency SLO applies **only to request-response endpoints** — SSE connections are excluded from this SLO. A separate connection-count gauge (`sse_active_connections` if instrumented) is the operational signal for SSE.

---

## Error Budget

| Window | Allowed downtime (availability SLO) | Allowed 5-min windows above p99 target |
|--------|-------------------------------------|----------------------------------------|
| 1 hour | 3.6 seconds | 1 window (≤ 5 min) |
| 24 hours | 1.44 minutes | 14 windows (≤ 70 min) |
| 30 days | 43.8 minutes | 432 windows (≤ 36 hours) |

> Latency columns count 5-minute scrape windows where p99 > 200ms, not wall-clock minutes. A single bad minute counts as one full window.


---

## Alert → Runbook mapping

| Alert | Severity | First response |
|-------|----------|---------------|
| `High5xxErrorRate` | critical | Check `GET /readyz`; inspect app logs for DB/Redis errors |
| `HighQueueLag` | warning | Check worker logs; verify worker is running (`railway logs --service worker`) |
| `DeadLetterRateNonZero` | critical | Query `SELECT * FROM dead_letter_events ORDER BY failed_at DESC LIMIT 10` |
| `HighP99Latency` | warning | Check Redis pool saturation and DB pool wait count in Grafana; verify SSE connections are excluded from the histogram (see SLO 2 note) |
| `RateLimiterCircuitBreakerOpen` | warning | Redis may be unavailable; check `GET /readyz` and Redis Railway service |
| `WebhookDeliveryFailureRate` | warning | Query `webhook_deliveries_total{status="failed"}` — inspect `webhook_subscriptions` for unreachable URLs; check for SSRF blocks in API logs |

---

## Out of scope

- **Worker throughput SLO**: The worker processes events asynchronously. A throughput target depends on deployment sizing rather than code behaviour. Queue lag (`queue_lag` gauge) is the operational signal — the alert fires before the lag becomes a user-visible problem.
- **Webhook delivery SLO**: Webhook delivery is best-effort with exponential back-off. No SLO is defined because delivery latency and success rate depend entirely on third-party endpoint reliability — outside EventPulse's control. `webhook_deliveries_total{status="failed"}` tracks permanent failures after retry exhaustion; alert on sustained non-zero rate.
- **SSE connection stability**: SSE connections are long-lived and browser-reconnected. No uptime SLO is defined; `X-Accel-Buffering: no` and cleared write deadlines are the operational mitigations.
- **Schema registry**: Schema validation failures (enforce mode) are client errors (422), not service errors. `schema_violations_total` tracks warn-mode violations. No SLO is defined — correctness depends on client payload conformance.
