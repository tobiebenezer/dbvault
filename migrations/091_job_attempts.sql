CREATE TABLE IF NOT EXISTS job_attempts (
    id TEXT PRIMARY KEY,
    original_job_id TEXT,
    job_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT
) STRICT;
