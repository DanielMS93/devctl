package panels

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/internal/claude"
)

// sessionViewerEntryMsg delivers a new JSONL entry from the tailer to the TUI.
type sessionViewerEntryMsg struct {
	entry claude.JSONLEntry
}

// sessionViewerClosedMsg is sent when the tailer channel closes.
type sessionViewerClosedMsg struct{}

// SessionViewer displays a live-streaming formatted log of Claude session activity.
// It follows the ViewerModel pattern: a plain struct driven from root.go Update().
type SessionViewer struct {
	Visible     bool
	width       int
	height      int
	vp          viewport.Model
	lines       []string
	autoScroll  bool
	sessionID   string
	sessionSlug string

	tailerEntries <-chan claude.JSONLEntry
	tailerCancel  func()
}

// NewSessionViewer creates a hidden SessionViewer.
func NewSessionViewer() SessionViewer {
	return SessionViewer{}
}

// SetSize updates the viewer dimensions.
func (v *SessionViewer) SetSize(width, height int) {
	v.width = width
	v.height = height
	if v.Visible {
		v.vp = viewport.New(viewport.WithWidth(width-4), viewport.WithHeight(height-6))
		v.vp.SetContent(strings.Join(v.lines, "\n"))
		if v.autoScroll {
			v.vp.GotoBottom()
		}
	}
}

// Open starts the live viewer for a Claude session. Returns a tea.Cmd that begins
// polling the tailer channel for new entries.
func (v *SessionViewer) Open(sessionID, slug, jsonlPath string) tea.Cmd {
	v.Visible = true
	v.autoScroll = true
	v.lines = nil
	v.sessionID = sessionID
	v.sessionSlug = slug

	// Create and start the JSONL tailer.
	tailer := claude.NewJSONLTailer(context.Background(), jsonlPath)
	v.tailerEntries = tailer.Entries
	v.tailerCancel = tailer.Stop

	v.vp = viewport.New(viewport.WithWidth(v.width-4), viewport.WithHeight(v.height-6))

	// Return a batch: start the tailer goroutine + start polling the channel.
	return tea.Batch(
		func() tea.Msg {
			tailer.Run() // blocks until stopped; runs in its own goroutine via tea.Cmd
			return nil
		},
		v.pollTailer(),
	)
}

// Close stops the tailer and hides the viewer.
func (v *SessionViewer) Close() {
	if v.tailerCancel != nil {
		v.tailerCancel()
		v.tailerCancel = nil
	}
	v.Visible = false
	v.tailerEntries = nil
	v.lines = nil
	v.sessionID = ""
	v.sessionSlug = ""
}

// pollTailer returns a tea.Cmd that reads the next entry from the tailer channel.
func (v *SessionViewer) pollTailer() tea.Cmd {
	ch := v.tailerEntries
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		entry, ok := <-ch
		if !ok {
			return sessionViewerClosedMsg{}
		}
		return sessionViewerEntryMsg{entry: entry}
	}
}

// Update handles messages when the viewer is visible.
// Returns (consumed bool, cmd tea.Cmd) following the ViewerModel pattern.
func (v *SessionViewer) Update(msg tea.Msg) (bool, tea.Cmd) {
	if !v.Visible {
		return false, nil
	}

	switch msg := msg.(type) {
	case sessionViewerEntryMsg:
		line := formatSessionEntry(msg.entry, v.width-6)
		v.lines = append(v.lines, line)
		v.vp.SetContent(strings.Join(v.lines, "\n"))
		if v.autoScroll {
			v.vp.GotoBottom()
		}
		return true, v.pollTailer()

	case sessionViewerClosedMsg:
		v.Close()
		return true, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			v.Close()
			return true, nil
		case "up", "k":
			v.vp.ScrollUp(1)
			if !v.vp.AtBottom() {
				v.autoScroll = false
			}
			return true, nil
		case "down", "j":
			v.vp.ScrollDown(1)
			if v.vp.AtBottom() {
				v.autoScroll = true
			}
			return true, nil
		case "pgup":
			v.vp.HalfPageUp()
			v.autoScroll = false
			return true, nil
		case "pgdown":
			v.vp.HalfPageDown()
			if v.vp.AtBottom() {
				v.autoScroll = true
			}
			return true, nil
		default:
			var cmd tea.Cmd
			v.vp, cmd = v.vp.Update(msg)
			return true, cmd
		}

	default:
		var cmd tea.Cmd
		v.vp, cmd = v.vp.Update(msg)
		return cmd != nil, cmd
	}
}

// View renders the session viewer overlay.
func (v SessionViewer) View() string {
	title := v.sessionSlug
	if title == "" {
		title = v.sessionID
		if len(title) > 12 {
			title = title[:12]
		}
	}

	autoLabel := "on"
	if !v.autoScroll {
		autoLabel = "off"
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("69")).
		Render("Live: " + title)

	helpLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(fmt.Sprintf("Esc=close  up/down=scroll  (auto-scroll: %s)", autoLabel))

	content := strings.Join([]string{header, v.vp.View(), helpLine}, "\n")

	return lipgloss.NewStyle().
		Width(v.width).
		Height(v.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("69")).
		Render(content)
}

// formatSessionEntry formats a JSONLEntry as a readable log line.
func formatSessionEntry(e claude.JSONLEntry, maxWidth int) string {
	ts := ""
	if !e.Timestamp.IsZero() {
		ts = e.Timestamp.Format("15:04:05")
	} else {
		ts = "--------"
	}

	switch e.Type {
	case "user":
		text := e.UserText
		if text == "" {
			text = "(user input)"
		}
		maxText := maxWidth - 18
		if maxText > 0 && len(text) > maxText {
			text = text[:maxText-1] + "..."
		}
		line := fmt.Sprintf("[%s] USER: %s", ts, text)
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(line)

	case "assistant":
		if e.ToolName != "" {
			target := e.ToolTarget
			maxTarget := maxWidth - 18
			if maxTarget > 0 && len(target) > maxTarget {
				target = target[:maxTarget-1] + "..."
			}
			line := fmt.Sprintf("[%s] [%s] %s", ts, e.ToolName, target)
			// Color by tool type.
			color := "6" // cyan for Read/Write/Edit
			switch e.ToolName {
			case "Bash":
				color = "3" // yellow
			case "Agent":
				color = "5" // magenta
			}
			return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(line)
		}
		line := fmt.Sprintf("[%s] (assistant)", ts)
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(line)

	default:
		line := fmt.Sprintf("[%s] (%s)", ts, e.Type)
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Faint(true).Render(line)
	}
}
