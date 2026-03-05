package panels

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// DetailPanel shows the changed files list for the selected worktree.
// When a file is selected and Enter is pressed, root.go opens the ViewerModel.
type DetailPanel struct {
	width    int
	height   int
	focused  bool
	worktree *tuimsg.WorktreeState
	selected int // selected file index
}

func NewDetailPanel() DetailPanel { return DetailPanel{} }

func (p *DetailPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DetailPanel) SetFocused(focused bool) {
	p.focused = focused
}

// SetWorktree updates which worktree's files to show.
func (p *DetailPanel) SetWorktree(wt *tuimsg.WorktreeState) {
	p.worktree = wt
	p.selected = 0 // reset selection on worktree change
}

// MoveUp moves file selection cursor up.
func (p *DetailPanel) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

// MoveDown moves file selection cursor down.
func (p *DetailPanel) MoveDown() {
	if p.worktree != nil && p.selected < len(p.worktree.ChangedFiles)-1 {
		p.selected++
	}
}

// SelectedFile returns the path of the currently selected changed file, or "".
func (p *DetailPanel) SelectedFile() string {
	if p.worktree == nil || len(p.worktree.ChangedFiles) == 0 {
		return ""
	}
	if p.selected < 0 || p.selected >= len(p.worktree.ChangedFiles) {
		return ""
	}
	return p.worktree.ChangedFiles[p.selected].Path
}

func (p DetailPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69")
	}

	innerW := p.width - 4
	if innerW < 1 {
		innerW = 1
	}

	var rows []string
	if p.worktree == nil {
		rows = append(rows, lipgloss.NewStyle().Bold(true).Render("Changed Files"))
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
			Render("(select a worktree to see changed files)"))
	} else {
		title := fmt.Sprintf("Changed Files — %s", p.worktree.Branch)
		rows = append(rows, lipgloss.NewStyle().Bold(true).Render(title))
		rows = append(rows, "")

		if len(p.worktree.ChangedFiles) == 0 {
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("2")).
				Render("(no changed files — working tree clean)"))
		} else {
			for i, cf := range p.worktree.ChangedFiles {
				cursor := "  "
				if i == p.selected {
					cursor = "> "
				}
				statusLabel := fmt.Sprintf("[%c%c]", cf.StagedStatus, cf.UnstagedStatus)
				line := fmt.Sprintf("%s%s %s", cursor, statusLabel, cf.Path)
				if i == p.selected && p.focused {
					line = lipgloss.NewStyle().Background(lipgloss.Color("17")).Bold(true).Width(innerW).Render(line)
				}
				rows = append(rows, line)
			}
			rows = append(rows, "")
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
				Render("Enter=open viewer"))
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
