CREATE TABLE IF NOT EXISTS pitr_runs (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    status TEXT NOT NULL,
    reached_target INTEGER NOT NULL CHECK (reached_target IN (0, 1)),
    result_json TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    error_code TEXT,
    error_message TEXT
) STRICT;
