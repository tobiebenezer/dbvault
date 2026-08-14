CREATE TABLE IF NOT EXISTS usage_records (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT,
    metric TEXT NOT NULL,
    quantity INTEGER NOT NULL,
    unit TEXT NOT NULL,
    recorded_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
CREATE INDEX IF NOT EXISTS idx_usage_tenant_time ON usage_records(organisation_id, recorded_at);
