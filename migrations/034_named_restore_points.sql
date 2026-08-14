CREATE TABLE IF NOT EXISTS named_restore_points (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    name TEXT NOT NULL,
    lsn TEXT,
    timeline INTEGER,
    gtid_set TEXT,
    created_at TEXT NOT NULL,
    created_by TEXT,
    UNIQUE (source_id, name)
) STRICT;
