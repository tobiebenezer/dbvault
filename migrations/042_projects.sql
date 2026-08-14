CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organisation_id, slug),
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
CREATE TABLE IF NOT EXISTS environments (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    class TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (project_id, name),
    FOREIGN KEY (organisation_id) REFERENCES organisations(id),
    FOREIGN KEY (project_id) REFERENCES projects(id)
) STRICT;
