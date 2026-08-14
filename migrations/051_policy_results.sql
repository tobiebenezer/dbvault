CREATE TABLE IF NOT EXISTS policy_results (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL,
    organisation_id TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    passed INTEGER NOT NULL CHECK (passed IN (0,1)),
    message TEXT,
    evaluated_at TEXT NOT NULL,
    FOREIGN KEY (policy_id) REFERENCES policies(id),
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
