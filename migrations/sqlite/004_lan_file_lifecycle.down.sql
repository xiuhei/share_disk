ALTER TABLE lan_files RENAME TO lan_files_v2;

CREATE TABLE lan_files (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL,
    object_id TEXT NOT NULL REFERENCES local_objects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    mime TEXT NOT NULL DEFAULT 'application/octet-stream',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO lan_files (id, owner_user_id, object_id, name, normalized_name, mime, created_at)
SELECT id, owner_user_id, object_id, name, normalized_name, mime, created_at
FROM lan_files_v2
WHERE status = 'active';

DROP TABLE lan_files_v2;

CREATE INDEX idx_lan_files_owner_created ON lan_files(owner_user_id, created_at DESC);
CREATE INDEX idx_lan_files_object ON lan_files(object_id);
