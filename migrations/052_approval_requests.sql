CREATE TABLE IF NOT EXISTS approval_requests (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    required_approvals INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
