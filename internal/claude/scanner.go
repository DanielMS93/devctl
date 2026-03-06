// Package claude provides session scanning for Claude Code sessions stored in ~/.claude.
package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ActiveThreshold is the duration after which a session is considered no longer running.
// Claude Code writes to JSONL multiple times per second while active, so 60s of silence
// means the session has stopped.
const ActiveThreshold = 60 * time.Second

// ToolActivity represents one tool invocation extracted from a JSONL assistant entry.
type ToolActivity struct {
	Tool      string    // e.g. "Bash", "Read", "Write", "Edit", "Agent"
	Target    string    // command or file path
	Timestamp time.Time // parsed from the entry's timestamp field
}

// Session represents one Claude Code session for a project.
type Session struct {
	ID                   string
	ProjectPath          string    // absolute path of the repo
	Slug                 string    // human-readable session slug (e.g. "agile-crunching-canyon")
	LastActivity         time.Time // file mtime
	IsActive             bool      // modified within ActiveThreshold
	WaitingForPermission bool      // last entry is tool_use with no tool_result (blocked on user approval)
	LastMessage          string    // last user text message, or slug as fallback
	RecentFiles          []string  // recently touched files (up to 10)
	CurrentTool          string    // name of the most recent tool_use with no subsequent user entry
	CurrentCommand       string    // target of the current tool (file path or truncated command)
	RecentTools          []ToolActivity // last 5 tool activities
}

// ClaudeProjectDir returns the ~/.claude/projects/ directory for the given repo path.
// Claude encodes the path by replacing "/" with "-".
func ClaudeProjectDir(repoPath string) string {
	encoded := strings.ReplaceAll(repoPath, "/", "-")
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects", encoded)
}

// ProjectSessions holds all sessions discovered for one project path.
type ProjectSessions struct {
	RepoPath string
	Sessions []Session
}

// ScanAllProjects discovers every project that has Claude sessions in ~/.claude/projects/
// and returns their sessions, sorted per-project by LastActivity descending.
// It reads the cwd field from JSONL entries to recover the actual filesystem path.
func ScanAllProjects(threshold time.Duration) ([]ProjectSessions, error) {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []ProjectSessions
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(projectsDir, entry.Name())
		repoPath := extractRepoPath(dir)
		if repoPath == "" {
			continue
		}
		sessions, err := ScanSessionsWithThreshold(repoPath, threshold)
		if err != nil || len(sessions) == 0 {
			continue
		}
		results = append(results, ProjectSessions{RepoPath: repoPath, Sessions: sessions})
	}
	return results, nil
}

// extractRepoPath reads the first line of any JSONL file in dir and returns the cwd field.
func extractRepoPath(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 2<<20), 2<<20)
		if sc.Scan() {
			var obj map[string]any
			if json.Unmarshal([]byte(sc.Text()), &obj) == nil {
				if cwd, ok := obj["cwd"].(string); ok && cwd != "" {
					f.Close()
					return cwd
				}
			}
		}
		f.Close()
	}
	return ""
}

// ScanSessions returns all sessions for the given repo path, sorted by LastActivity descending.
// Returns nil, nil if no sessions exist (directory missing).
func ScanSessions(repoPath string) ([]Session, error) {
	return ScanSessionsWithThreshold(repoPath, ActiveThreshold)
}

// ScanSessionsWithThreshold is like ScanSessions but with a configurable active threshold.
func ScanSessionsWithThreshold(repoPath string, threshold time.Duration) ([]Session, error) {
	dir := ClaudeProjectDir(repoPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		filePath := filepath.Join(dir, entry.Name())
		lastMsg, slug, recentFiles, extras := parseJSONL(filePath)
		// Skip metadata-only files (file-history-snapshot, progress) — not real sessions.
		if lastMsg == "" && slug == "" && len(recentFiles) == 0 {
			continue
		}
		lastActivity := info.ModTime()

		sessions = append(sessions, Session{
			ID:                   id,
			ProjectPath:          repoPath,
			Slug:                 slug,
			LastActivity:         lastActivity,
			IsActive:             IsActive2(lastActivity, threshold),
			WaitingForPermission: extras.waitingForPermission,
			LastMessage:          lastMsg,
			RecentFiles:          recentFiles,
			CurrentTool:          extras.currentTool,
			CurrentCommand:       extras.currentCommand,
			RecentTools:          extras.recentTools,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	// Deduplicate by slug — resumed sessions create new JSONL files but keep
	// the same slug. Keep only the most recent (already sorted by activity).
	seen := make(map[string]bool)
	deduped := sessions[:0]
	for _, s := range sessions {
		if s.Slug != "" {
			if seen[s.Slug] {
				continue
			}
			seen[s.Slug] = true
		}
		deduped = append(deduped, s)
	}

	return deduped, nil
}

// IsActive reports whether a session is active given a threshold duration.
func IsActive(s Session, threshold time.Duration) bool {
	return time.Since(s.LastActivity) < threshold
}

// IsActive2 is a helper used internally to avoid re-computing time.Since.
func IsActive2(lastActivity time.Time, threshold time.Duration) bool {
	return time.Since(lastActivity) < threshold
}

// sessionExtras holds the additional fields extracted from JSONL beyond message/slug/files.
type sessionExtras struct {
	waitingForPermission bool
	currentTool          string
	currentCommand string
	recentTools    []ToolActivity
}

// extractRecentTools scans JSONL lines from the end backwards and extracts
// the most recent tool_use activities from assistant entries.
func extractRecentTools(lines []string, maxTools int) []ToolActivity {
	var tools []ToolActivity
	for i := len(lines) - 1; i >= 0 && len(tools) < maxTools; i-- {
		var obj map[string]any
		if json.Unmarshal([]byte(lines[i]), &obj) != nil {
			continue
		}
		msg, _ := obj["message"].(map[string]any)
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		content, _ := msg["content"].([]any)
		// Process content elements in reverse so most-recent tool_use within
		// a single message is added first.
		for j := len(content) - 1; j >= 0 && len(tools) < maxTools; j-- {
			cm, _ := content[j].(map[string]any)
			if cm["type"] != "tool_use" {
				continue
			}
			name, _ := cm["name"].(string)
			if name == "" {
				continue
			}
			input, _ := cm["input"].(map[string]any)
			target := extractToolTarget(name, input)

			var ts time.Time
			if tsStr, ok := obj["timestamp"].(string); ok {
				ts, _ = time.Parse(time.RFC3339Nano, tsStr)
			}

			tools = append(tools, ToolActivity{
				Tool:      name,
				Target:    target,
				Timestamp: ts,
			})
		}
	}
	return tools
}

// extractToolTarget returns the relevant target string for a tool invocation.
func extractToolTarget(toolName string, input map[string]any) string {
	switch toolName {
	case "Bash":
		cmd, _ := input["command"].(string)
		if len(cmd) > 80 {
			cmd = cmd[:80]
		}
		return cmd
	case "Read", "Write", "Edit":
		if fp, ok := input["file_path"].(string); ok && fp != "" {
			return fp
		}
		if p, ok := input["path"].(string); ok && p != "" {
			return p
		}
		return ""
	case "Agent":
		st, _ := input["subagent_type"].(string)
		return st
	default:
		// For unknown tools, try file_path, then path, then command.
		if fp, ok := input["file_path"].(string); ok && fp != "" {
			return fp
		}
		if p, ok := input["path"].(string); ok && p != "" {
			return p
		}
		if cmd, ok := input["command"].(string); ok && cmd != "" {
			if len(cmd) > 80 {
				cmd = cmd[:80]
			}
			return cmd
		}
		return ""
	}
}

// determineCurrentTool checks whether the most recent assistant tool_use is after
// the last user entry, indicating the tool is still executing.
func determineCurrentTool(lines []string) (currentTool, currentCommand string) {
	lastUserIdx := -1
	lastAssistantToolIdx := -1
	var toolName, toolTarget string

	for i := len(lines) - 1; i >= 0; i-- {
		var obj map[string]any
		if json.Unmarshal([]byte(lines[i]), &obj) != nil {
			continue
		}
		entryType, _ := obj["type"].(string)
		if entryType == "user" && lastUserIdx == -1 {
			lastUserIdx = i
		}
		if lastAssistantToolIdx == -1 {
			msg, _ := obj["message"].(map[string]any)
			role, _ := msg["role"].(string)
			if role == "assistant" {
				content, _ := msg["content"].([]any)
				for j := len(content) - 1; j >= 0; j-- {
					cm, _ := content[j].(map[string]any)
					if cm["type"] == "tool_use" {
						name, _ := cm["name"].(string)
						if name != "" {
							lastAssistantToolIdx = i
							toolName = name
							input, _ := cm["input"].(map[string]any)
							toolTarget = extractToolTarget(name, input)
							break
						}
					}
				}
			}
		}
		// Once we've found both, no need to keep scanning.
		if lastUserIdx != -1 && lastAssistantToolIdx != -1 {
			break
		}
	}

	if lastAssistantToolIdx > lastUserIdx {
		return toolName, toolTarget
	}
	return "", ""
}

// detectWaitingForPermission checks if the session is blocked waiting for user
// approval. This is the case when the last assistant entry has a tool_use and
// there's no subsequent tool_result or user entry — the tool call is pending.
func detectWaitingForPermission(lines []string) bool {
	// Scan backwards to find the last meaningful entry type.
	for i := len(lines) - 1; i >= 0; i-- {
		var obj map[string]any
		if json.Unmarshal([]byte(lines[i]), &obj) != nil {
			continue
		}
		entryType, _ := obj["type"].(string)

		// If the last real entry is a user message or tool_result, not waiting.
		if entryType == "user" {
			return false
		}

		// Skip progress/system entries — they don't indicate tool completion.
		if entryType == "progress" || entryType == "system" || entryType == "file-history-snapshot" {
			continue
		}

		// Check if this is an assistant entry with tool_use.
		msg, _ := obj["message"].(map[string]any)
		role, _ := msg["role"].(string)
		if role == "assistant" {
			content, _ := msg["content"].([]any)
			for _, c := range content {
				cm, _ := c.(map[string]any)
				if cm["type"] == "tool_use" {
					return true // tool_use with no subsequent result = waiting
				}
			}
			return false // assistant entry but no tool_use
		}

		// Any other entry type — not waiting.
		return false
	}
	return false
}

// parseJSONL reads a session JSONL file and extracts the last user message text,
// the session slug, up to 10 recently touched file paths, and tool activity.
// It reads the whole file; Claude JSONL files are typically < 5MB.
func parseJSONL(path string) (lastMessage, slug string, recentFiles []string, extras sessionExtras) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil, sessionExtras{}
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2<<20), 2<<20) // 2MB per line
	for sc.Scan() {
		if t := sc.Text(); t != "" {
			lines = append(lines, t)
		}
	}

	if len(lines) == 0 {
		return "", "", nil, sessionExtras{}
	}

	// Check if this is a real session (has user or assistant entries).
	// Files with only file-history-snapshot/progress entries are metadata, not sessions.
	hasSessionContent := false
	for _, line := range lines {
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		t, _ := obj["type"].(string)
		if t == "user" || t == "assistant" {
			hasSessionContent = true
			break
		}
	}
	if !hasSessionContent {
		return "", "", nil, sessionExtras{}
	}

	// Extract slug from last line (present on every entry).
	var lastObj map[string]any
	if json.Unmarshal([]byte(lines[len(lines)-1]), &lastObj) == nil {
		if s, ok := lastObj["slug"].(string); ok {
			slug = s
		}
	}

	// Find the last meaningful user text message (scanning backwards).
	// Skip auto-generated messages like "Tool loaded." and XML-heavy system prompts.
	for i := len(lines) - 1; i >= 0; i-- {
		var obj map[string]any
		if json.Unmarshal([]byte(lines[i]), &obj) != nil {
			continue
		}
		if obj["type"] != "user" {
			continue
		}
		msg, _ := obj["message"].(map[string]any)
		content, _ := msg["content"].([]any)
		for _, c := range content {
			cm, _ := c.(map[string]any)
			if cm["type"] != "text" {
				continue
			}
			if text, _ := cm["text"].(string); text != "" {
				text = strings.TrimSpace(text)
				if !isUsefulMessage(text) {
					continue
				}
				// Strip XML wrapper tags to extract meaningful content.
				text = stripXMLWrappers(text)
				if len(text) > 120 {
					text = text[:120]
				}
				lastMessage = text
				break
			}
		}
		if lastMessage != "" {
			break
		}
	}

	// Fallback: try last assistant text output (what Claude said last).
	if lastMessage == "" {
		for i := len(lines) - 1; i >= 0; i-- {
			var obj map[string]any
			if json.Unmarshal([]byte(lines[i]), &obj) != nil {
				continue
			}
			msg, _ := obj["message"].(map[string]any)
			role, _ := msg["role"].(string)
			if role != "assistant" {
				continue
			}
			content, _ := msg["content"].([]any)
			for _, c := range content {
				cm, _ := c.(map[string]any)
				if cm["type"] != "text" {
					continue
				}
				if text, _ := cm["text"].(string); text != "" {
					text = strings.TrimSpace(text)
					if len(text) > 120 {
						text = text[:120]
					}
					lastMessage = text
					break
				}
			}
			if lastMessage != "" {
				break
			}
		}
	}

	// Final fallback: use slug.
	if lastMessage == "" {
		lastMessage = slug
	}

	// Extract recently touched files from tool_use events in the last 50 lines.
	start := len(lines) - 50
	if start < 0 {
		start = 0
	}
	seen := make(map[string]bool)
	for _, line := range lines[start:] {
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		msg, _ := obj["message"].(map[string]any)
		content, _ := msg["content"].([]any)
		for _, c := range content {
			cm, _ := c.(map[string]any)
			if cm["type"] != "tool_use" {
				continue
			}
			input, _ := cm["input"].(map[string]any)
			for _, key := range []string{"path", "file_path"} {
				if p, ok := input[key].(string); ok && p != "" && !seen[p] {
					seen[p] = true
					recentFiles = append(recentFiles, p)
				}
			}
		}
	}

	if len(recentFiles) > 10 {
		recentFiles = recentFiles[:10]
	}

	// Extract tool activity.
	recentTools := extractRecentTools(lines, 5)
	currentTool, currentCommand := determineCurrentTool(lines)
	waiting := detectWaitingForPermission(lines)
	extras = sessionExtras{
		waitingForPermission: waiting,
		currentTool:          currentTool,
		currentCommand:       currentCommand,
		recentTools:          recentTools,
	}

	return lastMessage, slug, recentFiles, extras
}

// isUsefulMessage returns false for auto-generated or uninformative messages.
func isUsefulMessage(text string) bool {
	lower := strings.ToLower(text)
	skip := []string{
		"tool loaded",
		"tool loaded.",
		"clear",
		"continue",
		"continue from where you left off.",
		"yes",
		"y",
		"ok",
		"approved",
		"[request interrupted by user for tool use]",
	}
	for _, s := range skip {
		if lower == s {
			return false
		}
	}
	// Skip messages that are purely XML tags (system prompts, skill invocations)
	if strings.HasPrefix(text, "<") && !strings.Contains(text[:min(len(text), 50)], " ") {
		return false
	}
	return true
}

// stripXMLWrappers extracts readable text from XML-wrapped content.
// e.g. "<objective>\nDo something\n</objective>" → "Do something"
func stripXMLWrappers(text string) string {
	// Try to find content between common wrapper tags.
	for _, tag := range []string{"objective", "task", "context", "command-message"} {
		open := "<" + tag + ">"
		close := "</" + tag + ">"
		if idx := strings.Index(text, open); idx != -1 {
			after := text[idx+len(open):]
			if end := strings.Index(after, close); end != -1 {
				inner := strings.TrimSpace(after[:end])
				if inner != "" {
					return inner
				}
			}
		}
	}
	// If text starts with XML but we couldn't extract, skip the first tag line.
	if strings.HasPrefix(text, "<") {
		if idx := strings.Index(text, "\n"); idx != -1 {
			rest := strings.TrimSpace(text[idx+1:])
			// Strip closing tags too.
			if ci := strings.Index(rest, "</"); ci != -1 {
				rest = strings.TrimSpace(rest[:ci])
			}
			if rest != "" {
				return rest
			}
		}
	}
	return text
}
