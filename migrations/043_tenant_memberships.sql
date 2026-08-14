CREATE TABLE IF NOT EXISTS tenant_memberships (
    organisation_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organisation_id, user_id),
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
