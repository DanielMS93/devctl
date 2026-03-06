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

// ActiveThreshold is the default duration after which a session is considered idle.
const ActiveThreshold = 20 * time.Minute

// Session represents one Claude Code session for a project.
type Session struct {
	ID           string
	ProjectPath  string    // absolute path of the repo
	Slug         string    // human-readable session slug (e.g. "agile-crunching-canyon")
	LastActivity time.Time // file mtime
	IsActive     bool      // modified within ActiveThreshold
	LastMessage  string    // last user text message, or slug as fallback
	RecentFiles  []string  // recently touched files (up to 10)
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
		lastMsg, slug, recentFiles := parseJSONL(filePath)
		lastActivity := info.ModTime()

		sessions = append(sessions, Session{
			ID:           id,
			ProjectPath:  repoPath,
			Slug:         slug,
			LastActivity: lastActivity,
			IsActive:     IsActive2(lastActivity, threshold),
			LastMessage:  lastMsg,
			RecentFiles:  recentFiles,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	return sessions, nil
}

// IsActive reports whether a session is active given a threshold duration.
func IsActive(s Session, threshold time.Duration) bool {
	return time.Since(s.LastActivity) < threshold
}

// IsActive2 is a helper used internally to avoid re-computing time.Since.
func IsActive2(lastActivity time.Time, threshold time.Duration) bool {
	return time.Since(lastActivity) < threshold
}

// parseJSONL reads a session JSONL file and extracts the last user message text,
// the session slug, and up to 10 recently touched file paths from tool_use events.
// It reads the whole file; Claude JSONL files are typically < 5MB.
func parseJSONL(path string) (lastMessage, slug string, recentFiles []string) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil
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
		return "", "", nil
	}

	// Extract slug from last line (present on every entry).
	var lastObj map[string]any
	if json.Unmarshal([]byte(lines[len(lines)-1]), &lastObj) == nil {
		if s, ok := lastObj["slug"].(string); ok {
			slug = s
		}
	}

	// Find the last user text message (scanning backwards).
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
				// Strip leading whitespace / XML objective wrappers for readability.
				text = strings.TrimSpace(text)
				// Truncate long prompts.
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

	// Fallback: use slug when no user text found.
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

	return lastMessage, slug, recentFiles
}
