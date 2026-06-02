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
1. Redis Lua script execution (~1–5ms under normal load)
2. Async queue write (`XADD`, ~1–3ms)
3. Optional direct DB write in the API handler (~5–15ms)

p99 > 200ms typically indicates Redis saturation or a slow DB connection pool — both tracked by [3.2 pool telemetry](../internal/telemetry/pool.go).

---

## Error Budget

| Window | Allowed downtime | Allowed p99 violations |
|--------|-----------------|----------------------|
| 1 hour | 3.6 seconds | 1.8 minutes |
| 24 hours | 1.44 minutes | 43.2 minutes |
| 30 days | 43.8 minutes | 8.7 hours |

---

## Alert → Runbook mapping

| Alert | Severity | First response |
|-------|----------|---------------|
| `High5xxErrorRate` | critical | Check `GET /readyz`; inspect app logs for DB/Redis errors |
| `HighQueueLag` | warning | Check worker logs; verify worker is running (`railway logs --service worker`) |
| `DeadLetterRateNonZero` | critical | Query `SELECT * FROM dead_letter_events ORDER BY failed_at DESC LIMIT 10` |
| `HighP99Latency` | warning | Check Redis pool saturation and DB pool wait count in Grafana |
| `RateLimiterCircuitBreakerOpen` | warning | Redis may be unavailable; check `GET /readyz` and Redis Railway service |

---

## Out of scope

- **Worker throughput SLO**: The worker processes events asynchronously. A throughput target depends on deployment sizing rather than code behaviour. Queue lag (`queue_lag` gauge) is the operational signal — the alert fires before the lag becomes a user-visible problem.
- **Webhook delivery SLO**: Webhook delivery is best-effort with exponential backoff. The `webhook_deliveries_total{status="failed"}` metric tracks permanent failures, but no SLO is defined because delivery depends on third-party endpoint reliability.
