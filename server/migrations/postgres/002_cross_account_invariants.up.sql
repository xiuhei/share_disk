-- Share Disk PostgreSQL Schema
-- Version: 002
-- Description: Forward-only constraints that were previously (incorrectly)
-- appended to the already-published 001 migration, plus same-account
-- invariants enforced at the database level.

-- (P0-01) Replica lookup indexes.
CREATE INDEX idx_replicas_ready_verified ON replicas(object_id) WHERE state = 'ready';
CREATE INDEX idx_replicas_verified_at ON replicas(verified_at);

-- (P0-01) Prevent duplicate active tasks for the same (user, object, target, reason).
CREATE UNIQUE INDEX transfer_tasks_active_unique ON transfer_tasks (user_id, object_id, target_device_id, reason)
    WHERE state NOT IN ('completed', 'canceled', 'failed_permanent');

-- (P0-11) Composite unique keys so that composite foreign keys have a valid target.
CREATE UNIQUE INDEX folders_user_id_id_unique ON folders(user_id, id);
CREATE UNIQUE INDEX file_objects_user_id_id_unique ON file_objects(user_id, id);
CREATE UNIQUE INDEX devices_user_id_id_unique ON devices(user_id, id);
CREATE UNIQUE INDEX file_entries_user_id_id_unique ON file_entries(user_id, id);
CREATE UNIQUE INDEX transfer_tasks_user_id_id_unique ON transfer_tasks(user_id, id);
CREATE UNIQUE INDEX tombstones_user_id_id_unique ON tombstones(user_id, id);

-- (P0-11) Composite foreign keys for tables that already carry user_id.
ALTER TABLE folders ADD CONSTRAINT folders_parent_same_account_fk
    FOREIGN KEY (user_id, parent_id) REFERENCES folders(user_id, id);

ALTER TABLE file_entries ADD CONSTRAINT file_entries_folder_same_account_fk
    FOREIGN KEY (user_id, folder_id) REFERENCES folders(user_id, id);
ALTER TABLE file_entries ADD CONSTRAINT file_entries_object_same_account_fk
    FOREIGN KEY (user_id, object_id) REFERENCES file_objects(user_id, id);
ALTER TABLE file_entries ADD CONSTRAINT file_entries_origin_device_same_account_fk
    FOREIGN KEY (user_id, origin_device_id) REFERENCES devices(user_id, id);

ALTER TABLE transfer_tasks ADD CONSTRAINT transfer_tasks_object_same_account_fk
    FOREIGN KEY (user_id, object_id) REFERENCES file_objects(user_id, id);
ALTER TABLE transfer_tasks ADD CONSTRAINT transfer_tasks_target_device_same_account_fk
    FOREIGN KEY (user_id, target_device_id) REFERENCES devices(user_id, id);
ALTER TABLE transfer_tasks ADD CONSTRAINT transfer_tasks_lease_owner_same_account_fk
    FOREIGN KEY (user_id, lease_owner) REFERENCES devices(user_id, id);

ALTER TABLE tombstones ADD CONSTRAINT tombstones_object_same_account_fk
    FOREIGN KEY (user_id, object_id) REFERENCES file_objects(user_id, id);

-- (P0-11) Add user_id to child tables lacking it, backfill, then enforce.

ALTER TABLE replicas ADD COLUMN user_id UUID;
UPDATE replicas r SET user_id = fo.user_id FROM file_objects fo WHERE fo.id = r.object_id;
ALTER TABLE replicas ALTER COLUMN user_id SET NOT NULL;
CREATE UNIQUE INDEX replicas_user_id_id_unique ON replicas(user_id, id);
ALTER TABLE replicas ADD CONSTRAINT replicas_object_same_account_fk
    FOREIGN KEY (user_id, object_id) REFERENCES file_objects(user_id, id);
ALTER TABLE replicas ADD CONSTRAINT replicas_device_same_account_fk
    FOREIGN KEY (user_id, device_id) REFERENCES devices(user_id, id);

ALTER TABLE transfer_sources ADD COLUMN user_id UUID;
UPDATE transfer_sources ts SET user_id = tt.user_id FROM transfer_tasks tt WHERE tt.id = ts.task_id;
ALTER TABLE transfer_sources ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE transfer_sources ADD CONSTRAINT transfer_sources_task_same_account_fk
    FOREIGN KEY (user_id, task_id) REFERENCES transfer_tasks(user_id, id);
ALTER TABLE transfer_sources ADD CONSTRAINT transfer_sources_source_device_same_account_fk
    FOREIGN KEY (user_id, source_device_id) REFERENCES devices(user_id, id);

ALTER TABLE trash_records ADD COLUMN user_id UUID;
UPDATE trash_records tr SET user_id = fe.user_id FROM file_entries fe WHERE fe.id = tr.file_entry_id;
ALTER TABLE trash_records ALTER COLUMN user_id SET NOT NULL;
CREATE UNIQUE INDEX trash_records_user_id_id_unique ON trash_records(user_id, id);
ALTER TABLE trash_records ADD CONSTRAINT trash_records_file_entry_same_account_fk
    FOREIGN KEY (user_id, file_entry_id) REFERENCES file_entries(user_id, id);
ALTER TABLE trash_records ADD CONSTRAINT trash_records_folder_same_account_fk
    FOREIGN KEY (user_id, original_folder_id) REFERENCES folders(user_id, id);

ALTER TABLE tombstone_acks ADD COLUMN user_id UUID;
UPDATE tombstone_acks ta SET user_id = t.user_id FROM tombstones t WHERE t.id = ta.tombstone_id;
ALTER TABLE tombstone_acks ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE tombstone_acks ADD CONSTRAINT tombstone_acks_tombstone_same_account_fk
    FOREIGN KEY (user_id, tombstone_id) REFERENCES tombstones(user_id, id);
ALTER TABLE tombstone_acks ADD CONSTRAINT tombstone_acks_device_same_account_fk
    FOREIGN KEY (user_id, device_id) REFERENCES devices(user_id, id);
