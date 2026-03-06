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
type DetailPanel struct {
	width    int
	height   int
	focused  bool
	worktree *tuimsg.WorktreeState

	// cursor is a unified index over (sessions + files).
	// 0..numSessions-1  → session items
	// numSessions..end  → file items
	cursor int
}

func NewDetailPanel() DetailPanel { return DetailPanel{} }

func (p *DetailPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DetailPanel) SetFocused(focused bool) { p.focused = focused }

// SetWorktree updates the displayed worktree and resets navigation.
func (p *DetailPanel) SetWorktree(wt *tuimsg.WorktreeState) {
	p.worktree = wt
	p.cursor = 0
}

func (p *DetailPanel) numSessions() int {
	if p.worktree == nil {
		return 0
	}
	return len(p.worktree.Sessions)
}

func (p *DetailPanel) numFiles() int {
	if p.worktree == nil {
		return 0
	}
	return len(p.worktree.ChangedFiles)
}

func (p *DetailPanel) totalItems() int { return p.numSessions() + p.numFiles() }

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
	if p.worktree == nil || p.cursor >= p.numSessions() {
		return nil
	}
	s := p.worktree.Sessions[p.cursor]
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
	if ns > 0 {
		active := countActiveSessions(*p.worktree)
		rows = append(rows, bold.Render(fmt.Sprintf("Sessions  (%d active)", active)))
		rows = append(rows, dim.Render(strings.Repeat("─", innerW)))

		for i, s := range p.worktree.Sessions {
			selected := p.focused && p.cursor == i
			rows = append(rows, renderSessionRow(s, selected, innerW))
			rows = append(rows, "")
		}
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
	if nf > 0 {
		hints = append(hints, "d=diff  f=file  enter=open")
	}
	hints = append(hints, "tab=switch panel")
	rows = append(rows, dim.Render(strings.Join(hints, "   ")))

	return strings.Join(rows, "\n")
}

// renderSessionRow renders one session entry. All layout strings are plain text;
// the selection highlight is applied to the whole row at the end.
func renderSessionRow(s tuimsg.ClaudeSession, selected bool, width int) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	status := "IDLE  "
	if s.IsActive {
		status = "ACTIVE"
	}

	shortID := s.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	age := formatAge(s.LastActivity)

	// Line 1: cursor + status + short id + age
	header := fmt.Sprintf("%s[%s] %s  %s", cursor, status, shortID, age)

	// Line 2: last message (indented)
	msg := s.LastMessage
	maxMsg := width - 12
	if maxMsg < 20 {
		maxMsg = 20
	}
	if len(msg) > maxMsg {
		msg = msg[:maxMsg-1] + "…"
	}
	msgLine := fmt.Sprintf("    %s", msg)

	// Line 3: files (indented, if any)
	var filesLine string
	if len(s.RecentFiles) > 0 {
		files := make([]string, len(s.RecentFiles))
		for i, f := range s.RecentFiles {
			parts := strings.Split(f, "/")
			if len(parts) > 2 {
				files[i] = strings.Join(parts[len(parts)-2:], "/")
			} else {
				files[i] = f
			}
		}
		joined := strings.Join(files, ", ")
		if len(joined) > width-14 {
			joined = joined[:width-15] + "…"
		}
		filesLine = fmt.Sprintf("    files: %s", joined)
	}

	if selected {
		hl := lipgloss.NewStyle().Background(lipgloss.Color("17")).Bold(true).Width(width)
		header = hl.Render(header)
		msgLine = hl.Render(msgLine)
		if filesLine != "" {
			filesLine = hl.Render(filesLine)
		}
	}

	lines := []string{header, msgLine}
	if filesLine != "" {
		lines = append(lines, filesLine)
	}
	return strings.Join(lines, "\n")
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
