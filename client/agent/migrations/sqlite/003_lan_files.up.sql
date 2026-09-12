-- LAN-visible logical file names. Object bytes remain owned by local_objects.
CREATE TABLE lan_files (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL,
    object_id TEXT NOT NULL REFERENCES local_objects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    mime TEXT NOT NULL DEFAULT 'application/octet-stream',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_lan_files_owner_created ON lan_files(owner_user_id, created_at DESC);
CREATE INDEX idx_lan_files_object ON lan_files(object_id);
