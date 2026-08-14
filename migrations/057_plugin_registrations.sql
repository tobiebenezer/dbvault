CREATE TABLE IF NOT EXISTS plugin_registrations (
    id TEXT PRIMARY KEY,
    organisation_id TEXT,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    category TEXT NOT NULL,
    api_version INTEGER NOT NULL,
    manifest_digest TEXT NOT NULL,
    signing_key_id TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (organisation_id, name, version)
) STRICT;
