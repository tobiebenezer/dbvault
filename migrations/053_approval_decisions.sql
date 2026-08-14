CREATE TABLE IF NOT EXISTS approval_decisions (
    request_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    approved INTEGER NOT NULL CHECK (approved IN (0,1)),
    reason TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (request_id, actor_id),
    FOREIGN KEY (request_id) REFERENCES approval_requests(id)
) STRICT;
