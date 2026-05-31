package events_test

import (
	"strings"
	"testing"

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
