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
				IdempotencyKey: "key_abc",
				Properties:     map[string]any{"page": "/home"},
			},
			wantCount: 0,
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
			name:      "idempotency_key too long",
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
