-- Canonical manifest identity shared by HTTP and P2P transfer adapters.
-- Existing objects are backfilled when they are next verified or imported.
ALTER TABLE local_objects ADD COLUMN manifest_version INTEGER;
ALTER TABLE local_objects ADD COLUMN manifest_digest BLOB;

