package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	flag.Parse()

	if *count < 1 {
		return fmt.Errorf("-count must be >= 1")
	}

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

	rawKeys := make([]string, *count)
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

	if *count == 1 {
		fmt.Println("Seed complete. Store the API key — it will not be shown again.")
		fmt.Println()
		fmt.Printf("Account ID:  %s\n", accountID)
		fmt.Printf("Project ID:  %s\n", projectID)
		fmt.Printf("API Key:     %s\n", rawKeys[0])
		fmt.Println()
		fmt.Printf("Usage:  Authorization: Bearer %s\n", rawKeys[0])
	} else {
		fmt.Printf("Seeded %d API keys for project %s\n\n", *count, projectID)
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
