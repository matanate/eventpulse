package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matangi/eventpulse/internal/api"
)

type Middleware struct {
	pool *pgxpool.Pool
}

func NewMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	m := &Middleware{pool: pool}
	return m.handler
}

func (m *Middleware) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken, ok := bearerToken(r)
		if !ok {
			api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing API key")
			return
		}

		hash := hashToken(rawToken)

		var apiKeyID, projectID string
		err := m.pool.QueryRow(r.Context(),
			`SELECT id, project_id FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`,
			hash,
		).Scan(&apiKeyID, &projectID)
		if err != nil {
			api.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing API key")
			return
		}

		ctx := WithProjectID(r.Context(), projectID)
		ctx = WithAPIKeyID(ctx, apiKeyID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(h, "Bearer ")
	if token == "" {
		return "", false
	}
	return token, true
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
