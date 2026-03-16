package panels

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/pkg/tui/tuimsg"
)

// PatchStatusUpdater is the interface for updating patch status in the DB.
// The concrete agent.PatchStore satisfies this interface.
type PatchStatusUpdater interface {
	UpdateStatus(ctx context.Context, id, status string) error
}

// PatchStatusMsg is sent after an approve/reject DB operation completes.
type PatchStatusMsg struct {
	Title  string
	Status string
	Err    error
}

// PatchPanel displays agent-generated patches with navigation and approve/reject.
type PatchPanel struct {
	width    int
	height   int
	focused  bool
	patches  []tuimsg.AgentPatch
	cursor   int
	showDiff bool
	vp       viewport.Model
	updater  PatchStatusUpdater
}

// NewPatchPanel creates a PatchPanel with the given status updater for direct DB operations.
func NewPatchPanel(updater PatchStatusUpdater) PatchPanel {
	return PatchPanel{updater: updater}
}

// SetPatches updates the patch list and clamps the cursor.
func (p *PatchPanel) SetPatches(ps tuimsg.PatchSnapshot) {
	p.patches = ps.Patches
	if p.cursor >= len(p.patches) {
		p.cursor = len(p.patches) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

// SetSize updates the panel dimensions.
func (p *PatchPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SetFocused updates the focus state.
func (p *PatchPanel) SetFocused(b bool) {
	p.focused = b
}

// MoveUp moves the cursor up one position.
func (p *PatchPanel) MoveUp() {
	if p.showDiff {
		p.vp.ScrollUp(1)
		return
	}
	if p.cursor > 0 {
		p.cursor--
	}
}

// MoveDown moves the cursor down one position.
func (p *PatchPanel) MoveDown() {
	if p.showDiff {
		p.vp.ScrollDown(1)
		return
	}
	if p.cursor < len(p.patches)-1 {
		p.cursor++
	}
}

// ToggleDiff toggles between list view and diff view of the selected patch.
func (p *PatchPanel) ToggleDiff() {
	if len(p.patches) == 0 {
		return
	}
	p.showDiff = !p.showDiff
	if p.showDiff {
		innerW := p.width - 4
		if innerW < 10 {
			innerW = 10
		}
		innerH := p.height - 6
		if innerH < 3 {
			innerH = 3
		}
		p.vp = viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(innerH))
		p.vp.SetContent(p.patches[p.cursor].PatchData)
	}
}

// CloseDiff exits diff view back to list. Returns true if diff was open.
func (p *PatchPanel) CloseDiff() bool {
	if p.showDiff {
		p.showDiff = false
		return true
	}
	return false
}

// ShowingDiff returns whether the panel is in diff view mode.
func (p *PatchPanel) ShowingDiff() bool {
	return p.showDiff
}

// SelectedPatch returns the patch at the cursor, or nil if empty.
func (p *PatchPanel) SelectedPatch() *tuimsg.AgentPatch {
	if len(p.patches) == 0 || p.cursor >= len(p.patches) {
		return nil
	}
	return &p.patches[p.cursor]
}

// ApprovePatch approves the selected draft patch via a direct DB update.
func (p *PatchPanel) ApprovePatch(ctx context.Context) tea.Cmd {
	patch := p.SelectedPatch()
	if patch == nil || patch.Status != "draft" || p.updater == nil {
		return nil
	}
	id := patch.ID
	title := patch.Title
	// Update local state immediately for responsive UI.
	p.patches[p.cursor].Status = "approved"
	return func() tea.Msg {
		err := p.updater.UpdateStatus(ctx, id, "approved")
		return PatchStatusMsg{Title: title, Status: "approved", Err: err}
	}
}

// RejectPatch rejects the selected patch via a direct DB update.
func (p *PatchPanel) RejectPatch(ctx context.Context) tea.Cmd {
	patch := p.SelectedPatch()
	if patch == nil || p.updater == nil {
		return nil
	}
	if patch.Status != "draft" && patch.Status != "approved" {
		return nil
	}
	id := patch.ID
	title := patch.Title
	// Update local state immediately for responsive UI.
	p.patches[p.cursor].Status = "rejected"
	return func() tea.Msg {
		err := p.updater.UpdateStatus(ctx, id, "rejected")
		return PatchStatusMsg{Title: title, Status: "rejected", Err: err}
	}
}

// View renders the patch panel.
func (p PatchPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69")
	}

	innerW := p.width - 4
	if innerW < 10 {
		innerW = 10
	}

	var content string
	if p.showDiff {
		content = p.renderDiffView(innerW)
	} else {
		content = p.renderListView(innerW)
	}

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Render(content)
}

// renderListView renders the patch list with cursor selection.
func (p PatchPanel) renderListView(innerW int) string {
	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var rows []string

	// Header with draft count.
	draftCount := 0
	for _, patch := range p.patches {
		if patch.Status == "draft" {
			draftCount++
		}
	}
	header := "Agent Patches"
	if draftCount > 0 {
		header += fmt.Sprintf(" (%d draft)", draftCount)
	}
	rows = append(rows, bold.Render(header))
	rows = append(rows, dim.Render(strings.Repeat("~", innerW)))

	if len(p.patches) == 0 {
		rows = append(rows, "")
		rows = append(rows, dim.Render("No agent patches yet."))
		rows = append(rows, dim.Render("Patches appear when agent workflows generate diffs."))
		rows = append(rows, "")
		rows = append(rows, dim.Render("p=close  tab=switch panel"))
		return strings.Join(rows, "\n")
	}

	// Patch rows.
	for i, patch := range p.patches {
		badge := renderStatusBadge(patch.Status)
		title := patch.Title
		maxTitle := innerW - 30
		if maxTitle < 10 {
			maxTitle = 10
		}
		if len(title) > maxTitle {
			title = title[:maxTitle-1] + "~"
		}

		age := formatAge(patch.CreatedAt)
		branch := patch.Branch
		if len(branch) > 16 {
			branch = branch[:15] + "~"
		}

		line := fmt.Sprintf("%s %s  %s  %s", badge, title, dim.Render(branch), dim.Render(age))

		if i == p.cursor {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Width(innerW).
				Render(line)
		}
		rows = append(rows, line)
	}

	rows = append(rows, "")
	rows = append(rows, dim.Render("Enter=view diff  a=approve  x=reject  p=close"))

	return strings.Join(rows, "\n")
}

// renderDiffView renders the scrollable diff viewport for the selected patch.
func (p PatchPanel) renderDiffView(innerW int) string {
	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	patch := p.patches[p.cursor]
	title := fmt.Sprintf("%s  %s", patch.Title, renderStatusBadge(patch.Status))

	header := bold.Render(title)
	helpLine := dim.Render("Esc=back  a=approve  x=reject  j/k=scroll")

	return strings.Join([]string{header, p.vp.View(), helpLine}, "\n")
}

// renderStatusBadge returns a colored status label.
func renderStatusBadge(status string) string {
	switch status {
	case "draft":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("[draft]")
	case "approved":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("[approved]")
	case "applied":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("[applied]")
	case "rejected":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[rejected]")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[" + status + "]")
	}
}

// formatAge is defined in right.go and shared across panels.
