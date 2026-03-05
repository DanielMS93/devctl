---
phase: 02-git-integration
plan: 03
subsystem: cli
tags: [cobra, sqlite, worktrees, git, uuid]

# Dependency graph
requires:
  - phase: 02-01
    provides: git.AddWorktree, git.RemoveWorktree, git.ListWorktrees via git CLI subprocesses
  - phase: 01-03
    provides: storage.Open, DB schema with repos and worktrees tables, PersistentPreRunE pattern
provides:
  - devctl worktree list (GIT-01): lists all tracked worktrees from DB
  - devctl worktree create <repo-path> <branch> (GIT-02): adds linked worktree + DB row
  - devctl worktree delete <worktree-path> (GIT-03): removes linked worktree + DB row
  - ensureRepo: idempotent repo auto-registration on first worktree create
  - dbKey context pattern: *sqlx.DB flows through cobra context to all subcommands
affects: [02-07, dashboard-integration, tui-panels]

# Tech tracking
tech-stack:
  added: [github.com/google/uuid promoted from indirect to direct dependency]
  patterns:
    - dbKey{} context key: DB passed via cobra context, not global variable
    - PersistentPreRunE on root command: DB stored in context before any subcommand runs
    - ensureRepo auto-registration: worktree create auto-tracks repo without separate repo add step

key-files:
  created:
    - cmd/devctl/worktree.go
  modified:
    - cmd/devctl/main.go
    - go.mod

key-decisions:
  - "dbKey{} in worktree.go (same package main): avoids global *sqlx.DB, no package-level state"
  - "PersistentPreRunE on root: single injection point covers all current and future subcommands"
  - "ensureRepo auto-registers repos: worktree create is the primary entry point, not a separate repo command"
  - "sanitizeBranch replaces /, \\, :, *, ?, <, >, |, \" with dash: safe worktree directory names from branch names like feature/add-login"
  - "worktree directory as sibling of repo at filepath.Dir(absRepoPath)/safeBranch: predictable location without config"

patterns-established:
  - "Context injection for DB: root PersistentPreRunE sets dbKey{} in context; subcommands retrieve with cmd.Context().Value(dbKey{})"
  - "Auto-resource registration: worktree create calls ensureRepo before git operations; no separate register step needed"

# Metrics
duration: 2min
completed: 2026-03-05
---

# Phase 2 Plan 03: Worktree CLI Subcommands Summary

**Cobra worktree command with list/create/delete subcommands backed by SQLite and git CLI, with DB injected via cobra context key**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-03-05T13:33:57Z
- **Completed:** 2026-03-05T13:35:24Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- `devctl worktree list` queries worktrees JOIN repos and prints all tracked worktrees
- `devctl worktree create <repo-path> <branch>` auto-registers repo if needed, creates linked worktree via git, inserts DB row
- `devctl worktree delete <worktree-path>` looks up repo from DB, removes worktree via git, deletes DB row
- DB handle flows cleanly through cobra context (dbKey{}) without global state
- `go build ./...` and `go vet ./...` pass clean; `devctl worktree list` runs without panic

## Task Commits

Each task was committed atomically:

1. **Task 1: Create cmd/devctl/worktree.go with list, create, delete subcommands** - `5a335e9` (feat)
2. **Task 2: Wire worktreeCmd into main.go and pass DB via context** - `4da6ff9` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `cmd/devctl/worktree.go` - worktreeCmd parent plus list/create/delete subcommands, ensureRepo, sanitizeBranch, dbKey type
- `cmd/devctl/main.go` - PersistentPreRunE adds DB to context; root.AddCommand(worktreeCmd) registered
- `go.mod` - github.com/google/uuid promoted from indirect to direct dependency

## Decisions Made
- `dbKey{}` defined in worktree.go (package main): no global *sqlx.DB needed; DB travels through cobra context
- `PersistentPreRunE` on root command: single DB injection point covers all subcommands
- `ensureRepo` auto-registers repos on first worktree create: users do not need a separate `devctl repo add` step
- `sanitizeBranch` replaces path-unsafe characters with dash: branch `feature/add-login` becomes directory `feature-add-login`
- Worktree directory placed as sibling of repo (`filepath.Dir(absRepoPath)/safeBranch`): predictable location without config

## Deviations from Plan

None - plan executed exactly as written. The `var _ = storage.Open` placeholder was preemptively omitted (plan noted it would cause a compile error since storage is not needed in worktree.go directly).

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- GIT-01, GIT-02, GIT-03 complete; worktree CRUD is functional end-to-end
- Plan 02-07 can now wire file copy-on-create (GIT-09) after repo_copy_files table is populated
- Dashboard panels can query worktrees table once polling is wired in later plans

## Self-Check: PASSED

- FOUND: cmd/devctl/worktree.go
- FOUND: cmd/devctl/main.go
- FOUND: 02-03-SUMMARY.md
- FOUND: commit 5a335e9 (Task 1)
- FOUND: commit 4da6ff9 (Task 2)

---
*Phase: 02-git-integration*
*Completed: 2026-03-05*
