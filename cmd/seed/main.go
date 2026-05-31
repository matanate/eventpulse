package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	count := flag.Int("count", 1, "number of API keys to generate (all share one account + project)")
	demo := flag.Bool("demo", false, "seed a stable demo account/project with 3 API keys for the live showcase")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if *demo {
		return runDemo(ctx, pool)
	}

	if *count < 1 {
		return fmt.Errorf("-count must be >= 1")
	}
	return runSeed(ctx, pool, *count)
}

func runDemo(ctx context.Context, pool *pgxpool.Pool) error {
	accountID, err := findOrCreateAccount(ctx, pool, "Demo Account")
	if err != nil {
		return fmt.Errorf("demo account: %w", err)
	}

	projectID, err := findOrCreateProject(ctx, pool, accountID, "EventPulse Demo")
	if err != nil {
		return fmt.Errorf("demo project: %w", err)
	}

	const keyCount = 3
	keys := make([]string, keyCount)
	for i := range keys {
		rawKey, err := generateKey()
		if err != nil {
			return fmt.Errorf("generate demo key %d: %w", i+1, err)
		}
		hash := hashKey(rawKey)
		prefix := rawKey[:12]
		if _, err := pool.Exec(ctx,
			`INSERT INTO api_keys (project_id, key_hash, prefix) VALUES ($1, $2, $3)`,
			projectID, hash, prefix,
		); err != nil {
			return fmt.Errorf("insert demo key %d: %w", i+1, err)
		}
		keys[i] = rawKey
	}

	fmt.Println("Demo seed complete. Save these values — keys will not be shown again.")
	fmt.Println()
	fmt.Printf("Account ID:   %s\n", accountID)
	fmt.Printf("Project ID:   %s\n", projectID)
	fmt.Println()
	for i, k := range keys {
		fmt.Printf("API Key %d:    %s\n", i+1, k)
	}
	fmt.Println()
	fmt.Println("Railway env vars:")
	fmt.Printf("  DEMO_PROJECT_ID=%s\n", projectID)
	fmt.Printf("  DEMO_API_KEY=%s\n", keys[0])
	fmt.Println()
	fmt.Println("Dashboard env vars (Cloudflare Pages):")
	fmt.Printf("  VITE_DEMO_PROJECT_ID=%s\n", projectID)
	fmt.Printf("  VITE_DEMO_API_KEY=%s\n", keys[0])
	return nil
}

func findOrCreateAccount(ctx context.Context, pool *pgxpool.Pool, name string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `SELECT id FROM accounts WHERE name = $1 LIMIT 1`, name).Scan(&id)
	if err == nil {
		fmt.Printf("Using existing account: %s\n", id)
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ($1) RETURNING id`, name,
	).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func findOrCreateProject(ctx context.Context, pool *pgxpool.Pool, accountID, name string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`SELECT id FROM projects WHERE account_id = $1 AND name = $2 LIMIT 1`, accountID, name,
	).Scan(&id)
	if err == nil {
		fmt.Printf("Using existing project: %s\n", id)
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, $2) RETURNING id`, accountID, name,
	).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func runSeed(ctx context.Context, pool *pgxpool.Pool, count int) error {
	var accountID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO accounts (name) VALUES ('Dev Account') RETURNING id`,
	).Scan(&accountID); err != nil {
		return fmt.Errorf("insert account: %w", err)
	}

	var projectID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO projects (account_id, name) VALUES ($1, 'Dev Project') RETURNING id`,
		accountID,
	).Scan(&projectID); err != nil {
		return fmt.Errorf("insert project: %w", err)
	}

	rawKeys := make([]string, count)
	for i := range rawKeys {
		rawKey, err := generateKey()
		if err != nil {
			return fmt.Errorf("generate key %d: %w", i, err)
		}

		hash := hashKey(rawKey)
		prefix := rawKey[:12]

		var apiKeyID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO api_keys (project_id, key_hash, prefix) VALUES ($1, $2, $3) RETURNING id`,
			projectID, hash, prefix,
		).Scan(&apiKeyID); err != nil {
			return fmt.Errorf("insert api_key %d: %w", i, err)
		}
		rawKeys[i] = rawKey
	}

	if count == 1 {
		fmt.Println("Seed complete. Store the API key — it will not be shown again.")
		fmt.Println()
		fmt.Printf("Account ID:  %s\n", accountID)
		fmt.Printf("Project ID:  %s\n", projectID)
		fmt.Printf("API Key:     %s\n", rawKeys[0])
		fmt.Println()
		fmt.Printf("Usage:  Authorization: Bearer %s\n", rawKeys[0])
	} else {
		fmt.Printf("Seeded %d API keys for project %s\n\n", count, projectID)
		fmt.Printf("Account ID:  %s\n", accountID)
		fmt.Printf("Project ID:  %s\n", projectID)
		fmt.Println()
		fmt.Println("Set for load testing:")
		fmt.Printf("  export EVENTPULSE_API_KEYS=%s\n", strings.Join(rawKeys, ","))
		fmt.Printf("  export EVENTPULSE_PROJECT_ID=%s\n", projectID)
	}

	return nil
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
