package panels

import (
	"charm.land/lipgloss/v2"
)

// LogBar is the bottom status bar.
// Phase 1: renders key binding hints and a static status line.
type LogBar struct {
	width int
}

func NewLogBar() LogBar { return LogBar{} }

func (b *LogBar) SetWidth(width int) {
	b.width = width
}

func (b LogBar) View() string {
	style := lipgloss.NewStyle().
		Width(b.width).
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("235"))

	return style.Render(" q: quit  tab: cycle panels  devctl dashboard")
}
