package tui

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/internal/claude"
	"github.com/DanielMS93/devctl/pkg/tui/panels"
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
	patchPanel    panels.PatchPanel
	showPatches   bool
	ideaPanel      panels.IdeaPanel
	showIdeas      bool
	sideQuestInput panels.SideQuestInput
	viewer         panels.ViewerModel
	sessionViewer  panels.SessionViewer
	logBar         panels.LogBar

	stateChan <-chan StateEvent
}

// NewRootModel creates a RootModel subscribed to the given event channel.
// patchUpdater may be nil if agent features are disabled.
func NewRootModel(events <-chan StateEvent, patchUpdater panels.PatchStatusUpdater, ideaCreator panels.IdeaCreator) RootModel {
	return RootModel{
		leftPanel:      panels.NewRepoPanel(),
		rightPanel:     panels.NewDetailPanel(),
		taskGraph:      panels.NewTaskGraphPanel(),
		patchPanel:     panels.NewPatchPanel(patchUpdater),
		ideaPanel:      panels.NewIdeaPanel(),
		sideQuestInput: panels.NewSideQuestInput(ideaCreator),
		viewer:         panels.NewViewerModel(),
		sessionViewer:  panels.NewSessionViewer(),
		logBar:         panels.NewLogBar(),
		stateChan:      events,
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

	// Always let session viewer process messages when visible (handles its own key routing).
	if m.sessionViewer.Visible {
		consumed, cmd := m.sessionViewer.Update(msg)
		if consumed {
			return m, cmd
		}
	}

	// Always let side-quest input process messages when visible.
	if m.sideQuestInput.Visible {
		consumed, cmd := m.sideQuestInput.Update(msg)
		if consumed {
			return m, cmd
		}
	}

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
				if m.showPatches {
					m.patchPanel.MoveUp()
				} else if m.showIdeas {
					m.ideaPanel.MoveUp()
				} else if m.showTaskGraph {
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
				if m.showPatches {
					m.patchPanel.MoveDown()
				} else if m.showIdeas {
					m.ideaPanel.MoveDown()
				} else if m.showTaskGraph {
					m.taskGraph.ScrollDown()
				} else {
					m.rightPanel.MoveDown()
				}
			}
		case "esc":
			if m.showPatches {
				if !m.patchPanel.CloseDiff() {
					m.showPatches = false
					m.logBar.SetStatus("")
				}
			} else if m.showIdeas {
				m.showIdeas = false
				m.logBar.SetStatus("")
			} else if m.showTaskGraph {
				m.showTaskGraph = false
				m.logBar.SetStatus("")
			}
		case "enter":
			if m.showPatches {
				m.patchPanel.ToggleDiff()
				return m, nil
			}
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
		case "s":
			if m.activePanel == PanelRight && !m.showPatches && !m.showIdeas && !m.showTaskGraph {
				if s := m.rightPanel.SelectedSession(); s != nil {
					wt := m.leftPanel.SelectedWorktree()
					repoPath := ""
					branch := ""
					if wt != nil {
						repoPath = wt.RepoPath
						branch = wt.Branch
					}
					cmd := m.sideQuestInput.Open(s, repoPath, branch)
					m.logBar.SetStatus("Side-quest: type prompt, enter=create, esc=cancel")
					return m, cmd
				}
			}
		case "a":
			if m.showPatches {
				ctx := context.Background()
				if cmd := m.patchPanel.ApprovePatch(ctx); cmd != nil {
					return m, cmd
				}
			} else if m.activePanel == PanelRight && !m.showTaskGraph {
				m.rightPanel.ToggleAllSessions()
			}
		case "x":
			if m.showPatches {
				ctx := context.Background()
				if cmd := m.patchPanel.RejectPatch(ctx); cmd != nil {
					return m, cmd
				}
			}
		case "p":
			m.showPatches = !m.showPatches
			if m.showPatches {
				m.showTaskGraph = false
				m.logBar.SetStatus("Patch review (p to close)")
			} else {
				m.logBar.SetStatus("")
			}
		case "n":
			if wt := m.leftPanel.SelectedWorktree(); wt != nil {
				return m, panels.LaunchNewClaudeSession(wt.WorktreePath)
			}
		case "t":
			m.showTaskGraph = !m.showTaskGraph
			if m.showTaskGraph {
				m.showIdeas = false
				m.logBar.SetStatus("Task graph view (t to close)")
			} else {
				m.logBar.SetStatus("")
			}
		case "i":
			m.showIdeas = !m.showIdeas
			if m.showIdeas {
				m.showTaskGraph = false
				m.showPatches = false
				m.logBar.SetStatus("Idea pipeline (i to close)")
			} else {
				m.logBar.SetStatus("")
			}
		case "l":
			if m.activePanel == PanelRight {
				if s := m.rightPanel.SelectedSession(); s != nil {
					projectDir := claude.ClaudeProjectDir(s.ProjectPath)
					jsonlPath := filepath.Join(projectDir, s.ID+".jsonl")
					cmd := m.sessionViewer.Open(s.ID, s.Slug, jsonlPath)
					m.logBar.SetStatus("Live session viewer (Esc to close)")
					return m, cmd
				}
			}
		}

	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		leftWidth := m.width / 4
		if leftWidth < 20 {
			leftWidth = 20
		}
		if mouse.X < leftWidth {
			m.activePanel = PanelLeft
		} else {
			m.activePanel = PanelRight
		}
		m.propagateSizes()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.propagateSizes()

	case StateEvent:
		m.leftPanel.SetState(msg)
		m.rightPanel.SetWorktree(m.leftPanel.SelectedWorktree())
		m.taskGraph.SetGraph(msg.Snapshot.TaskGraph)
		m.patchPanel.SetPatches(msg.Snapshot.Patches)
		m.ideaPanel.SetGraph(msg.Snapshot.IdeaGraph)
		// Re-arm: exactly one goroutine blocks on the channel at a time.
		cmds = append(cmds, m.subscribeToStateEvents())

	case panels.PatchStatusMsg:
		if msg.Err != nil {
			m.logBar.SetStatus(fmt.Sprintf("Patch error: %v", msg.Err))
		} else {
			m.logBar.SetStatus(fmt.Sprintf("Patch %q %s", msg.Title, msg.Status))
		}

	case panels.EditorFinishedMsg:
		if msg.Err != nil {
			slog.Warn("editor exited with error", "err", msg.Err)
		}

	case panels.SideQuestCreatedMsg:
		if msg.IdeaID != "" {
			m.logBar.SetStatus(fmt.Sprintf("Side-quest %s created: %s", msg.IdeaID, msg.Prompt))
		} else {
			m.logBar.SetStatus("Side-quest queued: " + msg.Prompt)
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
	if m.sessionViewer.Visible {
		right = m.sessionViewer.View()
	} else if m.viewer.Visible {
		right = m.viewer.View()
	} else if m.showPatches {
		right = m.patchPanel.View()
	} else if m.showIdeas {
		right = m.ideaPanel.View()
	} else if m.showTaskGraph {
		right = m.taskGraph.View()
	} else {
		right = m.rightPanel.View()
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	var full string
	if m.sideQuestInput.Visible {
		full = m.sideQuestInput.View()
	} else {
		full = lipgloss.JoinVertical(lipgloss.Left, body, m.logBar.View())
	}

	v := tea.NewView(full)
	v.AltScreen = true                      // v2: declarative; do NOT use tea.EnterAltScreen command
	v.MouseMode = tea.MouseModeCellMotion   // enable mouse click/release/wheel events
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
	m.patchPanel.SetSize(rightWidth, bodyHeight)
	m.patchPanel.SetFocused(m.activePanel == PanelRight && m.showPatches)
	m.ideaPanel.SetSize(rightWidth, bodyHeight)
	m.ideaPanel.SetFocused(m.activePanel == PanelRight && m.showIdeas)
	m.sideQuestInput.SetSize(m.width, m.height)
	m.viewer.SetSize(rightWidth, bodyHeight)
	m.sessionViewer.SetSize(rightWidth, bodyHeight)
	m.logBar.SetWidth(m.width)
}
