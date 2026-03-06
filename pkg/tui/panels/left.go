package panels

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
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
	label := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("75")). // bright blue
		Render(name)
	return label
}

// statusIndicator derives a visual status indicator from existing WorktreeState fields.
// Returns "●" for running (active Claude session), "!" for blocked (merge conflicts),
// or "○" for idle.
func statusIndicator(wt tuimsg.WorktreeState) string {
	// Running: has at least one active Claude session
	for _, s := range wt.Sessions {
		if s.IsActive {
			return "●"
		}
	}
	// Blocked: has merge conflicts (UU status in changed files)
	for _, cf := range wt.ChangedFiles {
		if cf.StagedStatus == 'U' || cf.UnstagedStatus == 'U' {
			return "!"
		}
	}
	return "○"
}

// styledIndicator returns the status indicator with appropriate color applied.
func styledIndicator(indicator string) string {
	switch indicator {
	case "●":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(indicator) // green
	case "!":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(indicator) // red
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(indicator) // dim
	}
}

// renderWorktreeRow renders a single worktree row as plain text layout, then
// applies selection highlight. Critically: no ANSI-styled strings inside
// fmt.Sprintf — that breaks width calculations and causes wrapping.
func renderWorktreeRow(wt tuimsg.WorktreeState, selected, focused bool, width int) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	branch := truncate(wt.Branch, 20)
	stats := buildStats(wt)
	indicator := statusIndicator(wt)

	if selected {
		// For selected rows, use the styled indicator within the highlighted line.
		styledLine := fmt.Sprintf("%s %s%-20s  %s", styledIndicator(indicator), cursor, branch, stats)
		return lipgloss.NewStyle().
			Background(lipgloss.Color("17")).
			Bold(true).
			Width(width).
			Render(styledLine)
	}
	return fmt.Sprintf("%s %s%-20s  %s", styledIndicator(indicator), cursor, branch, stats)
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
