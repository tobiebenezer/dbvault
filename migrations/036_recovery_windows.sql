CREATE TABLE IF NOT EXISTS recovery_windows (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    lineage_id TEXT NOT NULL,
    earliest_time TEXT NOT NULL,
    latest_time TEXT NOT NULL,
    earliest_position_json TEXT NOT NULL,
    latest_position_json TEXT NOT NULL,
    continuous INTEGER NOT NULL CHECK (continuous IN (0, 1)),
    gap_count INTEGER NOT NULL DEFAULT 0,
    last_verified_at TEXT NOT NULL
) STRICT;
