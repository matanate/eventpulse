CREATE TABLE event_schemas (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_name   TEXT NOT NULL,
    schema       JSONB NOT NULL,
    mode         TEXT NOT NULL DEFAULT 'warn' CHECK (mode IN ('enforce', 'warn')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Lookup index: one schema per (project, event_name).
CREATE UNIQUE INDEX event_schemas_project_event_idx
    ON event_schemas (project_id, event_name);
