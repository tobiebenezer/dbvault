CREATE TABLE IF NOT EXISTS upgrade_campaigns (
    id TEXT PRIMARY KEY,
    organisation_id TEXT,
    channel TEXT NOT NULL,
    target_version TEXT NOT NULL,
    strategy TEXT NOT NULL,
    maximum_unavailable_percent INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT
) STRICT;
