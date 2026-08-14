CREATE TABLE IF NOT EXISTS log_gc_plans (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    catalogue_version INTEGER NOT NULL,
    recovery_digest TEXT NOT NULL,
    candidates_json TEXT NOT NULL,
    estimated_bytes INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
) STRICT;
