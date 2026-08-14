CREATE TABLE IF NOT EXISTS realtime_cursors (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    stream TEXT NOT NULL,
    last_event_id TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(organisation_id, user_id, stream)
) STRICT;
