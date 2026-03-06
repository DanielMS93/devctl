package panels

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/danielmiessler/devctl/internal/git"
	"github.com/spf13/viper"
)

// diffModeLabels maps DiffMode index to display label.
var diffModeLabels = []string{"unstaged", "staged", "vs main", "vs origin"}

// EditorFinishedMsg is sent when the editor process exits.
type EditorFinishedMsg struct{ Err error }

// ClaudeFinishedMsg is sent when a claude --resume session exits.
type ClaudeFinishedMsg struct{ Err error }

// LaunchClaudeSession suspends the TUI, runs `claude --resume <sessionID>` in
// the session's project directory, then resumes the TUI when Claude exits.
func LaunchClaudeSession(sessionID, projectPath string) tea.Cmd {
	cmd := exec.Command("claude", "--resume", sessionID)
	cmd.Dir = projectPath
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return ClaudeFinishedMsg{Err: err}
	})
}

// ViewerModel displays file content or diff output in a scrollable viewport.
// It overlays the right panel area; root.go shows it when viewer.Visible is true.
type ViewerModel struct {
	Visible      bool
	width        int
	height       int
	vp           viewport.Model
	filePath     string
	worktreePath string
	diffMode     int  // 0-3 cycling through DiffMode constants
	showingDiff  bool // false = file preview, true = diff
}

// NewViewerModel creates a hidden ViewerModel. Call Open() to show it.
func NewViewerModel() ViewerModel {
	return ViewerModel{}
}

// SetSize updates dimensions. Must be called when terminal resizes.
func (v *ViewerModel) SetSize(width, height int) {
	v.width = width
	v.height = height
	if v.Visible {
		v.vp = viewport.New(viewport.WithWidth(width-4), viewport.WithHeight(height-6))
	}
}

// Open shows the viewer for a specific file in a worktree.
// Loads file content immediately with syntax highlighting.
func (v *ViewerModel) Open(worktreePath, filePath string) tea.Cmd {
	v.Visible = true
	v.worktreePath = worktreePath
	v.filePath = filePath
	v.diffMode = 0
	v.showingDiff = false
	v.vp = viewport.New(viewport.WithWidth(v.width-4), viewport.WithHeight(v.height-6))
	return v.loadFileContent()
}

// Close hides the viewer.
func (v *ViewerModel) Close() {
	v.Visible = false
	v.filePath = ""
	v.worktreePath = ""
}

// loadFileContent reads the file and sets syntax-highlighted content in viewport.
func (v *ViewerModel) loadFileContent() tea.Cmd {
	worktreePath := v.worktreePath
	filePath := v.filePath
	return func() tea.Msg {
		content, err := os.ReadFile(worktreePath + "/" + filePath)
		if err != nil {
			return viewerContentMsg{content: fmt.Sprintf("error reading file: %v", err)}
		}
		highlighted := highlightContent(string(content), filePath)
		return viewerContentMsg{content: highlighted}
	}
}

// loadDiffContent fetches git diff output for the current diff mode.
func (v *ViewerModel) loadDiffContent() tea.Cmd {
	mode := git.DiffMode(v.diffMode)
	worktreePath := v.worktreePath
	filePath := v.filePath
	return func() tea.Msg {
		out, err := git.Diff(context.Background(), worktreePath, mode, filePath)
		if err != nil {
			return viewerContentMsg{content: fmt.Sprintf("diff error: %v", err)}
		}
		if len(out) == 0 {
			return viewerContentMsg{content: "(no diff for this mode)"}
		}
		return viewerContentMsg{content: string(out)}
	}
}

type viewerContentMsg struct{ content string }

// Update handles key events when the viewer is visible.
// Returns (bool consumed, tea.Cmd).
func (v *ViewerModel) Update(msg tea.Msg) (bool, tea.Cmd) {
	if !v.Visible {
		return false, nil
	}

	switch msg := msg.(type) {
	case viewerContentMsg:
		v.vp.SetContent(msg.content)
		return true, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			v.Close()
			return true, nil
		case "d":
			// Cycle through diff modes
			if v.showingDiff {
				v.diffMode = (v.diffMode + 1) % 4
			}
			v.showingDiff = true
			return true, v.loadDiffContent()
		case "f":
			// Switch back to file preview
			v.showingDiff = false
			return true, v.loadFileContent()
		case "e":
			// Open in editor via tea.ExecProcess
			return true, openInEditor(v.worktreePath + "/" + v.filePath)
		default:
			// Forward scroll keys to viewport
			var cmd tea.Cmd
			v.vp, cmd = v.vp.Update(msg)
			return true, cmd
		}

	default:
		// Forward other msgs (mouse, etc.) to viewport
		var cmd tea.Cmd
		v.vp, cmd = v.vp.Update(msg)
		return cmd != nil, cmd // only consumed if viewport acted
	}
}

// View renders the viewer overlay. Only call when Visible=true.
func (v ViewerModel) View() string {
	title := v.filePath
	if v.showingDiff {
		title = fmt.Sprintf("%s [diff: %s]", v.filePath, diffModeLabels[v.diffMode])
	}

	help := "Esc=close  d=diff  f=file  e=edit  ↑↓=scroll"

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("69")).
		Render(title)

	helpLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(help)

	content := strings.Join([]string{header, v.vp.View(), helpLine}, "\n")

	return lipgloss.NewStyle().
		Width(v.width).
		Height(v.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("69")).
		Render(content)
}

// highlightContent applies chroma syntax highlighting to file content.
// filename is used for lexer detection by file extension.
// Falls back to plain content if highlighting fails.
func highlightContent(content, filename string) string {
	var sb strings.Builder
	err := quick.Highlight(&sb, content, filename, "terminal256", "monokai")
	if err != nil {
		return content // graceful degradation
	}
	return sb.String()
}

// openInEditor returns a tea.Cmd that suspends the TUI, opens the file in the user's editor,
// then resumes the TUI. Uses tea.ExecProcess (NOT tea.Suspend).
// Priority: viper config "editor" key > $EDITOR env var > "vi" fallback.
func openInEditor(filePath string) tea.Cmd {
	editor := viper.GetString("editor")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	return tea.ExecProcess(exec.Command(editor, filePath), func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}
