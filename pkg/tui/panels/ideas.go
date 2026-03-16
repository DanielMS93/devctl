package panels

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/pkg/tui/tuimsg"
)

// IdeaPanel renders the idea pipeline DAG.
type IdeaPanel struct {
	width   int
	height  int
	focused bool
	graph   tuimsg.IdeaGraphSnapshot
	scrollY int
	cursor  int // selected idea index
}

// NewIdeaPanel creates an empty IdeaPanel.
func NewIdeaPanel() IdeaPanel {
	return IdeaPanel{}
}

// SetSize updates the panel dimensions.
func (p *IdeaPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocused updates the focus state.
func (p *IdeaPanel) SetFocused(focused bool) {
	p.focused = focused
}

// SetGraph updates the idea graph data.
func (p *IdeaPanel) SetGraph(g tuimsg.IdeaGraphSnapshot) {
	p.graph = g
	if p.cursor >= len(g.Ideas) {
		p.cursor = len(g.Ideas) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

// MoveUp moves the cursor up.
func (p *IdeaPanel) MoveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

// MoveDown moves the cursor down.
func (p *IdeaPanel) MoveDown() {
	if p.cursor < len(p.graph.Ideas)-1 {
		p.cursor++
	}
}

// SelectedIdea returns the currently selected idea, or nil.
func (p *IdeaPanel) SelectedIdea() *tuimsg.ResolvedIdea {
	if p.cursor >= 0 && p.cursor < len(p.graph.Ideas) {
		return &p.graph.Ideas[p.cursor]
	}
	return nil
}

// ScrollUp moves the viewport up.
func (p *IdeaPanel) ScrollUp() {
	if p.scrollY > 0 {
		p.scrollY--
	}
}

// ScrollDown moves the viewport down.
func (p *IdeaPanel) ScrollDown() {
	p.scrollY++
}

// View renders the idea panel.
func (p IdeaPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69")
	}

	innerW := p.width - 4
	if innerW < 10 {
		innerW = 10
	}
	innerH := p.height - 4
	if innerH < 3 {
		innerH = 3
	}

	content := p.renderIdeas(innerW, innerH)

	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
	return style.Render(content)
}

func (p IdeaPanel) renderIdeas(innerW, innerH int) string {
	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var rows []string

	rows = append(rows, bold.Render("Idea Pipeline"))
	rows = append(rows, dim.Render(strings.Repeat("~", innerW)))

	if p.graph.HasCycle {
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		rows = append(rows, warn.Render("Warning: cycle detected in idea dependencies"))
		rows = append(rows, "")
	}

	if len(p.graph.Ideas) == 0 {
		rows = append(rows, "")
		rows = append(rows, dim.Render("No ideas yet. Create one with:"))
		rows = append(rows, "  devctl idea add <prompt>")
		rows = append(rows, "")
		rows = append(rows, dim.Render("j/k=navigate  i=close  tab=switch panel"))
		return p.applyScroll(rows, innerH)
	}

	// Render ideas as a list with state badges.
	for idx, idea := range p.graph.Ideas {
		selected := idx == p.cursor
		line := p.renderIdeaLine(idea, innerW, selected)
		rows = append(rows, line)
	}

	// Help bar.
	rows = append(rows, "")
	rows = append(rows, dim.Render("j/k=navigate  enter=details  r=launch  c=cancel  m=incorporate  i=close"))

	return p.applyScroll(rows, innerH)
}

func (p IdeaPanel) renderIdeaLine(idea tuimsg.ResolvedIdea, width int, selected bool) string {
	// State badge with color.
	var badge string
	switch {
	case idea.Incorporated:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[DONE]")
	case idea.State == "completed":
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true).Render("[COMPLETED]")
	case idea.State == "running":
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("[RUNNING]")
	case idea.State == "failed":
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("[FAILED]")
	case idea.IsReady:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render("[READY]")
	case idea.IsBlocked:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("[BLOCKED]")
	default:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[QUEUED]")
	}

	// Kind indicator.
	kindStr := ""
	if idea.Kind == "sequential" {
		kindStr = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render(" seq")
	}

	// Truncate prompt.
	maxPrompt := width - 20
	if maxPrompt < 10 {
		maxPrompt = 10
	}
	prompt := idea.Prompt
	if len(prompt) > maxPrompt {
		prompt = prompt[:maxPrompt-1] + "~"
	}

	id := idea.ID
	if len(id) > 8 {
		id = id[:8]
	}

	line := fmt.Sprintf("%s %s %s%s", id, badge, prompt, kindStr)

	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Bold(true).
			Width(width).
			Render(line)
	}
	return line
}

func (p IdeaPanel) applyScroll(rows []string, innerH int) string {
	total := len(rows)

	maxScroll := total - innerH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scrollY := p.scrollY
	if scrollY > maxScroll {
		scrollY = maxScroll
	}

	end := scrollY + innerH
	if end > total {
		end = total
	}

	visible := rows[scrollY:end]

	if total > innerH && scrollY < maxScroll {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		if len(visible) > 0 {
			visible[len(visible)-1] = dim.Render(fmt.Sprintf("... scroll (%d/%d)", scrollY+innerH, total))
		}
	}

	return strings.Join(visible, "\n")
}
