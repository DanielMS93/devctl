---
phase: 02-git-integration
plan: "02"
subsystem: database
tags: [sqlite, golang-migrate, schema, migrations]

# Dependency graph
requires:
  - phase: 02-01
    provides: internal/git package establishing git CLI subprocess pattern
  - phase: 01-03
    provides: RunMigrations() embedded migration runner and storage.Open()

provides:
  - worktree_state table: caches last-polled git state per worktree (branch, ahead, behind, staged, unstaged, untracked, polled_at)
  - repo_copy_files table: stores file patterns to copy on worktree create (id, repo_id, pattern, UNIQUE constraint)
  - Migration 002 SQL embedded in binary via existing glob embed

affects:
  - 02-03 (worktree CLI copy-on-create reads repo_copy_files)
  - 02-04 (state polling persistence writes/reads worktree_state)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "golang-migrate glob embed: new .sql files in migrations/ are auto-picked without Go code changes"
    - "behind=-1 sentinel: -1 distinguishes 'no upstream' from '0 commits behind'"

key-files:
  created:
    - pkg/storage/migrations/002_git_phase.up.sql
    - pkg/storage/migrations/002_git_phase.down.sql
  modified: []

key-decisions:
  - "worktree_state.behind defaults to -1 (not 0): sentinel for no upstream tracking branch, consistent with internal/git PollState convention from 02-01"
  - "repo_copy_files uses UUID PK (id TEXT): consistent with repos and worktrees tables; pattern column stores exact relative paths, glob deferred"

patterns-established:
  - "Migration file naming: NNN_phase_name.up.sql / NNN_phase_name.down.sql"

# Metrics
duration: 2min
completed: 2026-03-05
---

# Phase 02 Plan 02: Git Integration Schema Migration Summary

**SQLite migration 002 adding worktree_state (git poll cache with behind=-1 sentinel) and repo_copy_files (copy-on-create patterns) tables, applied automatically at startup via embedded golang-migrate runner**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-05T13:33:50Z
- **Completed:** 2026-03-05T13:35:03Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Created `worktree_state` table with FK to worktrees, caching branch/ahead/behind/staged/unstaged/untracked/polled_at; behind=-1 sentinel for no upstream
- Created `repo_copy_files` table with FK to repos, UNIQUE(repo_id, pattern) constraint and supporting index
- Verified migration runs cleanly on fresh DB: version 2, dirty=false in schema_migrations
- No Go code changes required — existing `//go:embed migrations/*.sql` glob picks up new files automatically

## Task Commits

Each task was committed atomically:

1. **Task 1: Create migration 002 up and down SQL files** - `a4cfac4` (feat)
2. **Task 2: Verify migration runs end-to-end** - no commit (verification only, no code changes)

**Plan metadata:** (see final commit below)

## Files Created/Modified
- `pkg/storage/migrations/002_git_phase.up.sql` - DDL for worktree_state and repo_copy_files tables
- `pkg/storage/migrations/002_git_phase.down.sql` - DROP statements for clean rollback

## Decisions Made
- `worktree_state.behind` defaults to -1, consistent with the `Behind=-1` sentinel established in 02-01 for PollState (no upstream tracking branch)
- `repo_copy_files.pattern` stores exact relative paths; glob expansion deferred to later phase as noted in plan

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None - `go build ./...` passed immediately; migration applied cleanly on first run.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Schema is ready for 02-03 (worktree CLI will INSERT/SELECT on repo_copy_files) and 02-04 (state poller will INSERT/SELECT on worktree_state)
- No blockers

---
*Phase: 02-git-integration*
*Completed: 2026-03-05*
