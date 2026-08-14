CREATE TABLE IF NOT EXISTS recovery_lineages (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    engine TEXT NOT NULL,
    native_identity TEXT NOT NULL,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    status TEXT NOT NULL
) STRICT;
