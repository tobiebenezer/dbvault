CREATE TABLE IF NOT EXISTS hook_runs (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    hook_type TEXT NOT NULL,
    status TEXT NOT NULL,
    exit_code INTEGER,
    output TEXT,
    started_at TEXT NOT NULL,
    completed_at TEXT
) STRICT;
