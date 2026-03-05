package tui

import (
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

	leftPanel  panels.RepoPanel
	rightPanel panels.DetailPanel
	logBar     panels.LogBar

	stateChan <-chan StateEvent
}

// NewRootModel creates a RootModel subscribed to the given event channel.
func NewRootModel(events <-chan StateEvent) RootModel {
	return RootModel{
		leftPanel:  panels.NewRepoPanel(),
		rightPanel: panels.NewDetailPanel(),
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

	switch msg := msg.(type) {
	case tea.KeyPressMsg: // v2: was tea.KeyMsg in v1
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % 2
			m.propagateSizes()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.propagateSizes()

	case StateEvent:
		m.leftPanel.SetState(msg)
		// Re-arm: exactly one goroutine blocks on the channel at a time.
		cmds = append(cmds, m.subscribeToStateEvents())
	}

	return m, tea.Batch(cmds...)
}

// View assembles the three panels using Lipgloss layout.
// All dimensions come from the stored WindowSizeMsg — never hardcoded.
// Returns tea.View (v2 API; v1 returned string).
func (m RootModel) View() tea.View {
	left := m.leftPanel.View()
	right := m.rightPanel.View()

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
	m.rightPanel.SetFocused(m.activePanel == PanelRight)
	m.logBar.SetWidth(m.width)
}
