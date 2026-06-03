-- Support the stats today_count query: COUNT(*) WHERE project_id=$1 AND received_at BETWEEN today_start AND today_end.
-- Without this index the query performs a sequential scan over all events for the project.
CREATE INDEX IF NOT EXISTS events_project_received_at_idx
    ON events (project_id, received_at DESC);
