CREATE TABLE IF NOT EXISTS policy_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    policy_json TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
