package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/matangi/eventpulse/internal/events"
	"github.com/matangi/eventpulse/internal/queue"
	"github.com/matangi/eventpulse/internal/telemetry"
)

// Worker runs configurable-concurrency goroutines that consume from a Redis Stream,
// persist events to Postgres, and handle retries + dead-lettering.
type Worker struct {
	consumer    queue.Consumer
	pool        *pgxpool.Pool
	concurrency int
}

func New(consumer queue.Consumer, pool *pgxpool.Pool, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Worker{consumer: consumer, pool: pool, concurrency: concurrency}
}

// Run starts concurrency goroutines and blocks until ctx is cancelled and all
// goroutines have finished their current message.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.runLoop(ctx)
		}(i)
	}
	wg.Wait()
}

func (w *Worker) runLoop(ctx context.Context) {
	reclaimTicker := time.NewTicker(30 * time.Second)
	defer reclaimTicker.Stop()

	for {
		// Non-blocking check for shutdown or reclaim tick before each read.
		select {
		case <-ctx.Done():
			return
		case <-reclaimTicker.C:
			w.reclaimAndProcess(ctx)
		default:
		}

		// Blocks up to 2 seconds waiting for new messages.
		msgs, err := w.consumer.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("worker: read from stream", "err", err)
			continue
		}

		for _, msg := range msgs {
			w.handleMessage(ctx, msg)
		}
	}
}

func (w *Worker) reclaimAndProcess(ctx context.Context) {
	msgs, err := w.consumer.Reclaim(ctx)
	if err != nil {
		slog.Error("worker: reclaim pending messages", "err", err)
		return
	}
	for _, msg := range msgs {
		if msg.DeliveryCount >= queue.MaxRetries {
			w.deadLetter(ctx, msg, fmt.Errorf("max retries (%d) exceeded", queue.MaxRetries))
		} else {
			w.handleMessage(ctx, msg)
		}
	}
}

// handleMessage decodes the payload, stores the event, and ACKs on success.
// Format errors are dead-lettered immediately; transient store errors are left
// un-ACKed so the message is redelivered by the next reclaim cycle.
func (w *Worker) handleMessage(ctx context.Context, msg queue.Message) {
	start := time.Now()

	e, err := decodeEvent(msg.Payload)
	if err != nil {
		// Unrecoverable format error — dead-letter without retry.
		telemetry.EventsFailedTotal.WithLabelValues("format").Inc()
		w.deadLetter(ctx, msg, fmt.Errorf("decode payload: %w", err))
		return
	}

	if err := events.Store(ctx, w.pool, e); err != nil {
		// Transient error — do not ACK; redelivered after MinIdleTime.
		telemetry.EventsFailedTotal.WithLabelValues("transient").Inc()
		slog.Error("worker: store event", "err", err,
			"msg_id", msg.ID,
			"project_id", e.ProjectID,
			"event", e.Event,
		)
		return
	}

	if err := events.UpsertDailyCount(ctx, w.pool, e.ProjectID, e.Event, e.Timestamp); err != nil {
		// Non-fatal: event is persisted; aggregate lag is acceptable.
		slog.Warn("worker: upsert daily count", "err", err, "project_id", e.ProjectID, "event", e.Event)
	}

	if err := w.consumer.Ack(ctx, msg.ID); err != nil {
		slog.Error("worker: ack message", "err", err, "msg_id", msg.ID)
	}

	telemetry.EventsProcessedTotal.WithLabelValues("success").Inc()
	telemetry.WorkerProcessingDurationSeconds.Observe(time.Since(start).Seconds())
}

func (w *Worker) deadLetter(ctx context.Context, msg queue.Message, reason error) {
	telemetry.EventsProcessedTotal.WithLabelValues("dead_letter").Inc()
	slog.Warn("worker: dead-lettering message",
		"msg_id", msg.ID,
		"delivery_count", msg.DeliveryCount,
		"reason", reason,
	)

	raw := json.RawMessage(msg.Payload)
	if !json.Valid(msg.Payload) {
		// Store raw bytes as a JSON string so the column stays valid JSONB.
		escaped, _ := json.Marshal(string(msg.Payload))
		raw = escaped
	}

	// Best-effort: extract project_id from the payload for scoped monitoring queries.
	// Left NULL when the payload is not parseable (format errors).
	var projectID *string
	var hdr struct {
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(msg.Payload, &hdr) == nil && hdr.ProjectID != "" {
		projectID = &hdr.ProjectID
	}

	_, dbErr := w.pool.Exec(ctx,
		`INSERT INTO dead_letter_events (project_id, raw_payload, error, attempt_count)
		 VALUES ($1, $2, $3, $4)`,
		projectID, raw, reason.Error(), msg.DeliveryCount,
	)
	if dbErr != nil {
		slog.Error("worker: insert dead letter", "err", dbErr, "msg_id", msg.ID)
		return
	}

	if err := w.consumer.Ack(ctx, msg.ID); err != nil {
		slog.Error("worker: ack dead-lettered message", "err", err, "msg_id", msg.ID)
	}
}

func decodeEvent(payload []byte) (*events.Event, error) {
	var e events.Event
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
