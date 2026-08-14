CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    driver TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    repository_id TEXT NOT NULL,
    config_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_snapshot_id TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS snapshots (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    status TEXT NOT NULL,
    snapshot_mode TEXT NOT NULL,
    database_size INTEGER NOT NULL,
    page_size INTEGER NOT NULL,
    page_count INTEGER NOT NULL,
    root_digest TEXT NOT NULL,
    schema_digest TEXT NOT NULL,
    chunk_count INTEGER NOT NULL,
    unique_chunk_count INTEGER NOT NULL,
    compressed_bytes INTEGER NOT NULL,
    unique_uploaded_bytes INTEGER NOT NULL,
    manifest_object_key TEXT NOT NULL,
    completion_object_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    committed_at TEXT,
    verified_at TEXT,
    restore_tested_at TEXT,
    tombstoned_at TEXT,
    delete_after TEXT
) STRICT;
