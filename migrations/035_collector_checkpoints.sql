CREATE TABLE IF NOT EXISTS collector_checkpoints (
    source_id TEXT PRIMARY KEY,
    engine TEXT NOT NULL,
    lineage_id TEXT NOT NULL,
    checkpoint_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
