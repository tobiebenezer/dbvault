CREATE TABLE IF NOT EXISTS saml_providers (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    metadata_url TEXT,
    certificate_pem TEXT,
    enabled INTEGER NOT NULL CHECK (enabled IN (0,1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
