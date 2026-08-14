CREATE TABLE IF NOT EXISTS support_bundles (
    id TEXT PRIMARY KEY,
    path TEXT NOT NULL,
    digest TEXT,
    status TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
