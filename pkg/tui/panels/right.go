package panels

import (
	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// DetailPanel is the right pane showing details for the selected item.
// Phase 1: renders a placeholder.
type DetailPanel struct {
	width   int
	height  int
	focused bool
}

func NewDetailPanel() DetailPanel { return DetailPanel{} }

func (p *DetailPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DetailPanel) SetFocused(focused bool) {
	p.focused = focused
}

// SetWorktree updates which worktree's details to show.
// Phase 2 Plan 06 replaces this stub with real changed-files rendering.
func (p *DetailPanel) SetWorktree(wt *tuimsg.WorktreeState) {
	// stub — implemented in 02-06
}

func (p DetailPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69") // blue when active
	}
	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	return style.Render("Detail\n\n(select a repo or worktree to see details)")
}
