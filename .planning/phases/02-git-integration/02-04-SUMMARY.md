---
phase: 02-git-integration
plan: 04
subsystem: tui
tags: [bubbletea, sqlite, git, polling, state-management]

# Dependency graph
requires:
  - phase: 02-git-integration/02-01
    provides: git.PollState, git.WorktreeState, git.ChangedFile types
  - phase: 02-git-integration/02-02
    provides: worktree_state DB cache table schema
  - phase: 01-foundation/01-02
    provides: tuimsg leaf package pattern, StateSnapshot, StateEvent

provides:
  - tuimsg.WorktreeState and tuimsg.ChangedFile types (TUI-layer git state)
  - tuimsg.StateSnapshot.Worktrees []WorktreeState (populated by Manager)
  - Manager.pollLoop calling git.PollState per tracked worktree every 5s
  - Manager.loadCachedSnapshot for instant startup rendering from DB
  - Manager.persistState writing polled state to worktree_state cache table
  - mapGitState mapping function isolating the git->tuimsg boundary in Manager

affects:
  - 02-05 through 02-07 (TUI panels that consume StateSnapshot.Worktrees)
  - Wave 4 (all TUI panel rendering depends on populated Worktrees slice)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "TUI state independence: tuimsg never imports internal/git; Manager owns the mapping"
    - "Nil DB guard: Manager gracefully degrades when DB is nil (test-safe)"
    - "Drop-on-full emit: poller never blocks on a lagging TUI"
    - "Startup cache load: loadCachedSnapshot emits cached state before first poll tick"
    - "Sentinel preservation: Behind=-1 survives DB round-trip (stored as -1, read as -1)"

key-files:
  created: []
  modified:
    - pkg/tui/tuimsg/messages.go
    - internal/dashboard/manager.go

key-decisions:
  - "tuimsg must NOT import internal/git: Manager owns git->tuimsg mapping to preserve architectural boundary"
  - "nil DB guard in loadCachedSnapshot and pollAllWorktrees: enables test-time Manager(nil) without panic"
  - "Drop-on-full emit policy: channel full = TUI lagging; drop tick rather than block poller goroutine"

patterns-established:
  - "State mapping at subsystem boundary: git.WorktreeState -> tuimsg.WorktreeState happens in Manager, not in either leaf package"
  - "Startup cache emit: load from DB immediately, then live poll on ticker cadence"

# Metrics
duration: 2min
completed: 2026-03-05
---

# Phase 2 Plan 04: TUI-Git Wiring Summary

**tuimsg.StateSnapshot gains Worktrees []WorktreeState; Manager polls git.PollState per tracked worktree every 5s and persists results to worktree_state cache, wiring real git data into every TUI tick**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-03-05T13:37:52Z
- **Completed:** 2026-03-05T13:39:51Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- tuimsg.StateSnapshot now carries `Worktrees []WorktreeState` with full git state per worktree
- Manager.pollLoop replaced Phase 1 stub with real git.PollState calls on 5-second ticker
- Startup cache load (loadCachedSnapshot) ensures TUI renders immediately without waiting for first poll
- worktree_state cache table written after every poll via INSERT OR REPLACE
- Architectural boundary preserved: tuimsg has zero imports from internal/git

## Task Commits

1. **Task 1: Expand tuimsg.StateSnapshot with WorktreeState types** - `e058021` (feat)
2. **Task 2: Rewrite Manager.pollLoop with real git polling and DB cache** - `cd26d24` (feat)

**Plan metadata:** (docs commit — see below)

## Files Created/Modified

- `/Users/daniel/Projects/devctl/pkg/tui/tuimsg/messages.go` - Added ChangedFile, WorktreeState types; Worktrees field on StateSnapshot
- `/Users/daniel/Projects/devctl/internal/dashboard/manager.go` - Full rewrite: loadCachedSnapshot, pollAllWorktrees, mapGitState, persistState, nil DB guards

## Decisions Made

- tuimsg must NOT import internal/git: Manager owns the git->tuimsg mapping to preserve the TUI layer's independence from the subprocess layer
- nil DB guard added to loadCachedSnapshot and pollAllWorktrees: existing tests pass Manager(nil) and must not panic
- Drop-on-full emit: when events channel is full (TUI lagging), the poller drops the tick rather than blocking

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Added nil DB guards to Manager methods**
- **Found during:** Task 2 (run existing test suite after rewrite)
- **Issue:** TestManagerStartStop passes `nil` DB — a valid test-time pattern documented in the test comment. The new loadCachedSnapshot called `m.db.QueryxContext` unconditionally, causing a nil pointer panic.
- **Fix:** Added `if m.db == nil { return tuimsg.StateSnapshot{...} }` early-returns in both `loadCachedSnapshot` and `pollAllWorktrees`
- **Files modified:** internal/dashboard/manager.go
- **Verification:** `go test ./internal/dashboard/... -race -v` passes; both tests pass cleanly
- **Committed in:** cd26d24 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug)
**Impact on plan:** Required for test suite to pass; no scope creep. nil DB is a legitimate test pattern explicitly noted in the test file.

## Issues Encountered

None beyond the nil DB guard deviation above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- StateSnapshot.Worktrees is now populated with real git data on every TUI tick
- Wave 4 TUI panels (02-05 through 02-07) can now consume Worktrees slice for rendering
- worktree_state cache ensures startup feels instant even before first poll cycle

---
*Phase: 02-git-integration*
*Completed: 2026-03-05*
