import http from 'k6/http';
import { check } from 'k6';
import { Counter, Trend } from 'k6/metrics';

// Custom metrics
const ingested202 = new Counter('ingested_202');
const rateLimited429 = new Counter('rate_limited_429');
const errors5xx = new Counter('errors_5xx');
const ingestDuration = new Trend('ingest_duration_ms', true);

// Config from environment
const BASE_URL = __ENV.EVENTPULSE_API_URL || 'http://localhost:8080';
const SCENARIO = __ENV.SCENARIO || 'throughput';

const rawKeys = __ENV.EVENTPULSE_API_KEYS || '';
if (!rawKeys) {
    throw new Error(
        'EVENTPULSE_API_KEYS is required.\n' +
        'Run: make seed SEED_COUNT=50\n' +
        'Then: export EVENTPULSE_API_KEYS=<output from seed>'
    );
}

const ALL_KEYS = rawKeys.split(',').filter(k => k.trim().length > 0);

// ratelimited scenario: deliberately saturate the limiter with few keys
const ACTIVE_KEYS = SCENARIO === 'ratelimited' ? ALL_KEYS.slice(0, 5) : ALL_KEYS;

const EVENT_NAMES = [
    'page_viewed',
    'button_clicked',
    'form_submitted',
    'checkout_started',
    'purchase_completed',
    'search_performed',
    'item_added_to_cart',
];

export const options = {
    scenarios: {
        ingest: {
            executor: 'ramping-arrival-rate',
            startRate: 0,
            timeUnit: '1s',
            preAllocatedVUs: 150,
            maxVUs: 600,
            stages: [
                { duration: '30s', target: 100 },    // warm up to 100 req/s
                { duration: '90s', target: 1000 },   // ramp to target
                { duration: '2m',  target: 1000 },   // sustain
                { duration: '30s', target: 0 },      // ramp down
            ],
        },
    },
    thresholds: {
        // No more than 10 hard server errors total
        errors_5xx: ['count<10'],
        // Successful ingestion p95 under 500ms
        ingest_duration_ms: ['p(95)<500'],
    },
    // Drop response bodies — we only care about status codes and timing
    discardResponseBodies: true,
};

export default function () {
    // Round-robin across active keys per virtual user
    const key = ACTIVE_KEYS[(__VU - 1) % ACTIVE_KEYS.length];

    const payload = JSON.stringify({
        event: EVENT_NAMES[Math.floor(Math.random() * EVENT_NAMES.length)],
        user_id: `vu-${__VU}`,
        properties: {
            source: 'k6',
            scenario: SCENARIO,
            iter: __ITER,
        },
    });

    const res = http.post(`${BASE_URL}/v1/events`, payload, {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${key}`,
        },
        tags: { endpoint: 'ingest' },
    });

    if (res.status === 202) {
        ingested202.add(1);
        ingestDuration.add(res.timings.duration);
    } else if (res.status === 429) {
        rateLimited429.add(1);
    } else if (res.status >= 500) {
        errors5xx.add(1);
    }

    check(res, {
        'not 5xx': (r) => r.status < 500,
    });
}
