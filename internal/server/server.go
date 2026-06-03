package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/matanate/eventpulse/internal/config"
)

func New(cfg *config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}
}

func Shutdown(ctx context.Context, srv *http.Server) error {
	return srv.Shutdown(ctx)
}
