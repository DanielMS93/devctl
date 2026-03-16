package panels

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/DanielMS93/devctl/pkg/tui/tuimsg"
)

// IdeaCreator is the interface for creating ideas from the TUI.
// Implemented by a wrapper around idea.Store in the tui package.
type IdeaCreator interface {
	CreateIdea(ctx context.Context, prompt, repoPath, parentSessionID, parentBranch string) (string, error)
}

// SideQuestCreatedMsg is sent when a side-quest is successfully created from the TUI.
type SideQuestCreatedMsg struct {
	IdeaID string
	Prompt string
}

// SideQuestInput is a text input overlay for spawning side-quests from a session.
type SideQuestInput struct {
	width   int
	height  int
	Visible bool
	input   textinput.Model
	session *tuimsg.ClaudeSession // parent session
	repoPath string
	branch   string
	creator  IdeaCreator
}

// NewSideQuestInput creates a SideQuestInput.
func NewSideQuestInput(creator IdeaCreator) SideQuestInput {
	ti := textinput.New()
	ti.SetWidth(60)
	return SideQuestInput{
		input:   ti,
		creator: creator,
	}
}

// SetSize updates dimensions.
func (s *SideQuestInput) SetSize(width, height int) {
	s.width = width
	s.height = height
	inputW := width - 8
	if inputW > 80 {
		inputW = 80
	}
	if inputW < 20 {
		inputW = 20
	}
	s.input.SetWidth(inputW)
}

// Open shows the input overlay for the given session context.
func (s *SideQuestInput) Open(session *tuimsg.ClaudeSession, repoPath, branch string) tea.Cmd {
	s.Visible = true
	s.session = session
	s.repoPath = repoPath
	s.branch = branch
	s.input.SetValue("")
	return s.input.Focus()
}

// Close hides the input overlay.
func (s *SideQuestInput) Close() {
	s.Visible = false
	s.input.Blur()
}

// Update handles messages when the input is visible.
// Returns (consumed, cmd). If consumed is true, the caller should not process the message further.
func (s *SideQuestInput) Update(msg tea.Msg) (bool, tea.Cmd) {
	if !s.Visible {
		return false, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			s.Close()
			return true, nil
		case "enter":
			prompt := s.input.Value()
			if prompt == "" {
				return true, nil
			}
			s.Close()
			return true, s.createIdea(prompt)
		}
	}

	// Forward to textinput.
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return true, cmd
}

// createIdea creates the side-quest idea and returns a command that sends SideQuestCreatedMsg.
func (s *SideQuestInput) createIdea(prompt string) tea.Cmd {
	sessionID := ""
	if s.session != nil {
		sessionID = s.session.ID
	}
	repoPath := s.repoPath
	branch := s.branch
	creator := s.creator

	return func() tea.Msg {
		if creator == nil {
			return SideQuestCreatedMsg{Prompt: prompt}
		}
		ctx := context.Background()
		id, err := creator.CreateIdea(ctx, prompt, repoPath, sessionID, branch)
		if err != nil {
			return SideQuestCreatedMsg{Prompt: prompt}
		}
		return SideQuestCreatedMsg{IdeaID: id, Prompt: prompt}
	}
}

// View renders the input overlay.
func (s SideQuestInput) View() string {
	if !s.Visible {
		return ""
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")).Render("Side-Quest")

	sessionInfo := ""
	if s.session != nil {
		slug := s.session.Slug
		if len(slug) > 30 {
			slug = slug[:27] + "..."
		}
		sessionInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
			Render(fmt.Sprintf("Parent session: %s", slug))
	}

	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Render("What should this side-quest investigate?")

	inputView := s.input.View()

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
		Render("enter=create  esc=cancel")

	var content string
	if sessionInfo != "" {
		content = fmt.Sprintf("%s\n%s\n\n%s\n%s\n\n%s", title, sessionInfo, prompt, inputView, hint)
	} else {
		content = fmt.Sprintf("%s\n\n%s\n%s\n\n%s", title, prompt, inputView, hint)
	}

	boxW := s.width - 4
	if boxW > 84 {
		boxW = 84
	}

	box := lipgloss.NewStyle().
		Width(boxW).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("2")).
		Render(content)

	// Center the box.
	return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, box)
}
