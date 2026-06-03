package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/matanate/eventpulse/internal/analytics"
	"github.com/matanate/eventpulse/internal/docs"
	"github.com/matanate/eventpulse/internal/events"
	"github.com/matanate/eventpulse/internal/health"
	"github.com/matanate/eventpulse/internal/queue"
	"github.com/matanate/eventpulse/internal/schemas"
	"github.com/matanate/eventpulse/internal/sse"
	"github.com/matanate/eventpulse/internal/telemetry"
	"github.com/matanate/eventpulse/internal/webhooks"
)

func NewRouter(
	checker *health.Checker,
	eventHandler *events.Handler,
	analyticsHandler *analytics.Handler,
	queueStatsHandler *queue.StatsHandler,
	webhookHandler *webhooks.Handler,
	sseHandler *sse.Handler,
	schemaHandler *schemas.Handler,
	authMW func(http.Handler) http.Handler,
	rlMW func(http.Handler) http.Handler,
) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{
			"https://eventpulse.pages.dev",
			"https://eventpulse.atedgimatan.com",
			"http://localhost:5173",
		},
		AllowedMethods: []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type", "Idempotency-Key"},
		MaxAge:         300,
	}))
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(telemetry.RequestLogger)
	r.Use(telemetry.RequestDuration)

	r.Get("/healthz", checker.Healthz)
	r.Get("/readyz", checker.Readyz)
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/openapi.json", docs.HandleSpec)

	r.Route("/v1", func(r chi.Router) {
		r.Use(authMW)

		// Admin endpoints: auth-protected but exempt from per-key rate limiting.
		r.Get("/admin/queue/stats", queueStatsHandler.HandleQueueStats)

		// SSE stream: auth-protected but exempt from rate limiting (long-lived connection).
		r.Get("/projects/{projectID}/stream", sseHandler.Handle)

		r.Group(func(r chi.Router) {
			r.Use(rlMW)

			r.Post("/events", eventHandler.HandleIngest)
			r.Post("/events/batch", eventHandler.HandleBatchIngest)

			r.Post("/webhooks", webhookHandler.HandleCreate)
			r.Get("/webhooks", webhookHandler.HandleList)
			r.Delete("/webhooks/{id}", webhookHandler.HandleDelete)

			r.Route("/projects/{projectID}", func(r chi.Router) {
				r.Get("/stats", analyticsHandler.HandleStats)
				r.Get("/events", analyticsHandler.HandleListEvents)
				r.Get("/events/top", analyticsHandler.HandleTopEvents)
				r.Post("/funnels", analyticsHandler.HandleFunnel)
				r.Get("/retention", analyticsHandler.HandleRetention)
				r.Route("/users/{userID}", func(r chi.Router) {
					r.Get("/events", analyticsHandler.HandleUserEvents)
				})
				r.Route("/schemas", func(r chi.Router) {
					r.Get("/", schemaHandler.HandleList)
					r.Post("/{event}", schemaHandler.HandleUpsert)
					r.Get("/{event}", schemaHandler.HandleGet)
					r.Delete("/{event}", schemaHandler.HandleDelete)
				})
			})
		})
	})

	return r
}
