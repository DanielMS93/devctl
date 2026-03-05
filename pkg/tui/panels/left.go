package panels

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// RepoPanel is the left pane showing the repository/worktree tree.
// Phase 1: renders a placeholder. Phase 2+ populates with real git state.
type RepoPanel struct {
	width     int
	height    int
	lastEvent tuimsg.StateEvent
	focused   bool
}

func NewRepoPanel() RepoPanel { return RepoPanel{} }

func (p *RepoPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *RepoPanel) SetState(e tuimsg.StateEvent) {
	p.lastEvent = e
}

func (p *RepoPanel) SetFocused(focused bool) {
	p.focused = focused
}

func (p RepoPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69") // blue when active
	}
	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	content := "Repos\n\n(no repos tracked yet)"
	if !p.lastEvent.Snapshot.UpdatedAt.IsZero() {
		content = fmt.Sprintf("Repos\n\n(no repos tracked yet)\n\nLast update: %s",
			p.lastEvent.Snapshot.UpdatedAt.Format("15:04:05"))
	}
	return style.Render(content)
}
