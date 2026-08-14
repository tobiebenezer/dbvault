CREATE TABLE IF NOT EXISTS service_accounts (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT,
    name TEXT NOT NULL,
    token_digest TEXT NOT NULL,
    scopes_json TEXT NOT NULL,
    expires_at TEXT,
    revoked_at TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id),
    FOREIGN KEY (project_id) REFERENCES projects(id)
) STRICT;
