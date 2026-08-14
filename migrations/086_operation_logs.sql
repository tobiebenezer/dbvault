CREATE TABLE IF NOT EXISTS operation_logs (
    id TEXT PRIMARY KEY,
    operation_id TEXT NOT NULL,
    stage TEXT NOT NULL,
    line TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
