package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/matangi/eventpulse/internal/analytics"
	"github.com/matangi/eventpulse/internal/events"
	"github.com/matangi/eventpulse/internal/health"
	"github.com/matangi/eventpulse/internal/telemetry"
)

func NewRouter(
	checker *health.Checker,
	eventHandler *events.Handler,
	analyticsHandler *analytics.Handler,
	authMW func(http.Handler) http.Handler,
	rlMW func(http.Handler) http.Handler,
) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{
			"https://*.pages.dev",
			"https://*.cloudflare.com",
			"https://eventpulse.atedgimatan.com",
			"http://localhost:5173",
		},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
		MaxAge:         300,
	}))
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(telemetry.RequestLogger)
	r.Use(telemetry.RequestDuration)

	r.Get("/healthz", checker.Healthz)
	r.Get("/readyz", checker.Readyz)
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/v1", func(r chi.Router) {
		r.Use(authMW)
		r.Use(rlMW)

		r.Post("/events", eventHandler.HandleIngest)
		r.Post("/events/batch", eventHandler.HandleBatchIngest)

		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Get("/stats", analyticsHandler.HandleStats)
			r.Get("/events", analyticsHandler.HandleListEvents)
			r.Get("/events/top", analyticsHandler.HandleTopEvents)
			r.Route("/users/{userID}", func(r chi.Router) {
				r.Get("/events", analyticsHandler.HandleUserEvents)
			})
		})
	})

	return r
}
