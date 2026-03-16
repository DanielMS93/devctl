package panels

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/pkg/tui/tuimsg"
)

// boxWidth is the fixed width of each task box in the graph.
const boxWidth = 24

// arrowGap is the width of the arrow connector between layers.
const arrowGap = 4

// layerWidth is the total width per layer (box + arrow gap).
const layerWidth = boxWidth + arrowGap

// TaskGraphPanel renders the resolved task DAG as layered text.
type TaskGraphPanel struct {
	width   int
	height  int
	focused bool
	graph   tuimsg.TaskGraphSnapshot
	scrollY int // vertical scroll offset for overflow
}

// NewTaskGraphPanel creates an empty TaskGraphPanel.
func NewTaskGraphPanel() TaskGraphPanel {
	return TaskGraphPanel{}
}

// SetSize updates the panel dimensions.
func (p *TaskGraphPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocused updates the focus state.
func (p *TaskGraphPanel) SetFocused(focused bool) {
	p.focused = focused
}

// SetGraph updates the task graph data.
func (p *TaskGraphPanel) SetGraph(g tuimsg.TaskGraphSnapshot) {
	p.graph = g
	// Clamp scroll if content shrunk.
	p.clampScroll()
}

// ScrollUp moves the viewport up by one line.
func (p *TaskGraphPanel) ScrollUp() {
	if p.scrollY > 0 {
		p.scrollY--
	}
}

// ScrollDown moves the viewport down by one line.
func (p *TaskGraphPanel) ScrollDown() {
	p.scrollY++
	p.clampScroll()
}

func (p *TaskGraphPanel) clampScroll() {
	// We don't know exact rendered height without rendering, so just cap at a reasonable max.
	// The View() method handles the actual truncation.
	if p.scrollY < 0 {
		p.scrollY = 0
	}
}

// View renders the task graph panel.
func (p TaskGraphPanel) View() string {
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

	content := p.renderGraph(innerW, innerH)

	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
	return style.Render(content)
}

// renderGraph builds the layered graph text.
func (p TaskGraphPanel) renderGraph(innerW, innerH int) string {
	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var rows []string

	rows = append(rows, bold.Render("Task Graph"))
	rows = append(rows, dim.Render(strings.Repeat("~", innerW)))

	if p.graph.HasCycle {
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		rows = append(rows, warn.Render("Warning: cycle detected in task dependencies"))
		rows = append(rows, "")
	}

	if len(p.graph.Tasks) == 0 {
		rows = append(rows, "")
		rows = append(rows, dim.Render("No tasks yet. Create one with:"))
		rows = append(rows, "  devctl tasks create <description>")
		rows = append(rows, "")
		rows = append(rows, dim.Render("j/k=scroll  t=close  tab=switch panel"))
		return p.applyScroll(rows, innerH)
	}

	// Group tasks by layer.
	maxLayer := 0
	layerMap := make(map[int][]tuimsg.ResolvedTask)
	for _, t := range p.graph.Tasks {
		layerMap[t.Layer] = append(layerMap[t.Layer], t)
		if t.Layer > maxLayer {
			maxLayer = t.Layer
		}
	}

	// Determine how many layers fit in the available width.
	maxVisibleLayers := innerW / layerWidth
	if maxVisibleLayers < 1 {
		maxVisibleLayers = 1
	}
	hiddenLayers := 0
	displayLayers := maxLayer + 1
	if displayLayers > maxVisibleLayers {
		hiddenLayers = displayLayers - maxVisibleLayers
		displayLayers = maxVisibleLayers
	}

	// Find max tasks in any visible layer (determines row count for the grid).
	maxTasksInLayer := 0
	for layer := 0; layer < displayLayers; layer++ {
		if len(layerMap[layer]) > maxTasksInLayer {
			maxTasksInLayer = len(layerMap[layer])
		}
	}

	// Render row by row. Each "row" is one task slot across all layers.
	for row := 0; row < maxTasksInLayer; row++ {
		var lineParts []string
		for layer := 0; layer < displayLayers; layer++ {
			tasks := layerMap[layer]
			var box string
			if row < len(tasks) {
				box = renderTaskBox(tasks[row])
			} else {
				box = strings.Repeat(" ", boxWidth)
			}
			if layer < displayLayers-1 {
				// Add arrow connector if this row has a task.
				arrow := "    "
				if row < len(tasks) {
					arrow = " -> "
				}
				lineParts = append(lineParts, box+arrow)
			} else {
				lineParts = append(lineParts, box)
			}
		}
		rows = append(rows, strings.Join(lineParts, ""))
		rows = append(rows, "") // blank line between task rows
	}

	if hiddenLayers > 0 {
		rows = append(rows, dim.Render(fmt.Sprintf("... +%d more layers", hiddenLayers)))
	}

	// Help bar.
	rows = append(rows, "")
	rows = append(rows, dim.Render("j/k=scroll  t=close  tab=switch panel"))

	return p.applyScroll(rows, innerH)
}

// applyScroll applies vertical scrolling to the rendered rows.
func (p TaskGraphPanel) applyScroll(rows []string, innerH int) string {
	total := len(rows)

	// Clamp scrollY to valid range.
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

	// Show scroll indicator if content overflows.
	if total > innerH && scrollY < maxScroll {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		if len(visible) > 0 {
			visible[len(visible)-1] = dim.Render(fmt.Sprintf("... scroll (%d/%d)", scrollY+innerH, total))
		}
	}

	return strings.Join(visible, "\n")
}

// renderTaskBox renders a single task as a fixed-width box with colored state badge.
func renderTaskBox(t tuimsg.ResolvedTask) string {
	// Truncate description.
	desc := t.Description
	maxDesc := boxWidth - 4
	if maxDesc < 8 {
		maxDesc = 8
	}
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-1] + "~"
	}

	// State badge with color.
	var badge string
	borderColorStr := "240"
	switch {
	case t.State == "completed":
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[DONE]")
		borderColorStr = "240"
	case t.IsReady:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render("[READY]")
		borderColorStr = "2"
	case t.State == "running":
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("[RUNNING]")
		borderColorStr = "3"
	case t.IsBlocked:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("[BLOCKED]")
		borderColorStr = "1"
	default:
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[QUEUED]")
		borderColorStr = "240"
	}

	content := desc + "\n" + badge

	return lipgloss.NewStyle().
		Width(boxWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColorStr)).
		Render(content)
}
