CREATE TABLE IF NOT EXISTS deployment_operations (
    id TEXT PRIMARY KEY,
    operation TEXT NOT NULL,
    server_id TEXT,
    version TEXT,
    requested_by TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    status TEXT NOT NULL,
    rollback_available INTEGER NOT NULL CHECK (rollback_available IN (0,1))
) STRICT;
