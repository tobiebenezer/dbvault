CREATE TABLE IF NOT EXISTS oidc_providers (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    issuer TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret_ref_json TEXT NOT NULL,
    group_claim TEXT,
    enabled INTEGER NOT NULL CHECK (enabled IN (0,1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
