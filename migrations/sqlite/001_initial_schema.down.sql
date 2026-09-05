-- Share Disk SQLite Schema Rollback
-- Version: 001
-- Description: Rollback initial schema

DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS outgoing_ops;
DROP TABLE IF EXISTS catalog_cache;
DROP TABLE IF EXISTS transfer_chunks;
DROP TABLE IF EXISTS local_transfers;
DROP TABLE IF EXISTS local_replicas;
DROP TABLE IF EXISTS local_objects;
-- Note: schema_migrations table is managed by the migrator