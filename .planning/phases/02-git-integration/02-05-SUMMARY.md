---
phase: 02-git-integration
plan: 05
subsystem: ui
tags: [bubbletea, lipgloss, tui, worktree, navigation]

# Dependency graph
requires:
  - phase: 02-git-integration/02-04
    provides: tuimsg.WorktreeState with Ahead/Behind/Staged/Unstaged/Untracked fields delivered via StateEvent
provides:
  - RepoPanel renders live WorktreeState rows with ahead/behind badge and status counts
  - Arrow key (up/down/j/k) navigation through worktree list
  - RepoPanel.SelectedWorktree() accessor for right panel consumption
  - Empty state message when no worktrees tracked
  - DetailPanel.SetWorktree() stub for Plan 02-06 to implement
affects: [02-06]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Cursor navigation stored as int index with bounds clamping in SetState/MoveUp/MoveDown
    - Selection propagated to right panel on every navigation event and StateEvent

key-files:
  created: []
  modified:
    - pkg/tui/panels/left.go
    - pkg/tui/root.go
    - pkg/tui/panels/right.go

key-decisions:
  - "Selection propagated to rightPanel immediately on navigation and on every StateEvent to keep panels in sync"
  - "Counts rendered only when non-zero to reduce visual noise per plan spec"
  - "SetWorktree() stub in DetailPanel lets 02-06 fill in without compilation breakage"

patterns-established:
  - "Clamped selection: SetState clamps selected index to len-1 after slice update"
  - "Stub-first: DetailPanel.SetWorktree() is a no-op until Plan 02-06 implements it"

# Metrics
duration: 5min
completed: 2026-03-05
---

# Phase 2 Plan 05: Left Panel Worktree List and Navigation Summary

**RepoPanel renders live WorktreeState rows with ahead/behind badges and staged/unstaged/untracked counts; arrow key and j/k navigation with selection propagated to right panel**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-03-05T00:00:00Z
- **Completed:** 2026-03-05T00:05:00Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Replaced placeholder RepoPanel with full worktree list renderer showing branch name, ahead/behind badge, and non-zero status counts
- Added MoveUp()/MoveDown() cursor navigation with bounds clamping; SelectedWorktree() accessor returns pointer to current row
- Wired up/down and j/k keys in root.go routing to left panel when focused; selection propagated to right panel after every navigation and StateEvent
- Added SetWorktree() no-op stub to DetailPanel so build compiles cleanly before Plan 02-06 fills it in
- Empty state message renders when Worktrees slice is empty

## Task Commits

Each task was committed atomically:

1. **Task 1: Rewrite RepoPanel with worktree list rendering and navigation** - `dd21c95` (feat)
2. **Task 2: Wire up/down key routing in root.go and propagate selection to right panel** - `94de815` (feat)

**Plan metadata:** (docs commit below)

## Files Created/Modified
- `pkg/tui/panels/left.go` - Full rewrite: WorktreeState list, navigation, badges, empty state, SelectedWorktree()
- `pkg/tui/root.go` - Added up/k and down/j key handling routed to left panel; SetWorktree() calls on nav and StateEvent
- `pkg/tui/panels/right.go` - Added SetWorktree(*tuimsg.WorktreeState) stub and tuimsg import

## Decisions Made
- Selection is propagated to the right panel on every navigation keystroke and on every StateEvent so both panels always reflect the same row without extra coordination logic.
- Status counts (S/U/?) rendered only when non-zero, reducing visual noise as specified in plan.
- SetWorktree() stub avoids a forward-reference build failure ahead of Plan 02-06.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Plan 02-06 can now implement DetailPanel.SetWorktree() to display changed files for the selected worktree
- SelectedWorktree() returns the correct WorktreeState pointer (or nil when empty) ready for right panel consumption

---
*Phase: 02-git-integration*
*Completed: 2026-03-05*
