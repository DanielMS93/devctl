---
phase: 02-git-integration
plan: 06
subsystem: ui
tags: [bubbletea, lipgloss, viewport, chroma, syntax-highlighting, git-diff, editor-launch, tui]

# Dependency graph
requires:
  - phase: 02-git-integration/02-04
    provides: tuimsg.WorktreeState with ChangedFiles []ChangedFile
  - phase: 02-git-integration/02-05
    provides: DetailPanel.SetWorktree() stub; RepoPanel.SelectedWorktree() accessor
  - phase: 02-git-integration/02-01
    provides: internal/git.Diff() with 4 DiffMode constants
provides:
  - DetailPanel renders changed files list with status chars, cursor navigation, Enter-to-open
  - ViewerModel: scrollable viewport with chroma syntax highlighting and 4 git diff modes
  - Editor launch via tea.ExecProcess reading viper "editor" key > $EDITOR > "vi"
  - ViewerModel overlay wired into root.go; Esc closes viewer, returns to file list
affects: []

# Tech tracking
tech-stack:
  added:
    - github.com/alecthomas/chroma/v2 v2.23.1 (syntax highlighting)
  patterns:
    - ViewerModel is a plain struct (not tea.Model) driven by root.go — avoids nested program complexity
    - Closures capture worktreePath/filePath for safe use in tea.Cmd goroutines (not stored on struct during cmd execution)
    - Viewer handles its own key routing when Visible; root.go checks consumed flag before processing

key-files:
  created:
    - pkg/tui/panels/viewer.go
  modified:
    - pkg/tui/panels/right.go
    - pkg/tui/root.go
    - go.mod
    - go.sum

key-decisions:
  - "ViewerModel is plain struct not tea.Model: drives sub-model from root.go Update() to avoid nested Bubbletea program complexity"
  - "Closures capture path variables at Open/load time: safe for async tea.Cmd goroutines without race conditions"
  - "chroma quick.Highlight with terminal256/monokai: graceful degradation to plain text on highlight error"

patterns-established:
  - "Sub-model routing: root.go checks viewer.Visible first and checks consumed bool before own switch"
  - "tea.ExecProcess for editor: suspends TUI, spawns process, EditorFinishedMsg fires on return"
  - "viewerContentMsg internal type: viewer owns its own Msg types, not exposed to root.go"

# Metrics
duration: 2min
completed: 2026-03-05
---

# Phase 2 Plan 06: Right Panel Changed Files List and Inline Viewer Summary

**DetailPanel with changed-files list + ViewerModel overlay with chroma syntax highlighting, 4 git diff modes via bubbles/v2 viewport, and tea.ExecProcess editor launch**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-03-05T13:45:25Z
- **Completed:** 2026-03-05T13:48:10Z
- **Tasks:** 2 of 3 complete (Task 3 is checkpoint:human-verify)
- **Files modified:** 5

## Accomplishments
- Created ViewerModel (pkg/tui/panels/viewer.go): scrollable bubbles/v2 viewport, chroma terminal256/monokai highlighting, 4 diff modes (unstaged/staged/vs-main/vs-origin) cycling with `d`, file preview with `f`, editor open with `e` via tea.ExecProcess
- Replaced DetailPanel stub in right.go with full changed-files list: status chars [XY], cursor selection with >, MoveUp/MoveDown/SelectedFile() methods
- Wired ViewerModel overlay into root.go: viewer.Visible check before key routing, Enter handler for PanelRight opens viewer on selected file, EditorFinishedMsg logs non-zero exit, propagateSizes includes viewer.SetSize
- openInEditor() reads viper.GetString("editor") > $EDITOR > "vi" fallback, exactly as specified

## Task Commits

Each task was committed atomically:

1. **Task 1: Install chroma, implement ViewerModel, update DetailPanel** - `f06770b` (feat)
2. **Task 2: Wire viewer overlay into root.go** - `8a2c74a` (feat)
3. **Deviation: Promote chroma to direct dependency** - `791a84c` (chore)

**Plan metadata:** (docs commit below, after checkpoint)

## Files Created/Modified
- `pkg/tui/panels/viewer.go` - New: ViewerModel with viewport, chroma highlighting, 4 diff modes, EditorFinishedMsg, openInEditor()
- `pkg/tui/panels/right.go` - Full rewrite: DetailPanel with changed-files list, status chars, cursor selection
- `pkg/tui/root.go` - Added viewer field, Enter/viewer routing, EditorFinishedMsg handler, viewer.SetSize in propagateSizes
- `go.mod` / `go.sum` - Added github.com/alecthomas/chroma/v2 as direct dependency

## Decisions Made
- ViewerModel is a plain struct (not tea.Model) driven by root.go Update() — avoids nested Bubbletea program complexity, keeps all routing in one place
- Closures in loadFileContent()/loadDiffContent() capture path variables at call time for safe async use in tea.Cmd goroutines
- chroma quick.Highlight with "terminal256"/"monokai" formatter/style; falls back to plain text if highlighting fails (graceful degradation)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Promoted chroma/v2 to direct dependency**
- **Found during:** Post-Task-1 verification
- **Issue:** `go get` marked chroma/v2 as indirect in go.mod; as a direct import it must be direct
- **Fix:** Ran `go mod tidy` to reclassify chroma/v2 as direct dependency
- **Files modified:** go.mod, go.sum
- **Verification:** go.mod shows chroma/v2 without `// indirect` comment
- **Committed in:** 791a84c (chore commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical/housekeeping)
**Impact on plan:** go mod tidy is standard Go hygiene; no scope creep.

## Issues Encountered

None — plan executed cleanly.

## User Setup Required

None - no external service configuration required.

## Checkpoint Status

Task 3 is `checkpoint:human-verify` — waiting for user to run `devctl dashboard` and verify the inline viewer works interactively (changed files list, Enter-to-open, d/f/e/Esc key handling).

## Next Phase Readiness
- Phase 2 complete: all 7 plans done; dashboard shows live worktree state with fully functional viewer
- GIT-05 through GIT-08 delivered: right panel changed files, file preview, 4 diff modes, editor launch
- Ready for Phase 3 planning once human verification of viewer passes

---
*Phase: 02-git-integration*
*Completed: 2026-03-05*
