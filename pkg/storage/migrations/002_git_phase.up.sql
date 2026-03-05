-- Phase 2: Git Integration schema additions.
-- worktree_state: caches last-polled git state per worktree.
-- Prevents stale display on startup before first poll completes.
-- behind=-1 is the sentinel value meaning "no upstream tracking branch configured".

CREATE TABLE IF NOT EXISTS worktree_state (
    worktree_id   TEXT PRIMARY KEY REFERENCES worktrees(id) ON DELETE CASCADE,
    branch        TEXT NOT NULL DEFAULT '',
    ahead         INTEGER NOT NULL DEFAULT 0,
    behind        INTEGER NOT NULL DEFAULT -1,  -- -1 = no upstream tracking branch
    staged        INTEGER NOT NULL DEFAULT 0,
    unstaged      INTEGER NOT NULL DEFAULT 0,
    untracked     INTEGER NOT NULL DEFAULT 0,
    polled_at     INTEGER NOT NULL DEFAULT 0    -- Unix timestamp (seconds)
);

-- repo_copy_files: files to copy from main worktree into each new linked worktree.
-- Stores exact relative paths (e.g. ".env", ".env.local").
-- Glob support deferred to a later phase.

CREATE TABLE IF NOT EXISTS repo_copy_files (
    id        TEXT PRIMARY KEY,                             -- UUID
    repo_id   TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL,                               -- relative path, e.g. ".env"
    UNIQUE(repo_id, pattern)
);

CREATE INDEX IF NOT EXISTS idx_repo_copy_files_repo_id ON repo_copy_files(repo_id);
