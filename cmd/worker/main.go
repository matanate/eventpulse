package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/matangi/eventpulse/internal/config"
	"github.com/matangi/eventpulse/internal/db"
	"github.com/matangi/eventpulse/internal/queue"
	rdb "github.com/matangi/eventpulse/internal/redis"
	"github.com/matangi/eventpulse/internal/telemetry"
	"github.com/matangi/eventpulse/internal/tracing"
	"github.com/matangi/eventpulse/internal/webhooks"
	"github.com/matangi/eventpulse/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	setupLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tracingShutdown, err := tracing.Setup(ctx, "worker", cfg.OTELEndpoint)
	if err != nil {
		slog.Error("tracing setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracingShutdown(shutdownCtx); err != nil {
			slog.Error("tracing shutdown error", "err", err)
		}
	}()

	pool, err := db.New(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient, err := rdb.New(cfg.RedisURL)
	if err != nil {
		slog.Error("redis connection failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			slog.Error("redis close error", "err", err)
		}
	}()

	consumerName := consumerID()
	consumer, err := queue.NewStreamConsumer(redisClient, consumerName)
	if err != nil {
		slog.Error("create stream consumer", "err", err)
		os.Exit(1)
	}

	// HTTP server — exposes /metrics for Prometheus and /healthz for Railway health checks.
	// Uses PORT env var (Railway convention) if set, otherwise falls back to METRICS_PORT.
	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = strconv.Itoa(cfg.MetricsPort)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	metricsSrv := &http.Server{
		Addr:    ":" + httpPort,
		Handler: mux,
	}
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "err", err)
		}
	}()

	// Queue lag tracker — polls XPENDING every 15s and updates the gauge.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := consumer.PendingCount(ctx); err == nil {
					telemetry.QueueLag.Set(float64(n))
				}
			}
		}
	}()

	enqueuer := webhooks.NewEnqueuer(pool)

	dispatcherCfg := webhooks.DispatcherConfig{
		PollInterval: cfg.WebhookPollInterval,
		BatchSize:    cfg.WebhookBatchSize,
		HTTPTimeout:  cfg.WebhookHTTPTimeout,
		MinInterval:  cfg.WebhookMinInterval,
		AllowHTTP:    cfg.IsDevelopment(),
		SecretKey:    cfg.WebhookSecretKey,
	}
	dispatcher := webhooks.NewDispatcher(pool, dispatcherCfg)
	go dispatcher.Run(ctx)

	w := worker.New(consumer, pool, cfg.WorkerConcurrency, enqueuer)

	slog.Info("worker starting",
		"env", cfg.Env,
		"concurrency", cfg.WorkerConcurrency,
		"consumer", consumerName,
		"metrics_port", cfg.MetricsPort,
	)

	w.Run(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("metrics server shutdown error", "err", err)
	}

	slog.Info("worker shutdown complete")
}

func consumerID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("worker-%s", host)
}

func setupLogger(cfg *config.Config) {
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if cfg.IsDevelopment() {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
