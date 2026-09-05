ALTER TABLE replicas DROP CONSTRAINT IF EXISTS replicas_endpoint_length_check;
ALTER TABLE replicas DROP COLUMN IF EXISTS endpoint;

DROP INDEX IF EXISTS file_entries_origin_client_unique;
ALTER TABLE file_entries DROP COLUMN IF EXISTS client_file_id;
