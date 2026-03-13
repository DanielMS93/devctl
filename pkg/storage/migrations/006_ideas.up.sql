-- Phase 6: Ideas (quest-based Claude session orchestration).
-- ideas: parallel/sequential work items that spawn Claude sessions in worktrees.
-- idea_deps: directed dependency edges between ideas.
-- State is one of queued/ready/running/completed/failed; "blocked" is computed at runtime.

CREATE TABLE IF NOT EXISTS ideas (
    id                TEXT PRIMARY KEY,
    prompt            TEXT NOT NULL,
    state             TEXT NOT NULL DEFAULT 'queued',   -- queued | ready | running | completed | failed
    kind              TEXT NOT NULL DEFAULT 'side',     -- side (parallel) | sequential (blocking deps)
    repo_id           TEXT NOT NULL DEFAULT '',
    branch            TEXT NOT NULL DEFAULT '',
    parent_branch     TEXT NOT NULL DEFAULT '',         -- branch to merge back into
    worktree_path     TEXT NOT NULL DEFAULT '',
    parent_session_id TEXT NOT NULL DEFAULT '',
    session_id        TEXT NOT NULL DEFAULT '',
    incorporated      INTEGER NOT NULL DEFAULT 0,       -- 1 if /main-quest has incorporated this
    error_msg         TEXT,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    started_at        INTEGER,
    completed_at      INTEGER
);

CREATE INDEX IF NOT EXISTS idx_ideas_state ON ideas(state);
CREATE INDEX IF NOT EXISTS idx_ideas_repo_id ON ideas(repo_id);

CREATE TABLE IF NOT EXISTS idea_deps (
    idea_id       TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    depends_on_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (idea_id, depends_on_id),
    CHECK (idea_id != depends_on_id)
);

CREATE INDEX IF NOT EXISTS idx_idea_deps_depends_on_id ON idea_deps(depends_on_id);
