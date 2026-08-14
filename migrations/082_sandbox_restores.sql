CREATE TABLE IF NOT EXISTS sandbox_restores (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    snapshot_id TEXT,
    target_time TEXT,
    engine TEXT NOT NULL,
    status TEXT NOT NULL,
    connection_ref TEXT,
    expires_at TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
