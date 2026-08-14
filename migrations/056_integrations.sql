CREATE TABLE IF NOT EXISTS integrations (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    configuration_json TEXT NOT NULL,
    secret_references_json TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0,1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
