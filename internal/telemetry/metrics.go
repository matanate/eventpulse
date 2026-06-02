package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EventsIngestedTotal counts events received by the ingestion API.
	EventsIngestedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_ingested_total",
		Help: "Total events received by the ingestion API.",
	}, []string{"status"}) // success | error

	// EventsProcessedTotal counts events handled by the worker.
	EventsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_processed_total",
		Help: "Total events processed by the worker.",
	}, []string{"status"}) // success | dead_letter

	// EventsFailedTotal counts events that failed processing.
	EventsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_failed_total",
		Help: "Total events that failed processing in the worker.",
	}, []string{"reason"}) // transient | format

	// APIRequestDurationSeconds tracks HTTP request latency by route.
	APIRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "api_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	// RateLimitedRequestsTotal counts requests rejected by the rate limiter.
	RateLimitedRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rate_limited_requests_total",
		Help: "Total requests rejected by the rate limiter.",
	})

	// QueueLag tracks the number of pending unacknowledged messages in the stream consumer group.
	QueueLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "queue_lag",
		Help: "Number of pending unacknowledged messages in the Redis stream consumer group.",
	})

	// WorkerProcessingDurationSeconds tracks how long the worker spends on each event.
	WorkerProcessingDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "worker_processing_duration_seconds",
		Help:    "Time spent processing a single event in the worker.",
		Buckets: prometheus.DefBuckets,
	})

	// WebhookDeliveriesTotal counts outbound webhook delivery attempts by outcome.
	WebhookDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_deliveries_total",
		Help: "Total webhook delivery attempts by outcome.",
	}, []string{"status"}) // delivered | retried | failed

	// WebhookDeliveryDurationSeconds tracks HTTP delivery round-trip time.
	WebhookDeliveryDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "webhook_delivery_duration_seconds",
		Help:    "Time spent on a successful webhook HTTP delivery.",
		Buckets: prometheus.DefBuckets,
	})

	// WebhookPendingDeliveries is set by the dispatcher on each poll cycle.
	WebhookPendingDeliveries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "webhook_pending_deliveries",
		Help: "Number of webhook delivery rows claimed in the latest dispatcher poll.",
	})
)
