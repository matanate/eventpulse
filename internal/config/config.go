package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              string
	MetricsPort       int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ShutdownTimeout   time.Duration
	DatabaseURL       string
	RedisURL          string
	Env               string
	LogLevel          string
	WorkerConcurrency int
	DBMaxConns        int
	DBMinConns        int
	OTELEndpoint      string // optional: OTLP HTTP endpoint (e.g. http://jaeger:4318); empty disables tracing

	// Webhook dispatcher tunables — all optional with safe defaults.
	WebhookPollInterval time.Duration
	WebhookBatchSize    int
	WebhookHTTPTimeout  time.Duration
	WebhookMinInterval  time.Duration // minimum delay between delivery attempts per subscription
	WebhookSecretKey    []byte        // 32-byte AES-256 key for at-rest secret encryption (required)
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:              getEnv("APP_PORT", "8080"),
		MetricsPort:       getInt("METRICS_PORT", 8081),
		Env:               getEnv("APP_ENV", "development"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		ReadTimeout:       getDuration("APP_READ_TIMEOUT", 5*time.Second),
		WriteTimeout:      getDuration("APP_WRITE_TIMEOUT", 10*time.Second),
		ShutdownTimeout:   getDuration("APP_SHUTDOWN_TIMEOUT", 15*time.Second),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          os.Getenv("REDIS_URL"),
		WorkerConcurrency: getInt("WORKER_CONCURRENCY", 5),
		DBMaxConns:        getInt("DB_MAX_CONNS", 25),
		DBMinConns:        getInt("DB_MIN_CONNS", 5),
		OTELEndpoint:      os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),

		WebhookPollInterval: getDuration("WEBHOOK_POLL_INTERVAL", time.Second),
		WebhookBatchSize:    clamp(getInt("WEBHOOK_BATCH_SIZE", 50), 1, 500),
		WebhookHTTPTimeout:  getDuration("WEBHOOK_HTTP_TIMEOUT", 10*time.Second),
		WebhookMinInterval:  getDuration("WEBHOOK_MIN_INTERVAL", time.Second),
	}

	webhookKey, keyErr := hex.DecodeString(os.Getenv("WEBHOOK_SECRET_KEY"))
	if keyErr == nil && len(webhookKey) == 32 {
		cfg.WebhookSecretKey = webhookKey
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if len(cfg.WebhookSecretKey) != 32 {
		missing = append(missing, "WEBHOOK_SECRET_KEY (must be 64 hex chars, generate with: openssl rand -hex 32)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
