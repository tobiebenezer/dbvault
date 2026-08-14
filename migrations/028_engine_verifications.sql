CREATE TABLE IF NOT EXISTS engine_verifications (
    id TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL,
    engine TEXT NOT NULL,
    verification_type TEXT NOT NULL,
    status TEXT NOT NULL,
    details_json TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT
) STRICT;
