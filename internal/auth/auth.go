package auth

import "context"

type contextKey string

const (
	projectIDKey contextKey = "project_id"
	apiKeyIDKey  contextKey = "api_key_id"
)

func ProjectIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(projectIDKey).(string)
	return v, ok && v != ""
}

func WithProjectID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, projectIDKey, id)
}

func APIKeyIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(apiKeyIDKey).(string)
	return v, ok && v != ""
}

func WithAPIKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, apiKeyIDKey, id)
}
