CREATE TABLE IF NOT EXISTS backup_sets (
    id TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL UNIQUE,
    engine TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    driver_api INTEGER NOT NULL,
    backup_mode TEXT NOT NULL,
    backup_format TEXT NOT NULL,
    set_digest TEXT NOT NULL,
    consistency_json TEXT NOT NULL,
    toolchain_json TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
