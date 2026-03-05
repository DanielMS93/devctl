-- Initial schema: schema_migrations is managed by golang-migrate automatically.
-- This first migration creates the foundation tables used across all phases.

CREATE TABLE IF NOT EXISTS repos (
    id          TEXT PRIMARY KEY,         -- UUID
    path        TEXT NOT NULL UNIQUE,     -- absolute filesystem path
    name        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,         -- Unix timestamp (seconds)
    updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS worktrees (
    id          TEXT PRIMARY KEY,
    repo_id     TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    path        TEXT NOT NULL UNIQUE,
    branch      TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_worktrees_repo_id ON worktrees(repo_id);
