ALTER TABLE lan_files RENAME TO lan_files_v1;

CREATE TABLE lan_files (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL,
    object_id TEXT NOT NULL REFERENCES local_objects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    mime TEXT NOT NULL DEFAULT 'application/octet-stream',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'trashed', 'purging')),
    deleted_at TEXT,
    purge_after TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK ((status = 'active' AND deleted_at IS NULL AND purge_after IS NULL) OR
           (status IN ('trashed', 'purging') AND deleted_at IS NOT NULL AND purge_after IS NOT NULL))
);

INSERT INTO lan_files (
    id, owner_user_id, object_id, name, normalized_name, mime, status,
    deleted_at, purge_after, created_at, updated_at
)
SELECT id, owner_user_id, object_id, name, normalized_name, mime, 'active',
       NULL, NULL, created_at, created_at
FROM lan_files_v1;

DROP TABLE lan_files_v1;

CREATE INDEX idx_lan_files_owner_status_created
    ON lan_files(owner_user_id, status, created_at DESC);
CREATE INDEX idx_lan_files_owner_status_purge
    ON lan_files(owner_user_id, status, purge_after);
CREATE INDEX idx_lan_files_object ON lan_files(object_id);
