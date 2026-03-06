package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/danielmiessler/devctl/pkg/tui/panels"
)

// PanelID identifies which panel is active.
type PanelID int

const (
	PanelLeft PanelID = iota
	PanelRight
)

// RootModel is the top-level Bubbletea model. It composes the three panels
// and owns the subscription to the background state manager's event channel.
type RootModel struct {
	width       int
	height      int
	activePanel PanelID

	leftPanel     panels.RepoPanel
	rightPanel    panels.DetailPanel
	taskGraph     panels.TaskGraphPanel
	showTaskGraph bool
	viewer        panels.ViewerModel
	logBar        panels.LogBar

	stateChan <-chan StateEvent
}

// NewRootModel creates a RootModel subscribed to the given event channel.
func NewRootModel(events <-chan StateEvent) RootModel {
	return RootModel{
		leftPanel:  panels.NewRepoPanel(),
		rightPanel: panels.NewDetailPanel(),
		taskGraph:  panels.NewTaskGraphPanel(),
		viewer:     panels.NewViewerModel(),
		logBar:     panels.NewLogBar(),
		stateChan:  events,
	}
}

// Init returns the initial command: subscribe to state events.
// v2 API: Init() returns tea.Cmd (not (tea.Model, tea.Cmd) as in v1).
func (m RootModel) Init() tea.Cmd {
	return m.subscribeToStateEvents()
}

// Update handles messages. RULES:
//   - Use tea.KeyPressMsg (v2), NOT tea.KeyMsg (v1 — removed).
//   - NEVER spawn raw goroutines here. All async work is a tea.Cmd.
//   - Re-arm the subscription on every StateEvent.
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Always let viewer process messages when visible (handles its own key routing).
	if m.viewer.Visible {
		consumed, cmd := m.viewer.Update(msg)
		if consumed {
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg: // v2: was tea.KeyMsg in v1
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % 2
			m.propagateSizes()
		case "up", "k":
			if m.activePanel == PanelLeft {
				m.leftPanel.MoveUp()
				m.rightPanel.SetWorktree(m.leftPanel.SelectedWorktree())
			} else if m.activePanel == PanelRight {
				if m.showTaskGraph {
					m.taskGraph.ScrollUp()
				} else {
					m.rightPanel.MoveUp()
				}
			}
		case "down", "j":
			if m.activePanel == PanelLeft {
				m.leftPanel.MoveDown()
				m.rightPanel.SetWorktree(m.leftPanel.SelectedWorktree())
			} else if m.activePanel == PanelRight {
				if m.showTaskGraph {
					m.taskGraph.ScrollDown()
				} else {
					m.rightPanel.MoveDown()
				}
			}
		case "esc":
			if m.showTaskGraph {
				m.showTaskGraph = false
				m.logBar.SetStatus("")
			}
		case "enter":
			if m.activePanel == PanelLeft {
				// Dive into the right panel for the selected repo.
				m.activePanel = PanelRight
				m.propagateSizes()
			} else if m.activePanel == PanelRight {
				if s := m.rightPanel.SelectedSession(); s != nil {
					// Launch claude --resume in the session's project directory.
					return m, panels.LaunchClaudeSession(s.ID, s.ProjectPath)
				} else if f := m.rightPanel.SelectedFile(); f != "" {
					if wt := m.leftPanel.SelectedWorktree(); wt != nil {
						return m, m.viewer.Open(wt.WorktreePath, f)
					}
				}
			}
		case "d":
			if m.activePanel == PanelRight {
				if f := m.rightPanel.SelectedFile(); f != "" {
					if wt := m.leftPanel.SelectedWorktree(); wt != nil {
						return m, m.viewer.OpenDiff(wt.WorktreePath, f)
					}
				}
			}
		case "f":
			if m.activePanel == PanelRight {
				if f := m.rightPanel.SelectedFile(); f != "" {
					if wt := m.leftPanel.SelectedWorktree(); wt != nil {
						return m, m.viewer.Open(wt.WorktreePath, f)
					}
				}
			}
		case "r":
			if m.activePanel == PanelRight {
				if s := m.rightPanel.SelectedSession(); s != nil {
					return m, panels.LaunchClaudeSession(s.ID, s.ProjectPath)
				}
			}
		case "a":
			if m.activePanel == PanelRight && !m.showTaskGraph {
				m.rightPanel.ToggleAllSessions()
			}
		case "n":
			if wt := m.leftPanel.SelectedWorktree(); wt != nil {
				return m, panels.LaunchNewClaudeSession(wt.WorktreePath)
			}
		case "t":
			m.showTaskGraph = !m.showTaskGraph
			if m.showTaskGraph {
				m.logBar.SetStatus("Task graph view (t to close)")
			} else {
				m.logBar.SetStatus("")
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.propagateSizes()

	case StateEvent:
		m.leftPanel.SetState(msg)
		m.rightPanel.SetWorktree(m.leftPanel.SelectedWorktree())
		m.taskGraph.SetGraph(msg.Snapshot.TaskGraph)
		// Re-arm: exactly one goroutine blocks on the channel at a time.
		cmds = append(cmds, m.subscribeToStateEvents())

	case panels.EditorFinishedMsg:
		if msg.Err != nil {
			slog.Warn("editor exited with error", "err", msg.Err)
		}

	case panels.ClaudeLaunchedMsg:
		if msg.Err != nil {
			slog.Warn("claude launch failed", "session", msg.SessionID, "err", msg.Err)
			m.logBar.SetStatus("⚠ could not open terminal window — is Terminal.app or iTerm2 running?")
		} else {
			id := msg.SessionID
			if len(id) > 8 {
				id = id[:8]
			}
			m.logBar.SetStatus("✓ opened claude --resume " + id + " in new window")
		}
	}

	return m, tea.Batch(cmds...)
}

// View assembles the three panels using Lipgloss layout.
// All dimensions come from the stored WindowSizeMsg — never hardcoded.
// Returns tea.View (v2 API; v1 returned string).
func (m RootModel) View() tea.View {
	left := m.leftPanel.View()

	var right string
	if m.viewer.Visible {
		right = m.viewer.View()
	} else if m.showTaskGraph {
		right = m.taskGraph.View()
	} else {
		right = m.rightPanel.View()
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	full := lipgloss.JoinVertical(lipgloss.Left, body, m.logBar.View())

	v := tea.NewView(full)
	v.AltScreen = true // v2: declarative; do NOT use tea.EnterAltScreen command
	return v
}

// subscribeToStateEvents returns a tea.Cmd that blocks until the next
// StateEvent from the background manager, then delivers it to Update().
// Bubbletea runs each Cmd in its own goroutine — this is safe.
func (m RootModel) subscribeToStateEvents() tea.Cmd {
	return func() tea.Msg {
		return <-m.stateChan // blocks; exactly one goroutine waits here at a time
	}
}

// propagateSizes distributes terminal dimensions and focus state to all sub-panels.
// Called on every WindowSizeMsg and whenever activePanel changes.
func (m *RootModel) propagateSizes() {
	logBarHeight := 1
	bodyHeight := m.height - logBarHeight

	leftWidth := m.width / 4
	if leftWidth < 20 {
		leftWidth = 20
	}
	rightWidth := m.width - leftWidth

	m.leftPanel.SetSize(leftWidth, bodyHeight)
	m.leftPanel.SetFocused(m.activePanel == PanelLeft)
	m.rightPanel.SetSize(rightWidth, bodyHeight)
	m.rightPanel.SetFocused(m.activePanel == PanelRight && !m.showTaskGraph)
	m.taskGraph.SetSize(rightWidth, bodyHeight)
	m.taskGraph.SetFocused(m.activePanel == PanelRight && m.showTaskGraph)
	m.viewer.SetSize(rightWidth, bodyHeight)
	m.logBar.SetWidth(m.width)
}
