CREATE TABLE shares (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_entry_id UUID NOT NULL REFERENCES file_entries(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    max_downloads INTEGER NOT NULL DEFAULT 0,
    download_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT shares_status_check CHECK (status IN ('active','revoked')),
    CONSTRAINT shares_downloads_check CHECK (max_downloads >= 0 AND download_count >= 0)
);

CREATE INDEX idx_shares_user_created ON shares(user_id, created_at DESC);
CREATE INDEX idx_shares_file ON shares(file_entry_id, status);
