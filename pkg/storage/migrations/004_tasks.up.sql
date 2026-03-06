-- Phase 5: Tasks and Dependencies schema.
-- tasks: user-defined work items scoped to a repo.
-- task_deps: directed dependency edges between tasks.
-- State is one of queued/running/completed; "blocked" is computed at runtime, never stored.

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,                                    -- UUID
    description TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'queued',                     -- queued | running | completed
    branch      TEXT NOT NULL DEFAULT '',                           -- optional linked git branch
    worktree_id TEXT NOT NULL DEFAULT '' REFERENCES worktrees(id) ON DELETE SET DEFAULT,
    repo_id     TEXT NOT NULL DEFAULT '' REFERENCES repos(id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL,                                   -- Unix timestamp (seconds)
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);
CREATE INDEX IF NOT EXISTS idx_tasks_repo_id ON tasks(repo_id);

CREATE TABLE IF NOT EXISTS task_deps (
    task_id       TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (task_id, depends_on_id),
    CHECK (task_id != depends_on_id)
);

CREATE INDEX IF NOT EXISTS idx_task_deps_depends_on_id ON task_deps(depends_on_id);
