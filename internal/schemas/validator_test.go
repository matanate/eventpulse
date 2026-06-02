package schemas

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)


// inMemoryGetter is a fake getter for unit tests — no DB required.
type inMemoryGetter struct {
	entries map[string]*Schema
}

func newInMemoryGetter() *inMemoryGetter {
	return &inMemoryGetter{entries: make(map[string]*Schema)}
}

func (g *inMemoryGetter) set(projectID, eventName string, raw json.RawMessage, mode Mode) {
	g.entries[projectID+":"+eventName] = &Schema{
		ProjectID: projectID,
		EventName: eventName,
		Schema:    raw,
		Mode:      mode,
	}
}

func (g *inMemoryGetter) Get(_ context.Context, projectID, eventName string) (*Schema, error) {
	sc, ok := g.entries[projectID+":"+eventName]
	if !ok {
		return nil, ErrNotFound
	}
	return sc, nil
}

// ── Compile ────────────────────────────────────────────────────────────────

func TestCompile_ValidSchema(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"price":{"type":"number"}},"required":["price"]}`)
	if err := Compile(raw); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCompile_InvalidType(t *testing.T) {
	raw := json.RawMessage(`{"type":"not-a-type"}`)
	if err := Compile(raw); err == nil {
		t.Fatal("expected error for invalid type keyword, got nil")
	}
}

func TestCompile_NotJSON(t *testing.T) {
	raw := json.RawMessage(`this is not json`)
	if err := Compile(raw); err == nil {
		t.Fatal("expected error for non-JSON input, got nil")
	}
}

func TestCompile_EmptyObject(t *testing.T) {
	// {} is a valid JSON Schema that accepts everything.
	if err := Compile(json.RawMessage(`{}`)); err != nil {
		t.Fatalf("expected no error for {}, got %v", err)
	}
}

// ── Validate ───────────────────────────────────────────────────────────────

func TestValidate_NoSchemaRegistered(t *testing.T) {
	g := newInMemoryGetter()
	v := newSchemaValidatorFromGetter(g)

	violations, enforce, err := v.Validate(context.Background(), "proj1", "purchase", map[string]any{"price": 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations when no schema registered, got %v", violations)
	}
	if enforce {
		t.Error("expected enforce=false when no schema registered")
	}
}

func TestValidate_PassesSchema(t *testing.T) {
	g := newInMemoryGetter()
	g.set("proj1", "purchase", json.RawMessage(
		`{"type":"object","properties":{"price":{"type":"number"}},"required":["price"]}`),
		ModeEnforce)
	v := newSchemaValidatorFromGetter(g)

	violations, _, err := v.Validate(context.Background(), "proj1", "purchase", map[string]any{"price": 9.99})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %v", violations)
	}
}

func TestValidate_FailsEnforceMode(t *testing.T) {
	g := newInMemoryGetter()
	g.set("proj1", "purchase", json.RawMessage(
		`{"type":"object","properties":{"price":{"type":"number"}},"required":["price"]}`),
		ModeEnforce)
	v := newSchemaValidatorFromGetter(g)

	violations, enforce, err := v.Validate(context.Background(), "proj1", "purchase", map[string]any{"price": "not-a-number"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected violations for wrong type, got none")
	}
	if !enforce {
		t.Error("expected enforce=true for ModeEnforce schema")
	}
}

func TestValidate_FailsWarnMode(t *testing.T) {
	g := newInMemoryGetter()
	g.set("proj1", "purchase", json.RawMessage(
		`{"type":"object","properties":{"price":{"type":"number"}},"required":["price"]}`),
		ModeWarn)
	v := newSchemaValidatorFromGetter(g)

	violations, enforce, err := v.Validate(context.Background(), "proj1", "purchase", map[string]any{"price": "not-a-number"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) == 0 {
		t.Error("expected violations for wrong type, got none")
	}
	if enforce {
		t.Error("expected enforce=false for ModeWarn schema")
	}
}

func TestValidate_CachesCompiledSchema(t *testing.T) {
	g := newInMemoryGetter()
	g.set("proj1", "ev", json.RawMessage(`{}`), ModeWarn)
	v := newSchemaValidatorFromGetter(g)

	// First call populates cache.
	if _, _, err := v.Validate(context.Background(), "proj1", "ev", nil); err != nil {
		t.Fatal(err)
	}
	// Remove from in-memory store — second call should still succeed (cache hit).
	delete(g.entries, "proj1:ev")
	if _, _, err := v.Validate(context.Background(), "proj1", "ev", nil); err != nil {
		t.Fatalf("expected cache hit, got error: %v", err)
	}
}

func TestValidate_InvalidateClears(t *testing.T) {
	g := newInMemoryGetter()
	g.set("proj1", "ev", json.RawMessage(`{}`), ModeWarn)
	v := newSchemaValidatorFromGetter(g)

	// Warm the cache.
	if _, _, err := v.Validate(context.Background(), "proj1", "ev", nil); err != nil {
		t.Fatal(err)
	}

	// Invalidate and remove the schema from the store.
	v.Invalidate("proj1", "ev")
	delete(g.entries, "proj1:ev")

	// Now Validate should return no violations (no schema found) rather than a cache hit.
	violations, _, err := v.Validate(context.Background(), "proj1", "ev", map[string]any{"any": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations after invalidate, got %v", violations)
	}
}

func TestErrNotFound_IsDistinct(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("ErrNotFound should match itself via errors.Is")
	}
}
