CREATE TABLE IF NOT EXISTS control_plane_audit_exports (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    destination_type TEXT NOT NULL,
    configuration_json TEXT NOT NULL,
    last_event_id TEXT,
    last_success_at TEXT,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id)
) STRICT;
