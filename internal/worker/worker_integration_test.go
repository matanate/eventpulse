//go:build integration

package worker_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/matanate/eventpulse/internal/events"
	"github.com/matanate/eventpulse/internal/queue"
	"github.com/matanate/eventpulse/internal/worker"
)

var (
	testPool      *pgxpool.Pool
	testRedis     *redis.Client
	testProjectID string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	pgCtr, err := tcpostgres.Run(ctx,
		"postgres:18.4-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		return 1
	}
	defer pgCtr.Terminate(ctx) //nolint:errcheck

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		return 1
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect pool: %v\n", err)
		return 1
	}
	defer testPool.Close()

	if err := runMigrations(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "migrations: %v\n", err)
		return 1
	}

	testProjectID, err = seedProject(ctx, testPool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed project: %v\n", err)
		return 1
	}

	redisC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:8.8-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start redis: %v\n", err)
		return 1
	}
	defer redisC.Terminate(ctx) //nolint:errcheck

	redisHost, err := redisC.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis host: %v\n", err)
		return 1
	}
	redisPort, err := redisC.MappedPort(ctx, "6379")
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis port: %v\n", err)
		return 1
	}

	testRedis = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort.Port()),
	})
	defer testRedis.Close()

	return m.Run()
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

// TestWorker_ProcessesEvent publishes an event via StreamPublisher, runs the worker
// briefly, and verifies the event landed in the events table.
func TestWorker_ProcessesEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Each test uses a unique Redis stream name to avoid cross-test interference.
	streamName := fmt.Sprintf("test-stream-%d", time.Now().UnixNano())
	pub, consumer := newTestComponents(t, streamName)

	e := &events.Event{
		ID:         "worker-test-event-id",
		ProjectID:  testProjectID,
		Event:      "test_event",
		UserID:     "u1",
		ReceivedAt: time.Now().UTC(),
		Timestamp:  time.Now().UTC(),
	}

	if err := pub.Publish(ctx, e); err != nil {
		t.Fatalf("publish: %v", err)
	}

	w := worker.New(consumer, testPool, 1, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	// Wait for the worker to process the event (up to 10 seconds).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM events WHERE project_id = $1 AND event = 'test_event'`,
			testProjectID,
		).Scan(&n)
		if n > 0 {
			cancel()
			<-done
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	cancel()
	<-done
	t.Fatal("event did not appear in database within 10s")
}

// TestWorker_DeadLetters_MalformedPayload directly adds an invalid JSON message to the
// stream and verifies the worker writes it to dead_letter_events.
func TestWorker_DeadLetters_MalformedPayload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamName := fmt.Sprintf("test-stream-dl-%d", time.Now().UnixNano())
	_, consumer := newTestComponents(t, streamName)

	// Add a malformed payload directly to the stream, bypassing the Publisher.
	if err := testRedis.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{"payload": "not-valid-json"},
	}).Err(); err != nil {
		t.Fatalf("xadd: %v", err)
	}

	w := worker.New(consumer, testPool, 1, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	// Wait for the dead letter row to appear (up to 10 seconds).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM dead_letter_events WHERE error LIKE '%decode payload%'`,
		).Scan(&n)
		if n > 0 {
			cancel()
			<-done
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	cancel()
	<-done
	t.Fatal("dead letter event did not appear within 10s")
}

// ג”€ג”€ Helpers ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

// newTestComponents creates a StreamPublisher and StreamConsumer backed by a
// dedicated stream (so tests don't interfere with each other).
func newTestComponents(t *testing.T, streamName string) (*queue.StreamPublisher, queue.Consumer) {
	t.Helper()

	// Monkey-patch the stream name via a thin wrapper consumer so each test
	// gets an isolated Redis stream.
	pub := queue.NewStreamPublisherWithStream(testRedis, streamName)
	consumer, err := queue.NewStreamConsumerWithStream(testRedis, "test-worker", streamName)
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	return pub, consumer
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}
	migrationsDir := filepath.Join(wd, "..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		sql, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func seedProject(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var accountID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('Worker Test Account') RETURNING id`,
	).Scan(&accountID); err != nil {
		return "", fmt.Errorf("insert account: %w", err)
	}
	var projectID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, 'Worker Test Project') RETURNING id`,
		accountID,
	).Scan(&projectID); err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}
	return projectID, nil
}
