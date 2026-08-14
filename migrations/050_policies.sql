CREATE TABLE IF NOT EXISTS policies (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    scope TEXT NOT NULL,
    severity TEXT NOT NULL,
    enforcement TEXT NOT NULL,
    expression TEXT NOT NULL,
    message TEXT NOT NULL,
    remediation TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
