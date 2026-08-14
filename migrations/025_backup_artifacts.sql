CREATE TABLE IF NOT EXISTS backup_artifacts (
    id TEXT PRIMARY KEY,
    backup_set_id TEXT NOT NULL,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    format TEXT NOT NULL,
    content_type TEXT NOT NULL,
    required INTEGER NOT NULL CHECK (required IN (0, 1)),
    sequence INTEGER NOT NULL,
    logical_size INTEGER NOT NULL,
    root_digest TEXT NOT NULL,
    metadata_json TEXT NOT NULL,
    UNIQUE (backup_set_id, sequence)
) STRICT;
