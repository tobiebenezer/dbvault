CREATE TABLE IF NOT EXISTS organisations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    plan TEXT NOT NULL DEFAULT 'community',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
