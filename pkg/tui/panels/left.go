package panels

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/pkg/tui/tuimsg"
)

// leftItem is one row in the left panel's flat render list.
// Headers are repo names (not selectable); worktree rows are selectable.
type leftItem struct {
	isHeader bool
	label    string               // repo name, for header items
	wt       tuimsg.WorktreeState // worktree data, for non-header items
}

// RepoPanel is the left pane showing repos grouped with their worktrees.
type RepoPanel struct {
	width    int
	height   int
	focused  bool
	selected int // index among selectable items only

	items     []leftItem // flat render list (headers + worktree rows)
	selectIdx []int      // positions in items[] that are selectable
}

func NewRepoPanel() RepoPanel { return RepoPanel{} }

func (p *RepoPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *RepoPanel) SetFocused(focused bool) { p.focused = focused }

func (p *RepoPanel) SetState(e tuimsg.StateEvent) {
	p.items, p.selectIdx = buildLeftItems(e.Snapshot.Worktrees)
	if p.selected >= len(p.selectIdx) && len(p.selectIdx) > 0 {
		p.selected = len(p.selectIdx) - 1
	}
}

func (p *RepoPanel) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

func (p *RepoPanel) MoveDown() {
	if p.selected < len(p.selectIdx)-1 {
		p.selected++
	}
}

// SelectedWorktree returns the currently selected WorktreeState, or nil.
func (p *RepoPanel) SelectedWorktree() *tuimsg.WorktreeState {
	if len(p.selectIdx) == 0 {
		return nil
	}
	if p.selected < 0 || p.selected >= len(p.selectIdx) {
		return nil
	}
	wt := p.items[p.selectIdx[p.selected]].wt
	return &wt
}

func (p *RepoPanel) SelectedIndex() int { return p.selected }

func (p RepoPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69")
	}

	innerW := p.width - 4
	if innerW < 10 {
		innerW = 10
	}

	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Render("Repos & Sessions"))
	rows = append(rows, "")

	if len(p.items) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
			Render("Scanning ~/.claude/projects/ …"))
	} else {
		selItemIdx := -1
		if len(p.selectIdx) > 0 && p.selected < len(p.selectIdx) {
			selItemIdx = p.selectIdx[p.selected]
		}
		for i, item := range p.items {
			if item.isHeader {
				rows = append(rows, renderRepoHeader(item.label, innerW))
			} else {
				rows = append(rows, renderWorktreeRow(item.wt, i == selItemIdx, p.focused, innerW))
			}
		}
	}

	content := strings.Join(rows, "\n")
	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	return style.Render(content)
}

// buildLeftItems groups worktrees by RepoPath and builds a flat render list.
func buildLeftItems(worktrees []tuimsg.WorktreeState) (items []leftItem, selectIdx []int) {
	type repoGroup struct {
		name string
		key  string
		wts  []tuimsg.WorktreeState
	}

	seen := make(map[string]*repoGroup)
	var order []string

	for _, wt := range worktrees {
		key := wt.RepoPath
		if key == "" {
			key = wt.WorktreePath
		}
		name := wt.RepoName
		if name == "" {
			name = filepath.Base(key)
		}
		if _, ok := seen[key]; !ok {
			seen[key] = &repoGroup{name: name, key: key}
			order = append(order, key)
		}
		seen[key].wts = append(seen[key].wts, wt)
	}

	for _, key := range order {
		g := seen[key]
		items = append(items, leftItem{isHeader: true, label: g.name})
		for _, wt := range g.wts {
			selectIdx = append(selectIdx, len(items))
			items = append(items, leftItem{wt: wt})
		}
	}
	return items, selectIdx
}

// renderRepoHeader renders a repo group header row.
func renderRepoHeader(name string, width int) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("75")). // bright blue
		Render(name)
}

// statusIndicator derives a visual status indicator from WorktreeState.
// Priority: active session > just finished session > agent status > merge conflict > idle.
func statusIndicator(wt tuimsg.WorktreeState) (indicator string, color string) {
	// Check session states
	var hasWaiting, hasActive, hasJustFinished bool
	for _, s := range wt.Sessions {
		if s.WaitingForPermission {
			hasWaiting = true
		} else if s.IsActive {
			hasActive = true
		} else if time.Since(s.LastActivity) < 30*time.Minute {
			hasJustFinished = true
		}
	}
	if hasWaiting {
		return "⚠", "5" // magenta — needs user input NOW
	}
	if hasActive {
		return "●", "2" // green — session running
	}
	if hasJustFinished {
		return "✓", "3" // yellow — just finished, needs attention
	}
	// Agent workflow status
	if wt.AgentStatus == "running" {
		return "⚙", "3" // yellow — agent in progress
	}
	if wt.AgentStatus == "failed" {
		return "✗", "1" // red — agent failed
	}
	if wt.AgentStatus == "completed" {
		return "✓", "6" // cyan — agent done
	}
	// Merge conflicts
	for _, cf := range wt.ChangedFiles {
		if cf.StagedStatus == 'U' || cf.UnstagedStatus == 'U' {
			return "!", "1" // red — conflicts
		}
	}
	return "○", "240" // dim — idle
}

// styledIndicator returns the status indicator with appropriate color applied.
func styledIndicator(indicator, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(indicator)
}

// renderWorktreeRow renders a single worktree row. Plain text layout first,
// then apply styling. No ANSI-styled strings inside fmt.Sprintf.
func renderWorktreeRow(wt tuimsg.WorktreeState, selected, focused bool, width int) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	branch := truncate(wt.Branch, 20)
	stats := buildStats(wt)
	indicator, color := statusIndicator(wt)

	// Build as plain text first for correct width calculation.
	plainLine := fmt.Sprintf("%s %s%-20s  %s", indicator, cursor, branch, stats)

	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	if selected {
		rowStyle = rowStyle.Background(lipgloss.Color("17")).Bold(true).Width(width)
	}
	return rowStyle.Render(plainLine)
}

// buildStats produces a plain-text stats string for a worktree row.
// No ANSI codes — safe to use inside fmt.Sprintf.
func buildStats(wt tuimsg.WorktreeState) string {
	var parts []string
	if wt.Behind == -1 {
		parts = append(parts, "no upstream")
	} else {
		parts = append(parts, fmt.Sprintf("+%d/-%d", wt.Ahead, wt.Behind))
	}
	if wt.Staged > 0 {
		parts = append(parts, fmt.Sprintf("S:%d", wt.Staged))
	}
	if wt.Unstaged > 0 {
		parts = append(parts, fmt.Sprintf("U:%d", wt.Unstaged))
	}
	if wt.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("?:%d", wt.Untracked))
	}
	active := countActiveSessions(wt)
	if active > 0 {
		parts = append(parts, fmt.Sprintf("◆%d", active))
	}
	return strings.Join(parts, " ")
}

// countActiveSessions returns the number of active Claude sessions for a worktree.
func countActiveSessions(wt tuimsg.WorktreeState) int {
	n := 0
	for _, s := range wt.Sessions {
		if s.IsActive {
			n++
		}
	}
	return n
}

// truncate shortens s to max length, adding "…" if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
