-- Share Disk PostgreSQL Schema
-- Version: 004
-- Description: Persist the target-local identifier of a replica and prevent
-- duplicate live transfer tasks for the same object and target device.

ALTER TABLE replicas ADD COLUMN client_file_id VARCHAR(64);

UPDATE replicas r
SET client_file_id = e.client_file_id
FROM file_entries e
WHERE e.object_id = r.object_id
  AND e.origin_device_id = r.device_id
  AND e.client_file_id IS NOT NULL;

CREATE UNIQUE INDEX replicas_device_client_file_unique
    ON replicas (user_id, device_id, client_file_id)
    WHERE client_file_id IS NOT NULL AND state <> 'deleted';

CREATE UNIQUE INDEX transfer_tasks_live_object_target_unique
    ON transfer_tasks (user_id, object_id, target_device_id)
    WHERE state NOT IN ('completed', 'canceled', 'failed_permanent');
