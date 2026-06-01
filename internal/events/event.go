package events

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	maxEventNameLen      = 255
	maxUserIDLen         = 255
	maxIdempotencyKeyLen = 255
	maxPropertiesKeys    = 50
	maxPropertiesBytes   = 4 * 1024 // 4 KiB encoded
	maxTimestampPast     = 24 * time.Hour
	maxTimestampFuture   = time.Minute
)

type Event struct {
	ID             string
	ProjectID      string
	Event          string
	UserID         string
	Properties     map[string]any
	IdempotencyKey string
	Timestamp      time.Time
	ReceivedAt     time.Time
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *Event) Validate() []ValidationError {
	var errs []ValidationError

	if strings.TrimSpace(e.Event) == "" {
		errs = append(errs, ValidationError{Field: "event", Message: "event name is required"})
	} else if len(e.Event) > maxEventNameLen {
		errs = append(errs, ValidationError{Field: "event", Message: "event name must not exceed 255 characters"})
	}

	if len(e.UserID) > maxUserIDLen {
		errs = append(errs, ValidationError{Field: "user_id", Message: "user_id must not exceed 255 characters"})
	}

	if len(e.IdempotencyKey) > maxIdempotencyKeyLen {
		errs = append(errs, ValidationError{Field: "idempotency_key", Message: "idempotency_key must not exceed 255 characters"})
	}

	if len(e.Properties) > maxPropertiesKeys {
		errs = append(errs, ValidationError{Field: "properties", Message: "properties must not exceed 50 top-level keys"})
	} else if len(e.Properties) > 0 {
		encoded, err := json.Marshal(e.Properties)
		if err != nil || len(encoded) > maxPropertiesBytes {
			errs = append(errs, ValidationError{Field: "properties", Message: "properties must not exceed 4 KiB when encoded"})
		}
	}

	if !e.Timestamp.IsZero() {
		now := time.Now().UTC()
		if e.Timestamp.Before(now.Add(-maxTimestampPast)) {
			errs = append(errs, ValidationError{Field: "timestamp", Message: "timestamp must not be more than 24 hours in the past"})
		} else if e.Timestamp.After(now.Add(maxTimestampFuture)) {
			errs = append(errs, ValidationError{Field: "timestamp", Message: "timestamp must not be more than 1 minute in the future"})
		}
	}

	return errs
}
