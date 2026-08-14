CREATE TABLE IF NOT EXISTS quotas (
    organisation_id TEXT PRIMARY KEY,
    maximum_sources INTEGER NOT NULL DEFAULT 0,
    maximum_agents INTEGER NOT NULL DEFAULT 0,
    maximum_storage_bytes INTEGER NOT NULL DEFAULT 0,
    maximum_concurrent_backups INTEGER NOT NULL DEFAULT 0,
    maximum_concurrent_restores INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
