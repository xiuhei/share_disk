CREATE TABLE transfer_runs (
    task_id TEXT PRIMARY KEY,
    object_id TEXT NOT NULL,
    name TEXT NOT NULL,
    state TEXT NOT NULL,
    size INTEGER NOT NULL DEFAULT 0,
    transferred INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    source_endpoint TEXT,
    error TEXT,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_transfer_runs_state_updated ON transfer_runs(state, updated_at DESC);
