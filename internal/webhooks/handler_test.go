package webhooks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests for validation-only paths that don't require a database.

func TestHandleCreate_MissingAuth(t *testing.T) {
	h := NewHandler(nil, true, testKey)
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.HandleCreate(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleList_MissingAuth(t *testing.T) {
	h := NewHandler(nil, true, testKey)
	r := httptest.NewRequest(http.MethodGet, "/v1/webhooks", nil)
	w := httptest.NewRecorder()
	h.HandleList(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleDelete_MissingAuth(t *testing.T) {
	h := NewHandler(nil, true, testKey)
	r := httptest.NewRequest(http.MethodDelete, "/v1/webhooks/some-id", nil)
	w := httptest.NewRecorder()
	h.HandleDelete(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestIsValidUUID(t *testing.T) {
	valid := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"00000000-0000-0000-0000-000000000000",
	}
	for _, s := range valid {
		if !isValidUUID(s) {
			t.Errorf("expected %q to be valid UUID", s)
		}
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"550e8400-e29b-41d4-a716-44665544000g", // invalid char
		"550e8400e29b41d4a716446655440000",      // no dashes
	}
	for _, s := range invalid {
		if isValidUUID(s) {
			t.Errorf("expected %q to be invalid UUID", s)
		}
	}
}
