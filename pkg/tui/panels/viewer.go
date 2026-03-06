package panels

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// ClaudeLaunchedMsg is sent after attempting to open a Claude session in a new window.
type ClaudeLaunchedMsg struct {
	SessionID string
	Err       error
}

// LaunchClaudeSession opens `claude --resume <sessionID>` in a new terminal window
// so devctl continues running in the current terminal.
// Supports iTerm2 and Terminal.app (macOS); falls back to a shell script approach.
func LaunchClaudeSession(sessionID, projectPath string) tea.Cmd {
	return func() tea.Msg {
		err := openClaudeInNewWindow(sessionID, projectPath)
		return ClaudeLaunchedMsg{SessionID: sessionID, Err: err}
	}
}

// LaunchNewClaudeSession opens a fresh `claude` session in a new terminal window.
func LaunchNewClaudeSession(projectPath string) tea.Cmd {
	return func() tea.Msg {
		err := openNewClaudeInWindow(projectPath)
		return ClaudeLaunchedMsg{SessionID: "new", Err: err}
	}
}

// openClaudeInNewWindow launches claude --resume in a docked pane (iTerm2 vertical split)
// or a new tab (Terminal.app). Uses a login shell so PATH includes ~/go/bin etc.
// Appends `; exec $SHELL` so the pane stays open after Claude exits.
func openClaudeInNewWindow(sessionID, projectPath string) error {
	claudePath := findClaudeBin()

	// Single-quote the path for the shell; path is unlikely to contain ' but handle it.
	safePath := strings.ReplaceAll(projectPath, `'`, `'"'"'`)
	// write text types this into the already-running zsh in the new pane, so no
	// zsh -l -c wrapper needed. Using the full claudePath avoids any PATH issues.
	// Trailing `; exec $SHELL` keeps the pane open after Claude exits.
	shellCmd := fmt.Sprintf("cd '%s' && %s --resume %s; exec $SHELL", safePath, claudePath, sessionID)

	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		return runAppleScript(iterm2SplitScript(shellCmd))
	default:
		return runAppleScript(terminalAppTabScript(shellCmd))
	}
}

// openNewClaudeInWindow launches a fresh claude session (no --resume) in the project dir.
func openNewClaudeInWindow(projectPath string) error {
	claudePath := findClaudeBin()
	safePath := strings.ReplaceAll(projectPath, `'`, `'"'"'`)
	shellCmd := fmt.Sprintf("cd '%s' && %s; exec $SHELL", safePath, claudePath)

	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		return runAppleScript(iterm2SplitScript(shellCmd))
	default:
		return runAppleScript(terminalAppTabScript(shellCmd))
	}
}

// iterm2SplitScript returns AppleScript that splits the current iTerm2 window
// vertically and types cmd into the new pane via write text.
// AppleScript double-quoted strings only require escaping " and \ — single
// quotes are fine as-is, so no shell-style '\'' escaping is needed here.
func iterm2SplitScript(shellCmd string) string {
	// Escape only the characters AppleScript cares about inside "...".
	escaped := strings.ReplaceAll(shellCmd, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`tell application "iTerm2"
	tell current session of current window
		set newSession to (split vertically with default profile)
	end tell
	tell newSession
		write text "%s"
	end tell
end tell`, escaped)
}

// terminalAppTabScript returns AppleScript that opens a new tab in Terminal.app.
func terminalAppTabScript(shellCmd string) string {
	escaped := strings.ReplaceAll(shellCmd, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`tell application "Terminal"
	activate
	tell application "System Events" to keystroke "t" using command down
	delay 0.4
	do script "%s" in front window
end tell`, escaped)
}

// runAppleScript runs an AppleScript via osascript, passing it on stdin to avoid
// shell-escaping the script itself.
func runAppleScript(script string) error {
	cmd := exec.Command("osascript")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findClaudeBin returns the full path to the claude binary, checking common locations.
func findClaudeBin() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, "go", "bin", "claude"),
		"/usr/local/bin/claude",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "claude"
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

// OpenDiff shows the viewer directly in diff mode for a specific file.
// Bypasses file preview and loads diff content immediately with diffMode=0 (unstaged).
func (v *ViewerModel) OpenDiff(worktreePath, filePath string) tea.Cmd {
	v.Visible = true
	v.worktreePath = worktreePath
	v.filePath = filePath
	v.diffMode = 0
	v.showingDiff = true
	v.vp = viewport.New(viewport.WithWidth(v.width-4), viewport.WithHeight(v.height-6))
	return v.loadDiffContent()
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
