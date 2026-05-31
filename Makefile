GOPATH := $(shell go env GOPATH)
GO     := go
BIN    := bin

.PHONY: run-api run-worker infra-up infra-down infra-obs migrate migrate-down \
        test test-integration lint build clean seed seed-demo loadtest

## ── Infrastructure ───────────────────────────────────────────────────────────

infra-up:
	docker compose -f deploy/docker-compose.yml up -d

infra-down:
	docker compose -f deploy/docker-compose.yml down

infra-obs:
	docker compose -f deploy/docker-compose.yml --profile observability up -d

## ── Migrations ───────────────────────────────────────────────────────────────
# Install: go install -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

migrate:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1

## ── Run ──────────────────────────────────────────────────────────────────────

run-api:
	$(GO) run ./cmd/ingestion-api

run-worker:
	$(GO) run ./cmd/worker

## ── Build ────────────────────────────────────────────────────────────────────

build:
	$(GO) build -o $(BIN)/ingestion-api ./cmd/ingestion-api
	$(GO) build -o $(BIN)/worker ./cmd/worker

## ── Test ─────────────────────────────────────────────────────────────────────

test:
	$(GO) test ./...

test-integration:
	$(GO) test -tags integration ./...

test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## ── Lint ─────────────────────────────────────────────────────────────────────
# Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

lint:
	golangci-lint run ./...

## ── Seed ─────────────────────────────────────────────────────────────────────
# SEED_COUNT: number of API keys to generate (default 1; use 50+ for load testing)

seed:
	$(GO) run ./cmd/seed -count $${SEED_COUNT:-1}

# seed-demo: creates or reuses "Demo Account"/"EventPulse Demo" project and
# generates 3 fresh API keys for the Railway live showcase. Run once; save output.
# Requires DATABASE_URL pointing at the Railway Postgres instance.
seed-demo:
	$(GO) run ./cmd/seed -demo

## ── Loadtest ─────────────────────────────────────────────────────────────────
# Install k6: https://k6.io/docs/getting-started/installation/
# Requires: export EVENTPULSE_API_KEYS=<comma-separated keys from `make seed SEED_COUNT=50`>
# Optional: export SCENARIO=ratelimited  (default: throughput)

loadtest:
	@test -n "$$EVENTPULSE_API_KEYS" || \
		(echo "Error: EVENTPULSE_API_KEYS not set. Run: make seed SEED_COUNT=50" && exit 1)
	k6 run loadtest/k6-events.js

## ── Clean ────────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BIN) coverage.out coverage.html
