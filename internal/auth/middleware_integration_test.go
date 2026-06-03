//go:build integration

package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/matanate/eventpulse/internal/auth"
	"github.com/matanate/eventpulse/internal/ratelimit"
)

var (
	testPool      *pgxpool.Pool
	testRedis     *redis.Client
	testServer    *httptest.Server
	validAPIKey   string
	revokedAPIKey string
	rlAPIKey      string
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

	validAPIKey, revokedAPIKey, rlAPIKey, err = seedKeys(ctx, testPool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		return 1
	}

	// High rate limit so auth tests are never blocked by the limiter.
	testServer = newTestServer(testPool, testRedis, ratelimit.Config{Limit: 10000, Window: time.Minute})
	defer testServer.Close()

	return m.Run()
}

// newTestServer builds a chi router with auth + rate limit middleware and a simple OK handler.
func newTestServer(pool *pgxpool.Pool, rdb *redis.Client, rlCfg ratelimit.Config) *httptest.Server {
	r := chi.NewRouter()
	r.Use(auth.NewMiddleware(pool))
	r.Use(ratelimit.NewLimiter(rdb, rlCfg).Middleware())
	r.Post("/v1/events", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(r)
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

func seedKeys(ctx context.Context, pool *pgxpool.Pool) (validKey, revokedKey, rlKey string, err error) {
	var accountID string
	if err = pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('Auth Test Account') RETURNING id`,
	).Scan(&accountID); err != nil {
		return "", "", "", fmt.Errorf("insert account: %w", err)
	}
	var projectID string
	if err = pool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, 'Auth Test Project') RETURNING id`,
		accountID,
	).Scan(&projectID); err != nil {
		return "", "", "", fmt.Errorf("insert project: %w", err)
	}

	if validKey, err = insertKey(ctx, pool, projectID, false); err != nil {
		return "", "", "", fmt.Errorf("valid key: %w", err)
	}
	if revokedKey, err = insertKey(ctx, pool, projectID, true); err != nil {
		return "", "", "", fmt.Errorf("revoked key: %w", err)
	}
	if rlKey, err = insertKey(ctx, pool, projectID, false); err != nil {
		return "", "", "", fmt.Errorf("rl key: %w", err)
	}
	return validKey, revokedKey, rlKey, nil
}

func insertKey(ctx context.Context, pool *pgxpool.Pool, projectID string, revoked bool) (string, error) {
	raw, err := generateKey()
	if err != nil {
		return "", err
	}
	hash := hashKey(raw)
	prefix := raw[:12]

	if revoked {
		_, err = pool.Exec(ctx,
			`INSERT INTO api_keys (project_id, key_hash, prefix, revoked_at) VALUES ($1, $2, $3, NOW())`,
			projectID, hash, prefix,
		)
	} else {
		_, err = pool.Exec(ctx,
			`INSERT INTO api_keys (project_id, key_hash, prefix) VALUES ($1, $2, $3)`,
			projectID, hash, prefix,
		)
	}
	if err != nil {
		return "", fmt.Errorf("insert key: %w", err)
	}
	return raw, nil
}

func generateKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "epk_" + hex.EncodeToString(b), nil
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func doRequest(t *testing.T, srv *httptest.Server, authHeader string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// ג”€ג”€ Tests ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€ג”€

func TestMiddleware_ValidKey(t *testing.T) {
	resp := doRequest(t, testServer, "Bearer "+validAPIKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	resp := doRequest(t, testServer, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestMiddleware_InvalidScheme(t *testing.T) {
	resp := doRequest(t, testServer, "Basic dXNlcjpwYXNz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestMiddleware_UnknownKey(t *testing.T) {
	resp := doRequest(t, testServer, "Bearer epk_000000000000000000000000000000000000")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestMiddleware_RevokedKey(t *testing.T) {
	resp := doRequest(t, testServer, "Bearer "+revokedAPIKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestRateLimit_ExceedsLimit(t *testing.T) {
	const limit = 3
	rlServer := newTestServer(testPool, testRedis, ratelimit.Config{Limit: limit, Window: time.Minute})
	defer rlServer.Close()

	for i := 0; i < limit; i++ {
		resp := doRequest(t, rlServer, "Bearer "+rlAPIKey)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, resp.StatusCode)
		}
	}

	resp := doRequest(t, rlServer, "Bearer "+rlAPIKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("request %d: got %d, want 429", limit+1, resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}
