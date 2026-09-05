DROP INDEX IF EXISTS transfer_tasks_live_object_target_unique;
DROP INDEX IF EXISTS replicas_device_client_file_unique;
ALTER TABLE replicas DROP COLUMN IF EXISTS client_file_id;
