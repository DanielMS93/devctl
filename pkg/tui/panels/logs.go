package panels

import (
	"charm.land/lipgloss/v2"
)

// LogBar is the bottom status bar showing key hints and transient status messages.
type LogBar struct {
	width  int
	status string // transient message; empty = show default hints
}

func NewLogBar() LogBar { return LogBar{} }

func (b *LogBar) SetWidth(width int) { b.width = width }

// SetStatus sets a transient status message shown in place of the default hint line.
func (b *LogBar) SetStatus(msg string) { b.status = msg }

func (b LogBar) View() string {
	text := " q=quit  tab=panels  ↑↓=navigate  enter=expand  d=diff  f=file  r=resume  t=tasks"
	if b.status != "" {
		text = "  " + b.status
	}

	return lipgloss.NewStyle().
		Width(b.width).
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("235")).
		Render(text)
}
