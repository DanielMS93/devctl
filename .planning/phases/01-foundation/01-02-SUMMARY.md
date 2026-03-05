---
phase: 01-foundation
plan: 02
subsystem: tui
tags: [go, bubbletea, bubbletea-v2, lipgloss, concurrency, channel, goroutine, tui, state-manager]

# Dependency graph
requires:
  - phase: 01-01
    provides: Go module with Charm v2 TUI stack in go.mod; sqlx dependency for Manager

provides:
  - Manager struct (internal/dashboard/manager.go) with Start/Stop/Events, buffered channel size 32
  - StateEvent and StateSnapshot types in pkg/tui/tuimsg (leaf package, no import cycles)
  - StateEvent/StateSnapshot re-exported via pkg/tui/messages.go type aliases
  - RootModel implementing tea.Model with v2 Init/Update/View signatures
  - Three stub panels: RepoPanel (left), DetailPanel (right), LogBar (bottom)
  - Recursive subscription pattern: subscribeToStateEvents() re-armed on every StateEvent

affects: [03-tui-shell, 02-worktree-manager, 04-session-tracking, 05-dependency-graph, 06-agent-integration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Recursive subscription: subscribeToStateEvents() returns tea.Cmd that blocks on channel; re-armed in Update case StateEvent"
    - "Buffered channel size 32: pollers never block on TUI lag; inner select on send prevents goroutine ignoring shutdown"
    - "ctx.Done() in every select branch: goroutine exits cleanly on cancellation, no leaks"
    - "tuimsg leaf package: shared types live in pkg/tui/tuimsg to break pkg/tui <-> pkg/tui/panels import cycle"
    - "v2 Init() returns tea.Cmd (not (Model,Cmd)); View() returns tea.View via tea.NewView(); AltScreen declarative field"
    - "KeyPressMsg (v2) replaces KeyMsg (v1 removed); no raw goroutines in Update()"

key-files:
  created:
    - internal/dashboard/manager.go
    - pkg/tui/tuimsg/messages.go
    - pkg/tui/messages.go
    - pkg/tui/root.go
    - pkg/tui/panels/left.go
    - pkg/tui/panels/right.go
    - pkg/tui/panels/logs.go
  modified: []

key-decisions:
  - "pkg/tui/tuimsg leaf package: breaks import cycle between pkg/tui and pkg/tui/panels; both can import tuimsg without circular dependency"
  - "Type aliases in pkg/tui/messages.go: StateEvent = tuimsg.StateEvent preserves public API while resolving the cycle"
  - "v2 Init() signature: returns tea.Cmd not (tea.Model,tea.Cmd); plan code had wrong signature, corrected to actual v2 API"
  - "AltScreen as View field: v.AltScreen = true in View() rather than command; v2 removed EnterAltScreen command"

patterns-established:
  - "State subscription: subscribe once in Init(), re-arm after every StateEvent in Update() — one goroutine waiting at a time"
  - "Panel sizing: propagateSizes() called on WindowSizeMsg and activePanel change; panels never hold hardcoded dimensions"
  - "Manager lifecycle: NewManager() -> Start(ctx) -> Events() -> Stop(); cancel is stored on Manager for safe multi-call"

# Metrics
duration: 3min
completed: 2026-03-05
---

# Phase 1 Plan 02: Background State Manager and Bubbletea v2 TUI Skeleton Summary

**Context-cancelled Manager goroutine emitting StateEvents via buffered channel (size 32) wired to a Bubbletea v2 RootModel using recursive subscription, three stub panels, and correct v2 Init/View/KeyPressMsg APIs**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-05T07:26:44Z
- **Completed:** 2026-03-05T07:29:45Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments

- Manager with Start/Stop/Events, context-cancelled pollLoop, and buffered channel (size 32) that never blocks pollers on TUI lag
- RootModel implementing the Bubbletea v2 Model interface: Init() returns Cmd, View() returns tea.View, keys use KeyPressMsg
- Recursive subscription pattern: subscribeToStateEvents() re-armed in Update on each StateEvent — exactly one goroutine waiting at a time
- Three stub panels (RepoPanel, DetailPanel, LogBar) sized dynamically from WindowSizeMsg with Lipgloss borders
- pkg/tui/tuimsg leaf package resolves the import cycle between pkg/tui and pkg/tui/panels

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement background state manager with context-cancelled goroutine** - `ac480a2` (feat)
2. **Task 2: Implement Bubbletea v2 RootModel and three stub panels** - `23d34cd` (feat)

**Plan metadata:** (docs commit follows this summary)

## Files Created/Modified

- `internal/dashboard/manager.go` - Manager struct with Start/Stop/Events, buffered channel 32, context-safe pollLoop
- `pkg/tui/tuimsg/messages.go` - Leaf package with StateEvent/StateSnapshot; prevents import cycles
- `pkg/tui/messages.go` - Re-exports StateEvent/StateSnapshot as type aliases from tuimsg
- `pkg/tui/root.go` - RootModel: v2 Init/Update/View, subscribeToStateEvents(), propagateSizes()
- `pkg/tui/panels/left.go` - RepoPanel with SetSize/SetState/SetFocused/View()
- `pkg/tui/panels/right.go` - DetailPanel with SetSize/SetFocused/View()
- `pkg/tui/panels/logs.go` - LogBar with SetWidth/View()

## Decisions Made

- **pkg/tui/tuimsg leaf package:** The plan's design had panels importing `tui.StateEvent` from the parent `pkg/tui` package, which imports `pkg/tui/panels` — a circular dependency. Created `pkg/tui/tuimsg` as a leaf package (no imports from either parent) to hold the shared types. `pkg/tui/messages.go` re-exports via type aliases so external callers see the same API.
- **Type aliases over re-declaration:** Used `type StateEvent = tuimsg.StateEvent` (alias, not new type) so `internal/dashboard` emitting `tuimsg.StateEvent` and `pkg/tui` handling `StateEvent` in Update's type switch are the same type with no conversion.
- **v2 Init() signature correction:** The plan showed `Init() (tea.Model, tea.Cmd)` but the actual Bubbletea v2 Model interface requires `Init() Cmd`. Implemented the correct v2 signature.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected Init() return type to match actual Bubbletea v2 API**
- **Found during:** Task 2 (RootModel implementation)
- **Issue:** Plan showed `Init() (tea.Model, tea.Cmd)` — the v1 signature. Bubbletea v2 Model interface defines `Init() Cmd` (single return value). Code with v1 signature would not satisfy the v2 Model interface and would not compile.
- **Fix:** Implemented `func (m RootModel) Init() tea.Cmd` matching the actual v2 interface.
- **Files modified:** pkg/tui/root.go
- **Verification:** `go build ./...` passes; interface is satisfied.
- **Committed in:** 23d34cd (Task 2 commit)

**2. [Rule 1 - Bug] Created pkg/tui/tuimsg to resolve import cycle**
- **Found during:** Task 2 (first build attempt)
- **Issue:** `pkg/tui` imports `pkg/tui/panels` (for RepoPanel/DetailPanel/LogBar in root.go); `pkg/tui/panels/left.go` imports `pkg/tui` for `tui.StateEvent` — circular dependency, Go refuses to compile.
- **Fix:** Created `pkg/tui/tuimsg/messages.go` as a leaf package with StateEvent and StateSnapshot. Panels import tuimsg; pkg/tui/messages.go re-exports as type aliases. No cycle remains.
- **Files modified:** pkg/tui/tuimsg/messages.go (new), pkg/tui/messages.go (updated to aliases), pkg/tui/panels/left.go (import updated)
- **Verification:** `go build ./...` passes with zero cycle errors.
- **Committed in:** 23d34cd (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 bugs)
**Impact on plan:** Both fixes required for compilation. The tuimsg approach is an additive structural fix that maintains the plan's intent (shared types accessible to both tui and panels). No scope creep.

## Issues Encountered

- Bubbletea v2 Init() signature differs from plan — confirmed against `go doc charm.land/bubbletea/v2 Model` before writing code.
- Import cycle from panels importing parent package — standard Go pattern resolved by extracting shared types to a leaf package.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Manager.Start/Stop/Events ready for Plan 03 (TUI shell / main.go) to wire into cobra command
- RootModel.NewRootModel(events) ready to receive Manager.Events() channel
- Panel sizing infrastructure established; Phase 2 populates panels with real git state
- tuimsg types established as the stable wire format between Manager and TUI

## Self-Check: PASSED

- All 7 implementation files exist on disk
- Commits ac480a2 and 23d34cd verified in git log
- `go build ./...` passes with zero errors
- `go vet ./...` passes with zero warnings

---
*Phase: 01-foundation*
*Completed: 2026-03-05*
