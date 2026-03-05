-- Reverse migration 002: drop Phase 2 tables.
DROP INDEX IF EXISTS idx_repo_copy_files_repo_id;
DROP TABLE IF EXISTS repo_copy_files;
DROP TABLE IF EXISTS worktree_state;
