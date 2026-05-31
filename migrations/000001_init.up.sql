CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE accounts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE projects (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE api_keys (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key_hash   TEXT NOT NULL UNIQUE,
    prefix     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE TABLE events (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID        NOT NULL REFERENCES projects(id),
    event           TEXT        NOT NULL,
    user_id         TEXT,
    properties      JSONB,
    idempotency_key TEXT,
    timestamp       TIMESTAMPTZ NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX events_idempotency_key_idx
    ON events (project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX events_project_event_time_idx
    ON events (project_id, event, timestamp DESC);

CREATE INDEX events_project_user_time_idx
    ON events (project_id, user_id, timestamp DESC)
    WHERE user_id IS NOT NULL;

CREATE TABLE daily_event_counts (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event      TEXT NOT NULL,
    date       DATE NOT NULL,
    count      BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (project_id, event, date)
);

CREATE TABLE dead_letter_events (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID,
    raw_payload   JSONB       NOT NULL,
    error         TEXT        NOT NULL,
    attempt_count INT         NOT NULL DEFAULT 0,
    failed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
