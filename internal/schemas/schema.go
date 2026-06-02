package schemas

import (
	"context"
	"encoding/json"
	"time"
)

// Mode controls what happens when properties fail JSON Schema validation.
type Mode string

const (
	ModeEnforce Mode = "enforce" // reject the event with 422
	ModeWarn    Mode = "warn"    // accept but emit a metric
)

// Schema represents a registered JSON Schema for a named event within a project.
type Schema struct {
	ID        string
	ProjectID string
	EventName string
	Schema    json.RawMessage
	Mode      Mode
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validator validates event properties against a registered JSON Schema.
// Validate returns (nil, _, nil) when no schema is registered.
// Violations is non-empty when properties fail validation.
// enforce=true means the caller should reject the event; false means warn only.
type Validator interface {
	Validate(ctx context.Context, projectID, eventName string, properties map[string]any) (violations []string, enforce bool, err error)
}
