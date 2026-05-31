package events

import (
	"strings"
	"time"
)

const (
	maxEventNameLen      = 255
	maxUserIDLen         = 255
	maxIdempotencyKeyLen = 255
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

	return errs
}
