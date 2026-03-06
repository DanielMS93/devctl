---
phase: 06-ai-observability
plan: 06
subsystem: tui
tags: [bubbletea, viewport, patches, agent, approve-reject]

# Dependency graph
requires:
  - phase: 06-01
    provides: AgentRunStore and PatchStore with CRUD operations
  - phase: 06-04
    provides: IdleTracker and WorkflowRunner for agent automation
  - phase: 06-05
    provides: Patch lifecycle CLI (apply/revert/edit) and agent config
provides:
  - PatchPanel TUI component for viewing/approving/rejecting agent patches
  - AgentPatch and PatchSnapshot tuimsg types for TUI rendering
  - Manager integration querying patches on each poll cycle
  - PatchStatusUpdater interface for import-cycle-safe DB operations from TUI
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: [PatchStatusUpdater interface for cross-layer DB access, overlay panel pattern for patch review]

key-files:
  created: [pkg/tui/panels/patches.go]
  modified: [pkg/tui/tuimsg/messages.go, internal/dashboard/manager.go, pkg/tui/root.go, pkg/tui/panels/logs.go, pkg/tui/messages.go, cmd/devctl/main.go]

key-decisions:
  - "PatchStatusUpdater interface in panels pkg prevents import cycle with internal/agent while enabling direct DB updates from TUI"
  - "Explicit nil check for PatchStore->interface conversion avoids non-nil interface wrapping nil pointer"
  - "Apply/revert remain CLI-only since they modify working tree; approve/reject are safe DB-only operations for TUI"

patterns-established:
  - "PatchStatusUpdater interface: minimal interface in consumer package satisfied by producer package"
  - "Direct DB operations from TUI panels via interface injection"

# Metrics
duration: 4min
completed: 2026-03-06
---

# Phase 6 Plan 6: Patch Review Panel Summary

**TUI patch review panel with list/diff views, cursor navigation, and direct approve/reject via DB interface injection**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-06T14:52:46Z
- **Completed:** 2026-03-06T14:56:46Z (Tasks 1-2; Task 3 checkpoint pending)
- **Tasks:** 2 of 3 (Task 3 is human verification checkpoint)
- **Files modified:** 7

## Accomplishments
- PatchPanel displays agent patches with status badges, branch, and age
- Diff view shows full patch content in scrollable viewport
- Direct approve ('a') and reject ('x') update DB status from TUI without CLI delegation
- Manager queries PatchStore for draft/approved/applied patches on each poll cycle

## Task Commits

Each task was committed atomically:

1. **Task 1: Patch TUI types + Manager integration** - `c14daee` (feat)
2. **Task 2: PatchPanel with direct approve/reject + root.go wiring** - `ea8de6b` (feat)
3. **Task 3: Full Phase 6 verification** - checkpoint:human-verify (pending)

## Files Created/Modified
- `pkg/tui/panels/patches.go` - PatchPanel with list/diff views, approve/reject, PatchStatusUpdater interface
- `pkg/tui/tuimsg/messages.go` - AgentPatch and PatchSnapshot types added to StateSnapshot
- `internal/dashboard/manager.go` - collectPatches and mapAgentPatches, PatchStore() getter
- `pkg/tui/root.go` - PatchPanel wiring: 'p' key, 'a'/'x' approve/reject, propagateSizes
- `pkg/tui/panels/logs.go` - Added p=patches to hint bar
- `pkg/tui/messages.go` - Re-exported PatchStatusUpdater from panels
- `cmd/devctl/main.go` - Passes PatchStore as PatchStatusUpdater to NewRootModel

## Decisions Made
- PatchStatusUpdater interface in panels package prevents import cycle with internal/agent
- Explicit nil check when converting *agent.PatchStore to PatchStatusUpdater interface
- Apply/revert kept as CLI-only operations (working tree modifications)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed viewport API for Bubbletea v2**
- **Found during:** Task 2 (PatchPanel creation)
- **Issue:** Used LineUp/LineDown which don't exist in bubbles v2 viewport
- **Fix:** Changed to ScrollUp/ScrollDown matching existing session_viewer.go pattern
- **Files modified:** pkg/tui/panels/patches.go
- **Verification:** go build passes
- **Committed in:** ea8de6b (Task 2 commit)

**2. [Rule 1 - Bug] Removed duplicate formatAge function**
- **Found during:** Task 2 (PatchPanel creation)
- **Issue:** formatAge already declared in right.go, redeclaration in patches.go caused compile error
- **Fix:** Removed duplicate, reuse existing function from right.go
- **Files modified:** pkg/tui/panels/patches.go
- **Verification:** go build passes
- **Committed in:** ea8de6b (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 bugs)
**Impact on plan:** Both fixes necessary for compilation. No scope creep.

## Issues Encountered
- viewer.go had uncommitted changes from prior phase (LaunchNewClaudeSession); included in Task 2 commit as it was required for compilation

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Task 3 checkpoint pending: full Phase 6 verification
- All code changes complete and compiling
- All tests passing

---
*Phase: 06-ai-observability*
*Completed: 2026-03-06 (pending Task 3 checkpoint)*
