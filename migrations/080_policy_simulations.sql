CREATE TABLE IF NOT EXISTS policy_simulations (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    request_json TEXT NOT NULL,
    result_json TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
