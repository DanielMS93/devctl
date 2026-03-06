---
phase: 06-ai-observability
plan: 01
subsystem: database
tags: [sqlite, sqlx, migrations, viper, agent]

requires:
  - phase: 05-tasks-and-dependencies
    provides: "migration pattern (004_tasks), store pattern (TaskStore), storage.Open/RunMigrations"
provides:
  - "agent_runs table for tracking agent workflow executions"
  - "agent_patches table for storing generated patches with review lifecycle"
  - "AgentRunStore with CRUD + ListByBranch + LastTriggered"
  - "PatchStore with CRUD + prefix match + ListByStatus + SetApplied/SetReverted"
  - "AgentConfig loaded from viper with workflow-level enable/disable"
affects: [06-02, 06-03, 06-04, 06-05, 06-06]

tech-stack:
  added: []
  patterns:
    - "Agent store follows same sqlx/context/error pattern as TaskStore"
    - "Nullable columns use pointer types (*int64, *string)"
    - "AgentConfig uses viper.SetDefault + explicit GetXxx reads"

key-files:
  created:
    - "pkg/storage/migrations/005_agent_patches.up.sql"
    - "pkg/storage/migrations/005_agent_patches.down.sql"
    - "internal/agent/store.go"
    - "internal/agent/store_test.go"
    - "internal/agent/config.go"
  modified: []

key-decisions:
  - "Nullable DB columns (completed_at, error_msg, description, etc.) use Go pointer types for proper NULL handling"
  - "AgentRunStore.UpdateStatus auto-sets completed_at on completed/failed status transitions"
  - "PatchStore.UpdateStatus auto-sets reviewed_at on approved/rejected status transitions"
  - "Known workflows (code_review, test_generation) loaded explicitly rather than dynamic mapstructure"

patterns-established:
  - "Agent package: store.go for data, config.go for viper config, matching Phase 5 task package structure"

duration: 2min
completed: 2026-03-06
---

# Phase 6 Plan 1: Agent Data Layer Summary

**SQLite migration 005 with agent_runs/agent_patches tables, CRUD stores with prefix-match Get, and viper-based AgentConfig**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-06T12:09:28Z
- **Completed:** 2026-03-06T12:12:18Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Migration 005 creates agent_runs and agent_patches tables with indexes
- AgentRunStore provides Create, Get, UpdateStatus, ListByBranch, LastTriggered
- PatchStore provides Create, Get (prefix match), ListByStatus, ListByRun, UpdateStatus, SetApplied, SetReverted
- AgentConfig loads from viper with sensible defaults for idle threshold, cooldown, max patch size, and per-workflow config
- 8 passing tests covering all store CRUD operations

## Task Commits

Each task was committed atomically:

1. **Task 1: Migration 005 + AgentRunStore and PatchStore** - `d4f50f5` (feat)
2. **Task 2: Agent configuration via viper** - `a8899de` (feat)

## Files Created/Modified
- `pkg/storage/migrations/005_agent_patches.up.sql` - agent_runs and agent_patches table schema
- `pkg/storage/migrations/005_agent_patches.down.sql` - drops both tables
- `internal/agent/store.go` - AgentRunStore and PatchStore with full CRUD
- `internal/agent/store_test.go` - 8 tests covering create, get, update, list, prefix match
- `internal/agent/config.go` - AgentConfig with viper defaults and duration helpers

## Decisions Made
- Nullable DB columns (completed_at, error_msg, description) use Go pointer types for proper NULL roundtrip
- UpdateStatus auto-sets completed_at/reviewed_at timestamps on terminal status transitions
- Known workflows loaded explicitly from viper rather than dynamic mapstructure unmarshalling

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Data layer ready for Plan 02 (idle detection) and Plan 03 (agent runner)
- All stores compile and pass tests; full project builds cleanly

---
*Phase: 06-ai-observability*
*Completed: 2026-03-06*
