CREATE TABLE IF NOT EXISTS log_replication_state (
    log_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    status TEXT NOT NULL,
    stored_bytes INTEGER NOT NULL DEFAULT 0,
    verified_at TEXT,
    error_code TEXT,
    error_message TEXT,
    PRIMARY KEY (log_id, destination_id)
) STRICT;
