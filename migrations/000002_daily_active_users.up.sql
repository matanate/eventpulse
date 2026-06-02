CREATE TABLE daily_active_users (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    user_id    TEXT NOT NULL,
    PRIMARY KEY (project_id, date, user_id)
);

-- Supports "first seen" lookups: all dates a user appeared (for cohort assignment).
CREATE INDEX daily_active_users_user_idx
    ON daily_active_users (project_id, user_id, date);
