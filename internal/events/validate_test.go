package events_test

import (
	"strings"
	"testing"
	"time"

	"github.com/matangi/eventpulse/internal/events"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		event     events.Event
		wantCount int
		wantField string
	}{
		{
			name:      "valid minimal event",
			event:     events.Event{Event: "user_signed_up"},
			wantCount: 0,
		},
		{
			name: "valid full event",
			event: events.Event{
				Event:          "page_view",
				UserID:         "usr_123",
				IdempotencyKey: "550e8400-e29b-41d4-a716-446655440000",
				Properties:     map[string]any{"page": "/home"},
			},
			wantCount: 0,
		},
		{
			name:      "valid UUID idempotency_key",
			event:     events.Event{Event: "test", IdempotencyKey: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
			wantCount: 0,
		},
		{
			name:      "empty idempotency_key is valid",
			event:     events.Event{Event: "test", IdempotencyKey: ""},
			wantCount: 0,
		},
		{
			name:      "idempotency_key not a valid UUID",
			event:     events.Event{Event: "test", IdempotencyKey: "not-a-uuid"},
			wantCount: 1,
			wantField: "idempotency_key",
		},
		{
			name:      "empty event name",
			event:     events.Event{Event: ""},
			wantCount: 1,
			wantField: "event",
		},
		{
			name:      "whitespace-only event name",
			event:     events.Event{Event: "   "},
			wantCount: 1,
			wantField: "event",
		},
		{
			name:      "event name too long",
			event:     events.Event{Event: strings.Repeat("a", 256)},
			wantCount: 1,
			wantField: "event",
		},
		{
			name:      "user_id too long",
			event:     events.Event{Event: "test", UserID: strings.Repeat("u", 256)},
			wantCount: 1,
			wantField: "user_id",
		},
		{
			name:      "idempotency_key long non-UUID string",
			event:     events.Event{Event: "test", IdempotencyKey: strings.Repeat("k", 256)},
			wantCount: 1,
			wantField: "idempotency_key",
		},
		{
			name: "multiple errors",
			event: events.Event{
				Event:  "",
				UserID: strings.Repeat("u", 256),
			},
			wantCount: 2,
		},
		// properties size validation
		{
			name: "properties too many keys",
			event: events.Event{
				Event: "test",
				Properties: func() map[string]any {
					m := make(map[string]any, 51)
					for i := range 51 {
						m[strings.Repeat("k", i+1)] = "v"
					}
					return m
				}(),
			},
			wantCount: 1,
			wantField: "properties",
		},
		{
			name: "properties encoded too large",
			event: events.Event{
				Event: "test",
				Properties: map[string]any{
					"data": strings.Repeat("x", 5*1024),
				},
			},
			wantCount: 1,
			wantField: "properties",
		},
		{
			name: "properties exactly 50 keys is valid",
			event: events.Event{
				Event: "test",
				Properties: func() map[string]any {
					m := make(map[string]any, 50)
					for i := range 50 {
						m[strings.Repeat("k", i+1)] = "v"
					}
					return m
				}(),
			},
			wantCount: 0,
		},
		// timestamp bounds validation
		{
			name: "timestamp too far in the past",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Now().Add(-25 * time.Hour),
			},
			wantCount: 1,
			wantField: "timestamp",
		},
		{
			name: "timestamp too far in the future",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Now().Add(2 * time.Minute),
			},
			wantCount: 1,
			wantField: "timestamp",
		},
		{
			name: "timestamp at boundary past is valid",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Now().Add(-23 * time.Hour),
			},
			wantCount: 0,
		},
		{
			name: "timestamp at boundary future is valid",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Now().Add(30 * time.Second),
			},
			wantCount: 0,
		},
		{
			name: "zero timestamp is valid (uses server time)",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Time{},
			},
			wantCount: 0,
		},
		// properties: both violations reported independently (not else-if)
		{
			name: "properties 51 keys and oversized both reported",
			event: events.Event{
				Event: "test",
				Properties: func() map[string]any {
					m := make(map[string]any, 51)
					for i := range 51 {
						m[strings.Repeat("k", i+1)] = strings.Repeat("v", 200)
					}
					return m
				}(),
			},
			wantCount: 2,
		},
		// properties: exactly at 4 KiB boundary
		{
			name: "properties at exactly 4096 bytes encoded is valid",
			event: events.Event{
				Event: "test",
				Properties: func() map[string]any {
					// Build a value that brings the total encoding to ≤ 4096 bytes.
					// {"k":"<val>"} base is 8 bytes; pad val to fill up to 4096.
					val := strings.Repeat("x", 4096-len(`{"k":""}`))
					return map[string]any{"k": val}
				}(),
			},
			wantCount: 0,
		},
		// timestamp: exact boundary edge cases
		{
			name: "timestamp at exactly -24h boundary is valid",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Now().Add(-24*time.Hour + time.Second),
			},
			wantCount: 0,
		},
		{
			name: "timestamp just past -24h is invalid",
			event: events.Event{
				Event:     "test",
				Timestamp: time.Now().Add(-24*time.Hour - time.Second),
			},
			wantCount: 1,
			wantField: "timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.event.Validate()
			if len(errs) != tt.wantCount {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tt.wantCount, errs)
			}
			if tt.wantField != "" && len(errs) > 0 && errs[0].Field != tt.wantField {
				t.Errorf("first error on field %q, want %q", errs[0].Field, tt.wantField)
			}
		})
	}
}
