CREATE TABLE IF NOT EXISTS job_log_chunks (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    sequence_number INTEGER NOT NULL,
    level TEXT NOT NULL,
    stage TEXT,
    message TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(job_id, sequence_number)
) STRICT;
