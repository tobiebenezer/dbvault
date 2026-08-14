CREATE TABLE IF NOT EXISTS scim_provisioning (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    base_url TEXT NOT NULL,
    token_digest TEXT NOT NULL,
    group_mappings_json TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0,1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
