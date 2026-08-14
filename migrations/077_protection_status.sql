CREATE TABLE IF NOT EXISTS protection_status (
    source_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    score INTEGER NOT NULL,
    summary TEXT NOT NULL,
    reasons_json TEXT NOT NULL,
    suggested_actions_json TEXT NOT NULL,
    calculated_at TEXT NOT NULL
) STRICT;
