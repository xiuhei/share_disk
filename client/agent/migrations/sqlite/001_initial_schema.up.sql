-- Share Disk SQLite Schema
-- Version: 001
-- Description: Initial schema for agent local state

-- Enable WAL mode for better concurrency
PRAGMA journal_mode=WAL;

-- Enable foreign keys
PRAGMA foreign_keys=ON;

-- Local objects table
CREATE TABLE local_objects (
    id TEXT PRIMARY KEY,
    sha256 BLOB NOT NULL,
    size INTEGER NOT NULL,
    relative_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'absent',
    last_verified_at TEXT,
    inode_snapshot INTEGER,
    file_id_snapshot TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),

    CONSTRAINT local_objects_sha256_unique UNIQUE (sha256, size),
    CONSTRAINT local_objects_status_check CHECK (status IN (
        'absent', 'importing', 'downloading', 'verifying',
        'ready', 'failed', 'quarantined', 'deleting',
        'missing', 'corrupt'
    )),
    CONSTRAINT local_objects_size_check CHECK (size >= 0)
);

CREATE INDEX idx_local_objects_sha256 ON local_objects(sha256);
CREATE INDEX idx_local_objects_status ON local_objects(status);

-- Local replicas table
CREATE TABLE local_replicas (
    id TEXT PRIMARY KEY,
    server_replica_id TEXT,
    object_id TEXT NOT NULL REFERENCES local_objects(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    reported_status TEXT,
    pending_result TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),

    CONSTRAINT local_replicas_object_device_unique UNIQUE (object_id, device_id)
);

CREATE INDEX idx_local_replicas_object ON local_replicas(object_id);

-- Local transfers table
CREATE TABLE local_transfers (
    task_id TEXT PRIMARY KEY,
    object_id TEXT NOT NULL REFERENCES local_objects(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued',
    candidate_sources TEXT, -- JSON array of source device IDs
    temp_path TEXT,
    total_chunks INTEGER NOT NULL DEFAULT 0,
    completed_chunks INTEGER NOT NULL DEFAULT 0,
    completed_bytes INTEGER NOT NULL DEFAULT 0,
    error TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),

    CONSTRAINT local_transfers_status_check CHECK (status IN (
        'queued', 'assigned', 'discovering', 'connecting',
        'transferring', 'verifying', 'completed', 'waiting_source',
        'retry_wait', 'paused', 'canceled', 'failed_permanent'
    )),
    CONSTRAINT local_transfers_chunks_check CHECK (total_chunks >= 0 AND completed_chunks >= 0),
    CONSTRAINT local_transfers_bytes_check CHECK (completed_bytes >= 0)
);

CREATE INDEX idx_local_transfers_object ON local_transfers(object_id);
CREATE INDEX idx_local_transfers_status ON local_transfers(status);

-- Transfer chunks table
CREATE TABLE transfer_chunks (
    task_id TEXT NOT NULL REFERENCES local_transfers(task_id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    offset INTEGER NOT NULL,
    size INTEGER NOT NULL,
    expected_hash BLOB,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),

    CONSTRAINT transfer_chunks_pk PRIMARY KEY (task_id, chunk_index),
    CONSTRAINT transfer_chunks_status_check CHECK (status IN ('pending', 'requested', 'received', 'verified', 'failed')),
    CONSTRAINT transfer_chunks_offset_check CHECK (offset >= 0),
    CONSTRAINT transfer_chunks_size_check CHECK (size > 0)
);

CREATE INDEX idx_transfer_chunks_task ON transfer_chunks(task_id);

-- Catalog cache table (read-only cache from server)
CREATE TABLE catalog_cache (
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    entity_version INTEGER NOT NULL,
    data TEXT, -- JSON data
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),

    CONSTRAINT catalog_cache_pk PRIMARY KEY (entity_type, entity_id)
);

CREATE INDEX idx_catalog_cache_type ON catalog_cache(entity_type);

-- Outgoing operations table (pending operations to sync with server)
CREATE TABLE outgoing_ops (
    id TEXT PRIMARY KEY,
    op_type TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_data TEXT NOT NULL, -- JSON request data
    status TEXT NOT NULL DEFAULT 'pending',
    response_code INTEGER,
    response_data TEXT, -- JSON response data
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),

    CONSTRAINT outgoing_ops_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    CONSTRAINT outgoing_ops_retry_count_check CHECK (retry_count >= 0)
);

CREATE INDEX idx_outgoing_ops_status ON outgoing_ops(status);
CREATE INDEX idx_outgoing_ops_next_retry ON outgoing_ops(next_retry_at);

-- Sync state table
CREATE TABLE sync_state (
    account_id TEXT PRIMARY KEY,
    last_applied_seq INTEGER NOT NULL DEFAULT 0,
    snapshot_version INTEGER,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Settings table
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Note: schema_migrations table is created by the migrator