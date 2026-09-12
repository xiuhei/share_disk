-- Share Disk SQLite Schema
-- Version: 002
-- Description: Chunk manifest storage for resumable transfers and verification

CREATE TABLE local_object_chunks (
    object_id TEXT NOT NULL REFERENCES local_objects(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    chunk_offset INTEGER NOT NULL,
    chunk_size INTEGER NOT NULL,
    chunk_hash BLOB NOT NULL,

    CONSTRAINT local_object_chunks_pk PRIMARY KEY (object_id, chunk_index),
    CONSTRAINT local_object_chunks_index_check CHECK (chunk_index >= 0),
    CONSTRAINT local_object_chunks_offset_check CHECK (chunk_offset >= 0),
    CONSTRAINT local_object_chunks_size_check CHECK (chunk_size > 0)
);

CREATE INDEX idx_local_object_chunks_object ON local_object_chunks(object_id);
