package panels

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// DetailPanel shows Claude sessions and changed files for the selected worktree.
// Sessions are always shown first (no mode toggle). Arrow keys navigate a unified
// list: sessions first, then files. Enter on a session shows the resume command.
// By default only active sessions are shown; pressing 'a' toggles showing all.
type DetailPanel struct {
	width    int
	height   int
	focused  bool
	worktree *tuimsg.WorktreeState

	// cursor is a unified index over (visible sessions + files).
	// 0..numVisibleSessions-1  → session items
	// numVisibleSessions..end  → file items
	cursor int

	// showAllSessions toggles between showing only active sessions vs all.
	showAllSessions bool
}

func NewDetailPanel() DetailPanel { return DetailPanel{} }

func (p *DetailPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DetailPanel) SetFocused(focused bool) { p.focused = focused }

// SetWorktree updates the displayed worktree. Resets cursor only when
// switching to a different worktree, so poll refreshes don't disrupt navigation.
func (p *DetailPanel) SetWorktree(wt *tuimsg.WorktreeState) {
	if wt == nil || p.worktree == nil || wt.WorktreePath != p.worktree.WorktreePath {
		p.cursor = 0
	}
	p.worktree = wt
	// Clamp cursor if items shrunk.
	if total := p.totalItems(); p.cursor >= total && total > 0 {
		p.cursor = total - 1
	}
}

// visibleSessions returns the sessions that should be displayed based on the
// showAllSessions toggle.
func (p *DetailPanel) visibleSessions() []tuimsg.ClaudeSession {
	if p.worktree == nil {
		return nil
	}
	if p.showAllSessions {
		return p.worktree.Sessions
	}
	var active []tuimsg.ClaudeSession
	for _, s := range p.worktree.Sessions {
		if s.IsActive {
			active = append(active, s)
		}
	}
	return active
}

func (p *DetailPanel) numSessions() int {
	return len(p.visibleSessions())
}

func (p *DetailPanel) numFiles() int {
	if p.worktree == nil {
		return 0
	}
	return len(p.worktree.ChangedFiles)
}

func (p *DetailPanel) totalItems() int { return p.numSessions() + p.numFiles() }

// ToggleAllSessions switches between showing only active sessions and all sessions.
func (p *DetailPanel) ToggleAllSessions() {
	p.showAllSessions = !p.showAllSessions
	// Clamp cursor after toggling since visible item count changed.
	if total := p.totalItems(); p.cursor >= total && total > 0 {
		p.cursor = total - 1
	} else if total == 0 {
		p.cursor = 0
	}
}

func (p *DetailPanel) MoveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *DetailPanel) MoveDown() {
	if p.cursor < p.totalItems()-1 {
		p.cursor++
	}
}

// SelectedSession returns the session under the cursor, or nil.
func (p *DetailPanel) SelectedSession() *tuimsg.ClaudeSession {
	visible := p.visibleSessions()
	if p.cursor >= len(visible) {
		return nil
	}
	s := visible[p.cursor]
	return &s
}

// SelectedFile returns the changed-file path under the cursor, or "".
func (p *DetailPanel) SelectedFile() string {
	ns := p.numSessions()
	if p.worktree == nil || p.cursor < ns {
		return ""
	}
	fi := p.cursor - ns
	if fi >= len(p.worktree.ChangedFiles) {
		return ""
	}
	return p.worktree.ChangedFiles[fi].Path
}

func (p DetailPanel) View() string {
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("69")
	}

	innerW := p.width - 4
	if innerW < 10 {
		innerW = 10
	}

	content := p.viewMain(innerW)

	style := lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
	return style.Render(content)
}

func (p DetailPanel) viewMain(innerW int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	bold := lipgloss.NewStyle().Bold(true)

	var rows []string

	if p.worktree == nil {
		rows = append(rows, bold.Render("Select a repo"))
		rows = append(rows, "")
		rows = append(rows, dim.Render("Use ↑/↓ in the left panel to pick a repo."))
		return strings.Join(rows, "\n")
	}

	repoLabel := p.worktree.RepoName
	if repoLabel == "" {
		repoLabel = p.worktree.Branch
	}
	rows = append(rows, bold.Render(fmt.Sprintf("%s  —  %s", repoLabel, p.worktree.Branch)))
	rows = append(rows, "")

	ns := p.numSessions()
	nf := p.numFiles()

	// ── Sessions section ────────────────────────────────────────────────
	totalSessions := len(p.worktree.Sessions)
	visible := p.visibleSessions()
	if totalSessions > 0 {
		active := countActiveSessions(*p.worktree)
		inactive := totalSessions - active

		var label string
		if p.showAllSessions {
			label = fmt.Sprintf("Sessions  %d active", active)
			if inactive > 0 {
				label += fmt.Sprintf(", %d inactive", inactive)
			}
			label += "  (a=hide inactive)"
		} else {
			label = fmt.Sprintf("Sessions  %d active", active)
			if inactive > 0 {
				label += dim.Render(fmt.Sprintf("  +%d inactive (a=show)", inactive))
			}
		}
		rows = append(rows, bold.Render(label))
		rows = append(rows, dim.Render(strings.Repeat("─", innerW)))

		for i, s := range visible {
			selected := p.focused && p.cursor == i
			rows = append(rows, renderSessionRow(s, selected, innerW))
		}
		rows = append(rows, "") // one blank line before next section
	}

	// ── Changed Files section ────────────────────────────────────────────
	rows = append(rows, bold.Render("Changed Files"))
	rows = append(rows, dim.Render(strings.Repeat("─", innerW)))

	if nf == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("2")).
			Render("(working tree clean)"))
	} else {
		for i, cf := range p.worktree.ChangedFiles {
			fi := ns + i
			selected := p.focused && p.cursor == fi
			cursor := "  "
			if selected {
				cursor = "> "
			}
			statusLabel := fmt.Sprintf("[%c%c]", cf.StagedStatus, cf.UnstagedStatus)
			line := fmt.Sprintf("%s%s %s", cursor, statusLabel, cf.Path)
			if selected {
				line = lipgloss.NewStyle().Background(lipgloss.Color("17")).Bold(true).
					Width(innerW).Render(line)
			}
			rows = append(rows, line)
		}
	}

	// ── Help bar ────────────────────────────────────────────────────────
	rows = append(rows, "")
	var hints []string
	if ns > 0 {
		hints = append(hints, "r=resume session")
	}
	if totalSessions > countActiveSessions(*p.worktree) {
		if p.showAllSessions {
			hints = append(hints, "a=hide inactive")
		} else {
			hints = append(hints, "a=show all")
		}
	}
	if nf > 0 {
		hints = append(hints, "d=diff  f=file  enter=open")
	}
	hints = append(hints, "n=new session  tab=switch panel")
	rows = append(rows, dim.Render(strings.Join(hints, "   ")))

	return strings.Join(rows, "\n")
}

// renderSessionRow renders one session entry as a compact 2-line block:
//   Line 1: cursor + status dot + slug/id + right-aligned age
//   Line 2: indented summary message (dim)
func renderSessionRow(s tuimsg.ClaudeSession, selected bool, width int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	cursor := "  "
	if selected {
		cursor = "> "
	}

	// Status: colored dot + text marker.
	var statusMarker string
	var statusLen int // visible character count of the marker
	if s.IsActive {
		statusMarker = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render("● RUN")
		statusLen = 5 // "● RUN"
	} else {
		statusMarker = dim.Render("○ idle")
		statusLen = 6 // "○ idle"
	}

	// Use last message as the primary label (more descriptive than slug/ID).
	// Fall back to slug, then truncated ID.
	label := cleanSummary(s.LastMessage)
	if label == "" {
		label = s.Slug
	}
	if label == "" {
		label = s.ID
		if len(label) > 8 {
			label = label[:8]
		}
	}

	age := formatAge(s.LastActivity)

	// Truncate label to fit: width - cursor(2) - marker - space(1) - age - gap(2)
	maxLabel := width - 2 - statusLen - 1 - len(age) - 2
	if maxLabel < 20 {
		maxLabel = 20
	}
	if len(label) > maxLabel {
		label = label[:maxLabel-1] + "…"
	}

	// Line 1: cursor + marker + space + label ... age (right-aligned)
	leftText := fmt.Sprintf("%s%s %s", cursor, statusMarker, label)
	// statusMarker has ANSI; visible width is cursor(2) + marker + space(1) + label
	visibleLeft := 2 + statusLen + 1 + len(label)
	gap := width - visibleLeft - len(age)
	if gap < 2 {
		gap = 2
	}
	header := leftText + strings.Repeat(" ", gap) + age

	// Line 2 (optional): slug as dim subtitle — only if slug exists and differs from label.
	var subtitleLine string
	if s.Slug != "" && s.Slug != label {
		subtitleLine = dim.Render(fmt.Sprintf("    %s", s.Slug))
	}

	// Line 3 (optional): current tool activity for active sessions.
	var toolLine string
	if s.CurrentTool != "" {
		toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Faint(true) // dim yellow
		target := s.CurrentCommand
		maxTarget := width - 10
		if maxTarget < 20 {
			maxTarget = 20
		}
		if len(target) > maxTarget {
			target = target[:maxTarget-1] + "…"
		}
		toolLine = toolStyle.Render(fmt.Sprintf("    [%s] %s", s.CurrentTool, target))
	}

	if selected {
		hl := lipgloss.NewStyle().Background(lipgloss.Color("17")).Bold(true).Width(width)
		header = hl.Render(header)
		if subtitleLine != "" {
			subtitleLine = hl.Render(fmt.Sprintf("    %s", s.Slug))
		}
		if toolLine != "" {
			toolLine = hl.Render(fmt.Sprintf("    [%s] %s", s.CurrentTool, s.CurrentCommand))
		}
	}

	result := header
	if subtitleLine != "" {
		result += "\n" + subtitleLine
	}
	if toolLine != "" {
		result += "\n" + toolLine
	}
	return result
}

// cleanSummary strips markdown formatting and other noise from session messages.
func cleanSummary(msg string) string {
	// Strip markdown bold/italic markers.
	msg = strings.ReplaceAll(msg, "**", "")
	msg = strings.ReplaceAll(msg, "__", "")
	// Strip image references.
	if strings.HasPrefix(msg, "[Image:") || strings.HasPrefix(msg, "[image:") {
		return "(image attached)"
	}
	// Strip leading XML-like tags that slipped through.
	if strings.HasPrefix(msg, "<") {
		if idx := strings.Index(msg, ">"); idx != -1 && idx < 30 {
			rest := strings.TrimSpace(msg[idx+1:])
			if ci := strings.Index(rest, "</"); ci != -1 {
				rest = strings.TrimSpace(rest[:ci])
			}
			if rest != "" {
				return rest
			}
		}
	}
	return msg
}

// formatAge returns a human-readable elapsed time string.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
