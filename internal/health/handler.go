package health

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Checker struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewChecker(db *pgxpool.Pool, redis *redis.Client) *Checker {
	return &Checker{db: db, redis: redis}
}

func (c *Checker) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (c *Checker) Readyz(w http.ResponseWriter, r *http.Request) {
	deps := map[string]string{}
	allOK := true

	if err := c.db.Ping(context.Background()); err != nil {
		deps["postgres"] = err.Error()
		allOK = false
	} else {
		deps["postgres"] = "ok"
	}

	if err := c.redis.Ping(context.Background()).Err(); err != nil {
		deps["redis"] = err.Error()
		allOK = false
	} else {
		deps["redis"] = "ok"
	}

	if allOK {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}

	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"status":       "error",
		"dependencies": deps,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
