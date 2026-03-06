-- Phase 6: Agent observability schema.
-- agent_runs: tracks automated agent workflow executions per branch.
-- agent_patches: stores generated patches with review/apply lifecycle.

CREATE TABLE IF NOT EXISTS agent_runs (
    id            TEXT PRIMARY KEY,                                      -- UUID
    repo_path     TEXT NOT NULL,
    branch        TEXT NOT NULL,
    workflow      TEXT NOT NULL,                                         -- e.g. code_review, test_generation
    status        TEXT NOT NULL DEFAULT 'pending',                       -- pending | running | completed | failed
    triggered_at  INTEGER NOT NULL,                                     -- Unix timestamp (seconds)
    completed_at  INTEGER,
    error_msg     TEXT
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_repo_branch ON agent_runs(repo_path, branch);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status);

CREATE TABLE IF NOT EXISTS agent_patches (
    id            TEXT PRIMARY KEY,                                      -- UUID
    run_id        TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    repo_path     TEXT NOT NULL,
    branch        TEXT NOT NULL,
    title         TEXT NOT NULL,
    description   TEXT,
    patch_data    TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'draft',                         -- draft | approved | applied | rejected | reverted
    created_at    INTEGER NOT NULL,                                     -- Unix timestamp (seconds)
    reviewed_at   INTEGER,
    applied_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_agent_patches_status ON agent_patches(status);
CREATE INDEX IF NOT EXISTS idx_agent_patches_run_id ON agent_patches(run_id);
