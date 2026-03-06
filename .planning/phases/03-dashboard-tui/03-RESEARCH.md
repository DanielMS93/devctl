# Phase 3: Dashboard TUI - Research

**Researched:** 2026-03-06
**Domain:** Bubbletea v2 TUI gap analysis (Go)
**Confidence:** HIGH

## Summary

Phase 3's success criteria are largely already met by the existing codebase from Phases 1, 2, and 2.1. The three-panel layout, keyboard navigation, session launching, file viewing, diff viewing, and resize handling all exist and work. This research identifies the **specific remaining gaps** between the current implementation and the Phase 3 success criteria, and recommends minimal, targeted changes to close them.

The gaps are small and well-scoped: (1) a unified worktree status concept (running/idle/finished/interrupted/blocked) that aggregates session and git state, (2) direct-access keyboard shortcuts (`d`, `f`, `r`) from the main view (currently only work inside the viewer overlay), and (3) the `t` shortcut for task view which is a Phase 5 dependency and should be stubbed.

**Primary recommendation:** Close the gaps with 3-4 small, focused changes -- add a `WorktreeStatus` derived field, wire up shortcut keys from the right panel, and update the status bar hints. No architectural changes needed.

## Standard Stack

### Core (already in use -- no changes)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| charm.land/bubbletea/v2 | v2 | TUI framework | Already wired, v2 API throughout |
| charm.land/lipgloss/v2 | v2 | TUI styling | Already used in all panels |
| charm.land/bubbles/v2/viewport | v2 | Scrollable content | Used in ViewerModel |
| github.com/alecthomas/chroma/v2 | v2 | Syntax highlighting | Used in viewer |

### Supporting (already in use -- no changes)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/spf13/viper | latest | Config | session.active_threshold_minutes |
| github.com/jmoiron/sqlx | latest | DB access | Manager polling |
| modernc.org/sqlite | latest | SQLite (no CGO) | worktree_state cache |

**No new dependencies needed for Phase 3.**

## Architecture Patterns

### Existing Project Structure (no changes needed)
```
pkg/tui/
  root.go              # RootModel, key routing, panel composition
  panels/
    left.go            # RepoPanel (repos + worktrees)
    right.go           # DetailPanel (sessions + files)
    viewer.go          # ViewerModel overlay (file preview, diff, editor)
    logs.go            # LogBar bottom status bar
  tuimsg/
    messages.go        # Shared message types (leaf pkg)
internal/
  dashboard/
    manager.go         # Background poller, DB + Claude discovery
  claude/
    scanner.go         # Session scanning
  git/                 # Git state polling
```

### Pattern: Derived Status from Existing Data
**What:** Compute a `WorktreeStatus` (running/idle/finished/interrupted/blocked) from existing `WorktreeState` fields -- sessions' IsActive flags, git stats, and branch state. This is a pure function, not new polling.
**When to use:** Whenever the left panel renders a worktree row.
**Example:**
```go
// In tuimsg/messages.go or panels/left.go

type WorktreeStatus int
const (
    StatusIdle WorktreeStatus = iota
    StatusRunning    // has at least one ACTIVE session
    StatusBlocked    // has merge conflicts (could check for UU in ChangedFiles)
    // "finished" and "interrupted" require task system (Phase 5)
    // For now: Running (active session) or Idle (no active session)
)

func DeriveStatus(wt WorktreeState) WorktreeStatus {
    for _, s := range wt.Sessions {
        if s.IsActive {
            return StatusRunning
        }
    }
    return StatusIdle
}
```

### Pattern: Key Routing from Right Panel
**What:** Add `d`, `f`, `r` shortcuts that work when the right panel is focused (not just inside the viewer overlay). The right panel knows which file/session is selected; these shortcuts act on the selection.
**When to use:** In root.go's `Update()` under `PanelRight` key handling.
**Example:**
```go
// In root.go Update(), under case "d":
case "d":
    if m.activePanel == PanelRight {
        if f := m.rightPanel.SelectedFile(); f != "" {
            if wt := m.leftPanel.SelectedWorktree(); wt != nil {
                // Open viewer directly in diff mode
                cmd := m.viewer.OpenDiff(wt.WorktreePath, f)
                return m, cmd
            }
        }
    }
case "f":
    if m.activePanel == PanelRight {
        if f := m.rightPanel.SelectedFile(); f != "" {
            if wt := m.leftPanel.SelectedWorktree(); wt != nil {
                return m, m.viewer.Open(wt.WorktreePath, f)
            }
        }
    }
case "r":
    if m.activePanel == PanelRight {
        if s := m.rightPanel.SelectedSession(); s != nil {
            return m, LaunchClaudeSession(s.ID, s.ProjectPath)
        }
    }
```

### Anti-Patterns to Avoid
- **Adding new goroutines:** All async work must go through tea.Cmd. The existing channel subscription pattern handles this.
- **Duplicating state:** Don't store WorktreeStatus separately; derive it from existing WorktreeState fields.
- **Breaking the tuimsg leaf constraint:** tuimsg must not import internal/git or pkg/tui/panels. The status derivation function can live in panels/left.go.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Scrollable file viewer | Custom scrolling | bubbles/viewport (already used) | Edge cases with ANSI, wrapping |
| Syntax highlighting | Custom lexer | chroma/v2 (already used) | Language detection, themes |
| Terminal session launch | Raw exec.Command | Existing AppleScript approach | Terminal app detection, PATH issues |

**Key insight:** Phase 3 is a gap-closing phase, not a building phase. Everything structural already exists.

## Common Pitfalls

### Pitfall 1: Over-Engineering Worktree Status
**What goes wrong:** Building a complex status state machine with "finished" and "interrupted" states that require task/process tracking which doesn't exist yet (Phase 5).
**Why it happens:** The success criteria mention these statuses but the task system isn't built yet.
**How to avoid:** Implement only what's derivable from current data: Running (active session) and Idle (no active session). Add Blocked if merge conflicts are detectable. Stub the others.
**Warning signs:** If you're adding new fields to WorktreeState or new polling to Manager, you're over-building.

### Pitfall 2: Keyboard Shortcut Conflicts
**What goes wrong:** Adding `d`, `f`, `r` to the main key handler without checking if the viewer is open, causing double-handling.
**Why it happens:** The viewer already handles `d` and `f` when visible. Root.go checks `viewer.Visible` first and returns early if consumed.
**How to avoid:** The existing pattern in root.go already handles this correctly -- viewer gets first pass. New shortcuts in the `PanelRight` case only fire when the viewer is NOT visible (consumed=true causes early return).

### Pitfall 3: `t` for Task View (Phase 5 dependency)
**What goes wrong:** Trying to implement task view when the task system doesn't exist.
**Why it happens:** Success criterion 3 lists `t` as a shortcut.
**How to avoid:** Register the `t` keybinding but show a "Tasks: coming soon" message in the log bar. Don't build task infrastructure.

### Pitfall 4: Status Bar Not Updating Hints
**What goes wrong:** Adding new keyboard shortcuts but forgetting to update the LogBar and the right panel's help bar to show them.
**Why it happens:** Hints are hardcoded strings in logs.go and right.go.
**How to avoid:** Update both hint strings when adding new shortcuts.

## Code Examples

### Adding OpenDiff to ViewerModel
```go
// In viewer.go -- new method to open directly in diff mode
func (v *ViewerModel) OpenDiff(worktreePath, filePath string) tea.Cmd {
    v.Visible = true
    v.worktreePath = worktreePath
    v.filePath = filePath
    v.diffMode = 0
    v.showingDiff = true
    v.vp = viewport.New(viewport.WithWidth(v.width-4), viewport.WithHeight(v.height-6))
    return v.loadDiffContent()
}
```

### Worktree Status Indicator in Left Panel
```go
// In left.go -- status indicator prefix for worktree rows
func statusIndicator(wt tuimsg.WorktreeState) string {
    for _, s := range wt.Sessions {
        if s.IsActive {
            return "●" // green dot for running
        }
    }
    // Check for conflicts (UU status in changed files)
    for _, cf := range wt.ChangedFiles {
        if cf.StagedStatus == 'U' || cf.UnstagedStatus == 'U' {
            return "!" // blocked
        }
    }
    return "○" // idle
}
```

### Updated Help Hints
```go
// In logs.go -- context-aware help
func (b LogBar) View() string {
    text := " q=quit  tab=panels  ↑↓=navigate  enter=expand  d=diff  f=file  r=resume"
    if b.status != "" {
        text = "  " + b.status
    }
    // ... render
}
```

## Gap Analysis: Success Criteria vs Current State

| Criterion | Current State | Gap | Effort |
|-----------|--------------|-----|--------|
| Three-panel layout | EXISTS: left + right + log bar | None | 0 |
| Visual status per worktree | Sessions show ACTIVE/IDLE; worktrees show git stats + session count | Need unified status indicator (running/idle/blocked) on worktree rows | Small |
| up/down navigation | EXISTS: works in both panels | None | 0 |
| enter to expand | EXISTS: left->right panel, right opens file/launches session | None | 0 |
| `d` to view diff | Only works inside viewer overlay | Add `d` shortcut from right panel (file selected) | Small |
| `f` to view file | Only works inside viewer overlay (enter already does this) | Add `f` as alias for enter-on-file from right panel | Small |
| `r` to restore session | Enter on session already does this | Add `r` as alias for enter-on-session from right panel | Small |
| `t` to view task | Tasks don't exist (Phase 5) | Stub with log bar message | Trivial |
| Resize handling | EXISTS: WindowSizeMsg propagated to all panels | None | 0 |

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| tea.KeyMsg | tea.KeyPressMsg | Bubbletea v2 | Already using v2 throughout |
| Init() returns (Model, Cmd) | Init() returns Cmd | Bubbletea v2 | Already using v2 |
| tea.EnterAltScreen command | v.AltScreen = true | Bubbletea v2 | Already using v2 |

No deprecated patterns in use. Codebase is current with Bubbletea v2.

## Open Questions

1. **What constitutes "finished" and "interrupted" status?**
   - What we know: Sessions have ACTIVE/IDLE. Git has dirty/clean state.
   - What's unclear: These statuses imply task/process lifecycle tracking that doesn't exist until Phase 5.
   - Recommendation: For Phase 3, derive only Running (active session) and Idle (no active session). Add Blocked if merge conflicts detected. Document that finished/interrupted require Phase 5 task system.

2. **Should `r` always launch in iTerm2 split, or should it support other terminals?**
   - What we know: Current implementation supports iTerm2 split pane and Terminal.app new tab.
   - What's unclear: Whether Linux terminal support is needed.
   - Recommendation: Keep current macOS-only approach. The `r` shortcut just calls the existing `LaunchClaudeSession` function.

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection of all TUI source files (root.go, left.go, right.go, viewer.go, logs.go, messages.go, manager.go, scanner.go)
- Bubbletea v2 API patterns verified from existing working code

### Secondary (MEDIUM confidence)
- MEMORY.md project conventions and phase history

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no changes needed, everything already exists
- Architecture: HIGH - no structural changes, just wiring shortcuts and adding a derived status
- Pitfalls: HIGH - based on direct code reading and understanding of existing patterns
- Gap analysis: HIGH - direct comparison of success criteria against actual source code

**Research date:** 2026-03-06
**Valid until:** 2026-04-06 (stable -- no external dependencies changing)
