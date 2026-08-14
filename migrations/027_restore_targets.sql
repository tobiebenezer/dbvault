CREATE TABLE IF NOT EXISTS restore_targets (
    id TEXT PRIMARY KEY,
    engine TEXT NOT NULL,
    name TEXT NOT NULL,
    config_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
