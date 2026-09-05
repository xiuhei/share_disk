-- Share Disk PostgreSQL Schema Rollback
-- Version: 001
-- Description: Rollback initial schema

DROP TABLE IF EXISTS account_events;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS tombstone_acks;
DROP TABLE IF EXISTS tombstones;
DROP TABLE IF EXISTS trash_records;
DROP TABLE IF EXISTS redundancy_policies;
DROP TABLE IF EXISTS transfer_sources;
DROP TABLE IF EXISTS transfer_tasks;
DROP TABLE IF EXISTS replicas;
DROP TABLE IF EXISTS file_entries;
DROP TABLE IF EXISTS file_objects;
DROP TABLE IF EXISTS folders;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS users;
-- Note: schema_migrations table is managed by the migrator