-- Share Disk PostgreSQL Schema
-- Version: 001
-- Description: Initial schema for control plane

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended', 'deleted'))
);

-- Create unique index on LOWER(account) since PostgreSQL doesn't support expression unique constraints in table definition
CREATE UNIQUE INDEX users_account_unique ON users (LOWER(account));

-- Devices table
CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    platform VARCHAR(50) NOT NULL,
    public_key BYTEA NOT NULL,
    peer_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    last_seen_at TIMESTAMP WITH TIME ZONE,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT devices_user_public_key_unique UNIQUE (user_id, public_key),
    CONSTRAINT devices_peer_id_unique UNIQUE (peer_id),
    CONSTRAINT devices_status_check CHECK (status IN ('active', 'suspended', 'deregistered'))
);

-- Sessions table
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    family_id UUID NOT NULL,
    refresh_hash BYTEA NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE,
    rotated_from UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT sessions_refresh_hash_unique UNIQUE (refresh_hash)
);

CREATE INDEX idx_sessions_user_device ON sessions(user_id, device_id);
CREATE INDEX idx_sessions_family ON sessions(family_id);

-- Folders table
CREATE TABLE folders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id UUID REFERENCES folders(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT folders_status_check CHECK (status IN ('active', 'deleted'))
);

-- Create unique indexes for folder names
-- For non-root folders (parent_id IS NOT NULL)
CREATE UNIQUE INDEX folders_user_parent_name_unique ON folders (user_id, parent_id, normalized_name)
    WHERE parent_id IS NOT NULL AND status = 'active';

-- For root folders (parent_id IS NULL)
CREATE UNIQUE INDEX folders_user_root_name_unique ON folders (user_id, normalized_name)
    WHERE parent_id IS NULL AND status = 'active';

CREATE INDEX idx_folders_user_parent ON folders(user_id, parent_id);

-- File objects table (content-addressed)
CREATE TABLE file_objects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sha256 BYTEA NOT NULL,
    size BIGINT NOT NULL,
    mime VARCHAR(255),
    chunk_size INTEGER NOT NULL DEFAULT 4194304, -- 4 MiB
    chunk_count INTEGER NOT NULL DEFAULT 1,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT file_objects_user_sha256_size_unique UNIQUE (user_id, sha256, size),
    CONSTRAINT file_objects_status_check CHECK (status IN ('pending', 'ready', 'corrupted', 'deleted')),
    CONSTRAINT file_objects_size_positive CHECK (size >= 0),
    CONSTRAINT file_objects_chunk_size_positive CHECK (chunk_size > 0),
    CONSTRAINT file_objects_chunk_count_positive CHECK (chunk_count > 0)
);

CREATE INDEX idx_file_objects_sha256 ON file_objects(sha256);
CREATE INDEX idx_file_objects_status ON file_objects(status);

-- File entries table (logical files in directories)
CREATE TABLE file_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    folder_id UUID NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    object_id UUID NOT NULL REFERENCES file_objects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    origin_device_id UUID REFERENCES devices(id),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT file_entries_status_check CHECK (status IN ('active', 'trashed', 'purging', 'purged'))
);

-- Create unique index for active file entries only
CREATE UNIQUE INDEX file_entries_folder_name_unique ON file_entries (folder_id, normalized_name)
    WHERE status = 'active';

CREATE INDEX idx_file_entries_folder ON file_entries(folder_id);
CREATE INDEX idx_file_entries_object ON file_entries(object_id);
CREATE INDEX idx_file_entries_status ON file_entries(status);

-- Replicas table
CREATE TABLE replicas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    object_id UUID NOT NULL REFERENCES file_objects(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    state VARCHAR(50) NOT NULL DEFAULT 'pending',
    size BIGINT,
    verified_at TIMESTAMP WITH TIME ZONE,
    reported_at TIMESTAMP WITH TIME ZONE,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT replicas_object_device_unique UNIQUE (object_id, device_id),
    CONSTRAINT replicas_state_check CHECK (state IN ('pending', 'ready', 'missing', 'corrupt', 'deleting', 'deleted'))
);

CREATE INDEX idx_replicas_object ON replicas(object_id);
CREATE INDEX idx_replicas_device ON replicas(device_id);
CREATE INDEX idx_replicas_state ON replicas(state);

-- Transfer tasks table
CREATE TABLE transfer_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    object_id UUID NOT NULL REFERENCES file_objects(id) ON DELETE CASCADE,
    target_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    reason VARCHAR(50) NOT NULL,
    state VARCHAR(50) NOT NULL DEFAULT 'queued',
    priority INTEGER NOT NULL DEFAULT 0,
    lease_owner UUID REFERENCES devices(id),
    lease_until TIMESTAMP WITH TIME ZONE,
    attempt INTEGER NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT transfer_tasks_state_check CHECK (state IN (
        'queued', 'assigned', 'discovering', 'connecting', 'transferring',
        'verifying', 'completed', 'waiting_source', 'retry_wait',
        'paused', 'canceled', 'failed_permanent'
    )),
    CONSTRAINT transfer_tasks_reason_check CHECK (reason IN ('pull', 'push', 'replication', 'redundancy'))
);

CREATE INDEX idx_transfer_tasks_user ON transfer_tasks(user_id);
CREATE INDEX idx_transfer_tasks_object ON transfer_tasks(object_id);
CREATE INDEX idx_transfer_tasks_target ON transfer_tasks(target_device_id);
CREATE INDEX idx_transfer_tasks_state ON transfer_tasks(state);
CREATE INDEX idx_transfer_tasks_lease ON transfer_tasks(lease_owner, lease_until);

-- Transfer sources table
CREATE TABLE transfer_sources (
    task_id UUID NOT NULL REFERENCES transfer_tasks(id) ON DELETE CASCADE,
    source_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rank INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    disabled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT transfer_sources_task_device_unique UNIQUE (task_id, source_device_id)
);

CREATE INDEX idx_transfer_sources_task ON transfer_sources(task_id);

-- Redundancy policies table
CREATE TABLE redundancy_policies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope_type VARCHAR(50) NOT NULL,
    scope_id UUID NOT NULL,
    desired_replicas INTEGER NOT NULL DEFAULT 1,
    constraints JSONB,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT redundancy_policies_scope_unique UNIQUE (scope_type, scope_id),
    CONSTRAINT redundancy_policies_scope_type_check CHECK (scope_type IN ('global', 'folder', 'object')),
    CONSTRAINT redundancy_policies_desired_check CHECK (desired_replicas >= 0)
);

-- Trash records table
CREATE TABLE trash_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_entry_id UUID NOT NULL REFERENCES file_entries(id) ON DELETE CASCADE,
    original_folder_id UUID NOT NULL REFERENCES folders(id),
    original_name VARCHAR(255) NOT NULL,
    deleted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    purge_after TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'trashed',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT trash_records_entry_unique UNIQUE (file_entry_id),
    CONSTRAINT trash_records_status_check CHECK (status IN ('trashed', 'purging', 'purged'))
);

CREATE INDEX idx_trash_records_purge ON trash_records(purge_after);
CREATE INDEX idx_trash_records_status ON trash_records(status);

-- Tombstones table
CREATE TABLE tombstones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    object_id UUID NOT NULL REFERENCES file_objects(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT tombstones_object_generation_unique UNIQUE (object_id, generation),
    CONSTRAINT tombstones_status_check CHECK (status IN ('active', 'expired', 'acked'))
);

CREATE INDEX idx_tombstones_expires ON tombstones(expires_at);
CREATE INDEX idx_tombstones_status ON tombstones(status);

-- Tombstone acknowledgments table
CREATE TABLE tombstone_acks (
    tombstone_id UUID NOT NULL REFERENCES tombstones(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    acked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    result VARCHAR(50) NOT NULL DEFAULT 'success',

    CONSTRAINT tombstone_acks_pk PRIMARY KEY (tombstone_id, device_id),
    CONSTRAINT tombstone_acks_result_check CHECK (result IN ('success', 'already_absent', 'failed'))
);

-- Idempotency keys table
CREATE TABLE idempotency_keys (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    request_hash BYTEA NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'processing',
    response_code INTEGER,
    response_body JSONB,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT idempotency_keys_pk PRIMARY KEY (user_id, key),
    CONSTRAINT idempotency_keys_status_check CHECK (status IN ('processing', 'completed', 'failed'))
);

CREATE INDEX idx_idempotency_keys_expires ON idempotency_keys(expires_at);

-- Account events table
CREATE TABLE account_events (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    type VARCHAR(100) NOT NULL,
    entity_id UUID NOT NULL,
    entity_version BIGINT,
    payload JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT account_events_pk PRIMARY KEY (user_id, seq)
);

CREATE INDEX idx_account_events_type ON account_events(type);
CREATE INDEX idx_account_events_entity ON account_events(entity_id);

-- Note: schema_migrations table is created by the migrator