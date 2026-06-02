package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/matangi/eventpulse/internal/analytics"
	"github.com/matangi/eventpulse/internal/auth"
	"github.com/matangi/eventpulse/internal/config"
	"github.com/matangi/eventpulse/internal/db"
	"github.com/matangi/eventpulse/internal/events"
	"github.com/matangi/eventpulse/internal/health"
	"github.com/matangi/eventpulse/internal/queue"
	rdb "github.com/matangi/eventpulse/internal/redis"
	"github.com/matangi/eventpulse/internal/ratelimit"
	"github.com/matangi/eventpulse/internal/server"
	"github.com/matangi/eventpulse/internal/tracing"
	"github.com/matangi/eventpulse/internal/webhooks"
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

	tracingShutdown, err := tracing.Setup(ctx, "ingestion-api", cfg.OTELEndpoint)
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
	defer redisClient.Close()

	authMW := auth.NewMiddleware(pool)
	limiter := ratelimit.NewLimiter(redisClient, ratelimit.Config{
		Limit:  100,
		Window: time.Minute,
	})

	publisher := queue.NewStreamPublisher(redisClient)
	inspector := queue.NewInspector(redisClient)

	checker := health.NewChecker(pool, redisClient)
	eventHandler := events.NewHandler(publisher, pool)
	analyticsHandler := analytics.NewHandler(pool)
	queueStatsHandler := queue.NewStatsHandler(inspector, pool)
	webhookHandler := webhooks.NewHandler(pool, cfg.IsDevelopment(), cfg.WebhookSecretKey)
	router := server.NewRouter(checker, eventHandler, analyticsHandler, queueStatsHandler, webhookHandler, authMW, limiter.Middleware())
	// otelhttp is always installed; with the no-op provider it is a thin pass-through.
	// This ensures incoming traceparent headers create a root span even when no
	// exporter is configured, allowing child spans to be linked correctly.
	handler := otelhttp.NewHandler(router, "ingestion-api")
	srv := server.New(cfg, handler)

	slog.Info("ingestion-api starting", "port", cfg.Port, "env", cfg.Env)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			slog.Info("server stopped", "reason", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx, srv); err != nil {
		slog.Error("shutdown error", "err", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
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
