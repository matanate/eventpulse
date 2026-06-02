package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"golang.org/x/sync/singleflight"
)

// getter is the minimal read interface SchemaValidator needs from the store.
type getter interface {
	Get(ctx context.Context, projectID, eventName string) (*Schema, error)
}

// SchemaValidator compiles JSON Schemas and validates event properties.
// Compiled schemas are cached in-process; call Invalidate after any upsert or delete.
// singleflight ensures at most one in-flight compile per (projectID, eventName) on cache miss.
type SchemaValidator struct {
	store getter
	mu    sync.Mutex
	cache map[string]*compiledEntry
	sf    singleflight.Group
}

type compiledEntry struct {
	compiled *jsonschema.Schema
	mode     Mode
}

// NewSchemaValidator creates a validator backed by the given store.
func NewSchemaValidator(store *Store) *SchemaValidator {
	return &SchemaValidator{
		store: store,
		cache: make(map[string]*compiledEntry),
	}
}

// newSchemaValidatorFromGetter creates a validator backed by any getter implementation.
// Used in tests.
func newSchemaValidatorFromGetter(g getter) *SchemaValidator {
	return &SchemaValidator{
		store: g,
		cache: make(map[string]*compiledEntry),
	}
}

// Compile parses and validates that raw is a well-formed JSON Schema.
// Returns an error suitable for returning as a 400 to the API caller.
func Compile(raw json.RawMessage) error {
	c := jsonschema.NewCompiler()
	c.ExtractAnnotations = false
	if err := c.AddResource("schema.json", bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("invalid JSON Schema: %w", err)
	}
	if _, err := c.Compile("schema.json"); err != nil {
		return fmt.Errorf("invalid JSON Schema: %w", err)
	}
	return nil
}

// Validate checks properties against the registered schema for (projectID, eventName).
// Returns (nil, false, nil) when no schema is registered — caller should accept the event.
func (v *SchemaValidator) Validate(ctx context.Context, projectID, eventName string, properties map[string]any) (violations []string, enforce bool, err error) {
	entry, err := v.resolveEntry(ctx, projectID, eventName)
	if err != nil {
		return nil, false, err
	}
	if entry == nil {
		return nil, false, nil
	}

	// Marshal properties to JSON for the jsonschema library.
	b, err := json.Marshal(properties)
	if err != nil {
		return nil, false, fmt.Errorf("marshal properties: %w", err)
	}

	var iface any
	if err := json.Unmarshal(b, &iface); err != nil {
		return nil, false, fmt.Errorf("unmarshal properties: %w", err)
	}

	if err := entry.compiled.Validate(iface); err != nil {
		violations = collectViolations(err)
	}

	return violations, entry.mode == ModeEnforce, nil
}

// collectViolations walks the validation error tree to collect leaf-level messages.
func collectViolations(err error) []string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []string{err.Error()}
	}
	if len(ve.Causes) == 0 {
		return []string{ve.Error()}
	}
	var out []string
	for _, cause := range ve.Causes {
		out = append(out, collectViolations(cause)...)
	}
	return out
}

// Invalidate removes the cached compiled schema for (projectID, eventName).
// Call after any upsert or delete.
func (v *SchemaValidator) Invalidate(projectID, eventName string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.cache, cacheKey(projectID, eventName))
}

func (v *SchemaValidator) resolveEntry(ctx context.Context, projectID, eventName string) (*compiledEntry, error) {
	key := cacheKey(projectID, eventName)

	v.mu.Lock()
	if e, ok := v.cache[key]; ok {
		v.mu.Unlock()
		return e, nil
	}
	v.mu.Unlock()

	// singleflight coalesces concurrent cache-miss fetches for the same key so
	// at most one DB query and compile runs per (projectID, eventName) at a time.
	result, err, _ := v.sf.Do(key, func() (any, error) {
		sc, err := v.store.Get(ctx, projectID, eventName)
		if errors.Is(err, ErrNotFound) {
			return (*compiledEntry)(nil), nil
		}
		if err != nil {
			return nil, fmt.Errorf("load schema: %w", err)
		}

		c := jsonschema.NewCompiler()
		c.ExtractAnnotations = false
		if err := c.AddResource("schema.json", bytes.NewReader(sc.Schema)); err != nil {
			return nil, fmt.Errorf("compile schema: %w", err)
		}
		compiled, err := c.Compile("schema.json")
		if err != nil {
			return nil, fmt.Errorf("compile schema: %w", err)
		}

		return &compiledEntry{compiled: compiled, mode: sc.Mode}, nil
	})
	if err != nil {
		return nil, err
	}

	entry, _ := result.(*compiledEntry)
	if entry != nil {
		v.mu.Lock()
		v.cache[key] = entry
		v.mu.Unlock()
	}
	return entry, nil
}

func cacheKey(projectID, eventName string) string {
	return projectID + ":" + eventName
}
