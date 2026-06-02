CREATE TABLE webhook_subscriptions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    url          TEXT NOT NULL,
    secret       TEXT NOT NULL,
    filter_event TEXT,
    active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial index for the worker's subscription-match query.
CREATE INDEX webhook_subscriptions_match_idx
    ON webhook_subscriptions (project_id, filter_event)
    WHERE active;

CREATE TABLE webhook_deliveries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES webhook_subscriptions(id) ON DELETE CASCADE,
    event_id        UUID NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INT  NOT NULL DEFAULT 0,
    next_retry_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error      TEXT,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial index for the dispatcher's due-deliveries query.
CREATE INDEX webhook_deliveries_due_idx
    ON webhook_deliveries (next_retry_at)
    WHERE status = 'pending';

-- Prevents duplicate delivery rows when the worker reprocesses a message.
CREATE UNIQUE INDEX webhook_deliveries_dedupe_idx
    ON webhook_deliveries (subscription_id, event_id);
