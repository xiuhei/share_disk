-- Share Disk PostgreSQL Schema Rollback
-- Version: 002
-- Description: Rollback same-account invariant constraints.

ALTER TABLE tombstone_acks DROP CONSTRAINT IF EXISTS tombstone_acks_device_same_account_fk;
ALTER TABLE tombstone_acks DROP CONSTRAINT IF EXISTS tombstone_acks_tombstone_same_account_fk;
ALTER TABLE tombstone_acks DROP COLUMN IF EXISTS user_id;

ALTER TABLE trash_records DROP CONSTRAINT IF EXISTS trash_records_folder_same_account_fk;
ALTER TABLE trash_records DROP CONSTRAINT IF EXISTS trash_records_file_entry_same_account_fk;
DROP INDEX IF EXISTS trash_records_user_id_id_unique;
ALTER TABLE trash_records DROP COLUMN IF EXISTS user_id;

ALTER TABLE transfer_sources DROP CONSTRAINT IF EXISTS transfer_sources_source_device_same_account_fk;
ALTER TABLE transfer_sources DROP CONSTRAINT IF EXISTS transfer_sources_task_same_account_fk;
ALTER TABLE transfer_sources DROP COLUMN IF EXISTS user_id;

ALTER TABLE replicas DROP CONSTRAINT IF EXISTS replicas_device_same_account_fk;
ALTER TABLE replicas DROP CONSTRAINT IF EXISTS replicas_object_same_account_fk;
DROP INDEX IF EXISTS replicas_user_id_id_unique;
ALTER TABLE replicas DROP COLUMN IF EXISTS user_id;

ALTER TABLE tombstones DROP CONSTRAINT IF EXISTS tombstones_object_same_account_fk;
ALTER TABLE transfer_tasks DROP CONSTRAINT IF EXISTS transfer_tasks_lease_owner_same_account_fk;
ALTER TABLE transfer_tasks DROP CONSTRAINT IF EXISTS transfer_tasks_target_device_same_account_fk;
ALTER TABLE transfer_tasks DROP CONSTRAINT IF EXISTS transfer_tasks_object_same_account_fk;
ALTER TABLE file_entries DROP CONSTRAINT IF EXISTS file_entries_origin_device_same_account_fk;
ALTER TABLE file_entries DROP CONSTRAINT IF EXISTS file_entries_object_same_account_fk;
ALTER TABLE file_entries DROP CONSTRAINT IF EXISTS file_entries_folder_same_account_fk;
ALTER TABLE folders DROP CONSTRAINT IF EXISTS folders_parent_same_account_fk;

DROP INDEX IF EXISTS tombstones_user_id_id_unique;
DROP INDEX IF EXISTS transfer_tasks_user_id_id_unique;
DROP INDEX IF EXISTS file_entries_user_id_id_unique;
DROP INDEX IF EXISTS devices_user_id_id_unique;
DROP INDEX IF EXISTS file_objects_user_id_id_unique;
DROP INDEX IF EXISTS folders_user_id_id_unique;

DROP INDEX IF EXISTS transfer_tasks_active_unique;
DROP INDEX IF EXISTS idx_replicas_verified_at;
DROP INDEX IF EXISTS idx_replicas_ready_verified;
