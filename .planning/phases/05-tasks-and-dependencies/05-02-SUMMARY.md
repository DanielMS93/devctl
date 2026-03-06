---
phase: 05-tasks-and-dependencies
plan: 02
subsystem: cli
tags: [cobra, task-management, dependency-graph, cycle-detection]

# Dependency graph
requires:
  - phase: 05-tasks-and-dependencies
    plan: 01
    provides: "TaskStore and DepStore CRUD data layer"
  - phase: 01-foundation
    provides: "Cobra CLI structure, DB context injection via PersistentPreRunE"
provides:
  - "devctl tasks create/list/update/delete CLI commands"
  - "devctl deps add/remove/list CLI commands"
  - "Cycle detection on dependency insertion via DFS"
affects: [05-03, 05-04, dashboard-tasks]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Task/deps commands follow existing Cobra pattern from repo.go/worktree.go"
    - "Prefix-match ID resolution for all task-referencing commands"
    - "Cycle detection via upstream DFS before dep insertion"

key-files:
  created:
    - cmd/devctl/task.go
    - cmd/devctl/deps.go
  modified:
    - cmd/devctl/main.go

key-decisions:
  - "Partial update via Get-then-Update: fetch current task to preserve unchanged fields when only --state or --branch provided"
  - "Cycle detection in CLI layer (deps.go) not store layer: keeps store simple, CLI owns validation logic"
  - "lookupRepoID helper in task.go for repo flag resolution: avoids exposing repo internals"

patterns-established:
  - "CLI cycle detection pattern: fetch all deps, run wouldCreateCycle DFS, reject before Add"
  - "Partial update pattern: Get current, merge flags, Update full record"

# Metrics
duration: 2min
completed: 2026-03-06
---

# Phase 5 Plan 2: CLI Commands Summary

**Cobra subcommands for task CRUD and dependency management with DFS cycle detection**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-06T11:15:18Z
- **Completed:** 2026-03-06T11:17:06Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Four task subcommands (create, list, update, delete) with --repo, --branch, --state flags
- Three dependency subcommands (add, remove, list) with --task filter flag
- Cycle detection via DFS on upstream edges prevents circular dependencies
- Prefix-match ID resolution for ergonomic short task IDs

## Task Commits

Each task was committed atomically:

1. **Task 1: devctl tasks subcommands** - `ac2b791` (feat)
2. **Task 2: devctl deps subcommands with cycle detection** - `ccf01a8` (feat)

## Files Created/Modified
- `cmd/devctl/task.go` - Task CRUD subcommands (create, list, update, delete) with repo/branch flags
- `cmd/devctl/deps.go` - Dependency management subcommands (add, remove, list) with cycle detection
- `cmd/devctl/main.go` - Registration of taskCmd and depsCmd on rootCmd

## Decisions Made
- Partial update via Get-then-Update pattern: CLI fetches current task state, merges with provided flags, then calls Update with full record. This avoids modifying the store API.
- Cycle detection lives in CLI layer (deps.go) not the store: keeps DepStore as a pure data layer while the CLI owns validation logic.
- lookupRepoID helper encapsulates repo path-to-ID resolution for the --repo flag without exposing repo internals.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CLI commands ready for dependency resolver integration (Plan 05-03)
- wouldCreateCycle function can be reused or extended by resolver
- Dashboard integration (Plan 05-04) can use same TaskStore/DepStore

---
*Phase: 05-tasks-and-dependencies*
*Completed: 2026-03-06*
