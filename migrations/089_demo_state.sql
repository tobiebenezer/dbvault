CREATE TABLE IF NOT EXISTS demo_state (
    id TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL CHECK (enabled IN (0,1)),
    sample_data_version TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
