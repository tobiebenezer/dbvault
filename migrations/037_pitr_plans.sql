CREATE TABLE IF NOT EXISTS pitr_plans (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    engine TEXT NOT NULL,
    target_json TEXT NOT NULL,
    plan_json TEXT NOT NULL,
    repository_digest TEXT NOT NULL,
    catalogue_version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
) STRICT;
