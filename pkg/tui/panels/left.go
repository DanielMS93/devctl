package panels

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// RepoPanel is the left pane showing the worktree list with git state.
type RepoPanel struct {
	width     int
	height    int
	worktrees []tuimsg.WorktreeState
	selected  int // index into worktrees
	focused   bool
}

func NewRepoPanel() RepoPanel { return RepoPanel{} }

func (p *RepoPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *RepoPanel) SetState(e tuimsg.StateEvent) {
	p.worktrees = e.Snapshot.Worktrees
	// Clamp selection to valid range after state update
	if p.selected >= len(p.worktrees) && len(p.worktrees) > 0 {
		p.selected = len(p.worktrees) - 1
	}
}

func (p *RepoPanel) SetFocused(focused bool) {
	p.focused = focused
}

// MoveUp moves the selection cursor up by one row. No-op at top.
func (p *RepoPanel) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

// MoveDown moves the selection cursor down by one row. No-op at bottom.
func (p *RepoPanel) MoveDown() {
	if p.selected < len(p.worktrees)-1 {
		p.selected++
	}
}

// SelectedWorktree returns the currently selected WorktreeState, or nil if empty.
func (p *RepoPanel) SelectedWorktree() *tuimsg.WorktreeState {
	if len(p.worktrees) == 0 || p.selected < 0 || p.selected >= len(p.worktrees) {
		return nil
	}
	wt := p.worktrees[p.selected]
	return &wt
}

// SelectedIndex returns the current selection index.
func (p *RepoPanel) SelectedIndex() int { return p.selected }

func (p RepoPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69")
	}

	// Inner dimensions account for border (2 chars each side)
	innerW := p.width - 4
	if innerW < 1 {
		innerW = 1
	}

	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Render("Worktrees"))
	rows = append(rows, "")

	if len(p.worktrees) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
			"(no worktrees tracked)\nUse: devctl worktree create",
		))
	} else {
		for i, wt := range p.worktrees {
			row := renderWorktreeRow(wt, i == p.selected, innerW)
			rows = append(rows, row)
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

func renderWorktreeRow(wt tuimsg.WorktreeState, selected bool, width int) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	// Ahead/behind badge
	var badge string
	if wt.Behind == -1 {
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("no upstream")
	} else {
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(
			fmt.Sprintf("+%d/-%d", wt.Ahead, wt.Behind),
		)
	}

	// Status counts (only show non-zero)
	var counts []string
	if wt.Staged > 0 {
		counts = append(counts, lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(
			fmt.Sprintf("S:%d", wt.Staged),
		))
	}
	if wt.Unstaged > 0 {
		counts = append(counts, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(
			fmt.Sprintf("U:%d", wt.Unstaged),
		))
	}
	if wt.Untracked > 0 {
		counts = append(counts, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
			fmt.Sprintf("?:%d", wt.Untracked),
		))
	}

	statusStr := strings.Join(counts, " ")
	line := fmt.Sprintf("%s%-20s  %s  %s", cursor, truncate(wt.Branch, 20), badge, statusStr)

	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("17")). // dark blue
			Bold(true).
			Width(width).
			Render(line)
	}
	return line
}

// truncate shortens s to max length, adding "…" if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
