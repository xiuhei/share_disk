-- Share Disk PostgreSQL Schema
-- Version: 003
-- Description: Link authoritative catalog entries and replicas back to the
-- device-local LAN file that can serve their verified content.

ALTER TABLE file_entries ADD COLUMN client_file_id VARCHAR(64);

CREATE UNIQUE INDEX file_entries_origin_client_unique
    ON file_entries (user_id, origin_device_id, client_file_id)
    WHERE origin_device_id IS NOT NULL AND client_file_id IS NOT NULL;

ALTER TABLE replicas ADD COLUMN endpoint TEXT;

ALTER TABLE replicas ADD CONSTRAINT replicas_endpoint_length_check
    CHECK (endpoint IS NULL OR length(endpoint) <= 2048);
