package schemas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when no schema is registered for the given key.
var ErrNotFound = errors.New("schema not found")

// Store handles persistence of event schemas.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Upsert creates or replaces the schema for (projectID, eventName).
func (s *Store) Upsert(ctx context.Context, projectID, eventName string, schema json.RawMessage, mode Mode) (*Schema, error) {
	const q = `
		INSERT INTO event_schemas (project_id, event_name, schema, mode)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (project_id, event_name) DO UPDATE
			SET schema     = EXCLUDED.schema,
			    mode       = EXCLUDED.mode,
			    updated_at = NOW()
		RETURNING id, project_id, event_name, schema, mode, created_at, updated_at`

	row := s.pool.QueryRow(ctx, q, projectID, eventName, schema, string(mode))
	return scanSchema(row)
}

// Get returns the schema for (projectID, eventName) or ErrNotFound.
func (s *Store) Get(ctx context.Context, projectID, eventName string) (*Schema, error) {
	const q = `
		SELECT id, project_id, event_name, schema, mode, created_at, updated_at
		FROM event_schemas
		WHERE project_id = $1 AND event_name = $2`

	row := s.pool.QueryRow(ctx, q, projectID, eventName)
	sc, err := scanSchema(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sc, err
}

// Delete removes the schema for (projectID, eventName). Silently succeeds if not found.
func (s *Store) Delete(ctx context.Context, projectID, eventName string) error {
	const q = `DELETE FROM event_schemas WHERE project_id = $1 AND event_name = $2`
	_, err := s.pool.Exec(ctx, q, projectID, eventName)
	if err != nil {
		return fmt.Errorf("delete schema: %w", err)
	}
	return nil
}

// List returns all schemas registered for projectID.
func (s *Store) List(ctx context.Context, projectID string) ([]*Schema, error) {
	const q = `
		SELECT id, project_id, event_name, schema, mode, created_at, updated_at
		FROM event_schemas
		WHERE project_id = $1
		ORDER BY event_name`

	rows, err := s.pool.Query(ctx, q, projectID)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	defer rows.Close()

	out := make([]*Schema, 0)
	for rows.Next() {
		sc, err := scanSchema(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func scanSchema(row pgx.Row) (*Schema, error) {
	var sc Schema
	var rawSchema []byte
	var mode string
	err := row.Scan(&sc.ID, &sc.ProjectID, &sc.EventName, &rawSchema, &mode, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sc.Schema = json.RawMessage(rawSchema)
	sc.Mode = Mode(mode)
	return &sc, nil
}
