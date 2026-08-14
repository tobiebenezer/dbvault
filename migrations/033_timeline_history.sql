CREATE TABLE IF NOT EXISTS timeline_history (
    source_id TEXT NOT NULL,
    system_identifier TEXT NOT NULL,
    timeline INTEGER NOT NULL,
    parent_timeline INTEGER NOT NULL,
    switch_lsn TEXT NOT NULL,
    reason TEXT,
    object_key TEXT NOT NULL,
    verified_at TEXT,
    PRIMARY KEY (source_id, system_identifier, timeline)
) STRICT;
