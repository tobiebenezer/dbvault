CREATE TABLE IF NOT EXISTS job_cancellation_requests (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    status TEXT NOT NULL,
    reason TEXT,
    created_at TEXT NOT NULL,
    completed_at TEXT
) STRICT;
