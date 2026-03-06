package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonlLine builds a JSONL line for a given type/message combo (minimal structure).
func jsonlLine(t *testing.T, obj map[string]any) string {
	t.Helper()
	b, err := json.Marshal(obj)
	require.NoError(t, err)
	return string(b)
}

func TestClaudeProjectDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		repoPath string
		want     string
	}{
		{
			repoPath: "/Users/daniel/Projects/devctl",
			want:     filepath.Join(home, ".claude", "projects", "-Users-daniel-Projects-devctl"),
		},
		{
			repoPath: "/home/user/my-project",
			want:     filepath.Join(home, ".claude", "projects", "-home-user-my-project"),
		},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, ClaudeProjectDir(tc.repoPath))
	}
}

func TestIsActive(t *testing.T) {
	s := Session{LastActivity: time.Now().Add(-5 * time.Minute)}
	assert.True(t, IsActive(s, 20*time.Minute))
	assert.False(t, IsActive(s, 3*time.Minute))
}

// writeFixtureSession creates a fake Claude project dir structure with a single session JSONL.
func writeFixtureSession(t *testing.T, dir, sessionID string, lines []string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, sessionID+".jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func userTextLine(text, slug, sessionID string) map[string]any {
	return map[string]any{
		"type":      "user",
		"sessionId": sessionID,
		"slug":      slug,
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": text},
			},
		},
	}
}

func toolUseLine(name, path, slug, sessionID string) map[string]any {
	return map[string]any{
		"sessionId": sessionID,
		"slug":      slug,
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": name,
					"input": map[string]any{
						"path": path,
					},
				},
			},
		},
	}
}

func TestScanSessions_NoDir(t *testing.T) {
	sessions, err := ScanSessionsWithThreshold("/nonexistent/path/xyz", 20*time.Minute)
	assert.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestScanSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// Use dir as the project dir directly — no JSONL files.
	// Patch home by temporarily overriding via a subdir structure.
	// Easier: call parseJSONL on a nonexistent file.
	sessions, err := scanSessionsInDir("/nonexistent", dir, 20*time.Minute)
	assert.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestScanSessions_Fixture(t *testing.T) {
	projectDir := t.TempDir()
	repoPath := "/fake/repo"
	sessionID := "aaaabbbb-0000-0000-0000-000000000001"

	lines := []string{
		mustMarshal(t, userTextLine("fix the bug in parser", "cool-session", sessionID)),
		mustMarshal(t, toolUseLine("Read", "/fake/repo/pkg/parser.go", "cool-session", sessionID)),
		mustMarshal(t, toolUseLine("Edit", "/fake/repo/pkg/parser.go", "cool-session", sessionID)),
		mustMarshal(t, toolUseLine("Write", "/fake/repo/pkg/other.go", "cool-session", sessionID)),
		mustMarshal(t, map[string]any{
			"type":      "user",
			"sessionId": sessionID,
			"slug":      "cool-session",
			"message": map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "tool_result", "content": "ok"},
				},
			},
		}),
	}
	writeFixtureSession(t, projectDir, sessionID, lines)

	sessions, err := scanSessionsInDir(repoPath, projectDir, 20*time.Minute)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	s := sessions[0]
	assert.Equal(t, sessionID, s.ID)
	assert.Equal(t, repoPath, s.ProjectPath)
	assert.Equal(t, "cool-session", s.Slug)
	assert.Equal(t, "fix the bug in parser", s.LastMessage)
	assert.True(t, s.IsActive) // just written, well within 20min
	// Deduplicated: parser.go appears only once
	assert.Contains(t, s.RecentFiles, "/fake/repo/pkg/parser.go")
	assert.Contains(t, s.RecentFiles, "/fake/repo/pkg/other.go")
	assert.Len(t, s.RecentFiles, 2)
}

func TestScanSessions_IdleSession(t *testing.T) {
	projectDir := t.TempDir()
	repoPath := "/fake/repo"
	sessionID := "aaaabbbb-0000-0000-0000-000000000002"

	lines := []string{
		mustMarshal(t, userTextLine("old work", "old-session", sessionID)),
	}
	p := writeFixtureSession(t, projectDir, sessionID, lines)

	// Backdate the file mtime by 30 minutes.
	old := time.Now().Add(-30 * time.Minute)
	require.NoError(t, os.Chtimes(p, old, old))

	sessions, err := scanSessionsInDir(repoPath, projectDir, 20*time.Minute)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.False(t, sessions[0].IsActive)
}

func TestScanSessions_MultipleSorted(t *testing.T) {
	projectDir := t.TempDir()
	repoPath := "/fake/repo"

	ids := []string{
		"aaaabbbb-0000-0000-0000-000000000010",
		"aaaabbbb-0000-0000-0000-000000000011",
	}
	for _, id := range ids {
		lines := []string{mustMarshal(t, userTextLine("msg", "slug", id))}
		writeFixtureSession(t, projectDir, id, lines)
	}

	// Make second file newer.
	newer := time.Now()
	older := newer.Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(projectDir, ids[0]+".jsonl"), older, older))
	require.NoError(t, os.Chtimes(filepath.Join(projectDir, ids[1]+".jsonl"), newer, newer))

	sessions, err := scanSessionsInDir(repoPath, projectDir, 20*time.Minute)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	assert.Equal(t, ids[1], sessions[0].ID, "newer session should be first")
	assert.Equal(t, ids[0], sessions[1].ID)
}

func TestScanSessions_SlugFallback(t *testing.T) {
	projectDir := t.TempDir()
	repoPath := "/fake/repo"
	sessionID := "aaaabbbb-0000-0000-0000-000000000003"

	// Only tool_result content, no user text — fallback to slug.
	lines := []string{
		mustMarshal(t, map[string]any{
			"type":      "user",
			"sessionId": sessionID,
			"slug":      "the-slug",
			"message": map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "tool_result", "content": "done"},
				},
			},
		}),
	}
	writeFixtureSession(t, projectDir, sessionID, lines)

	sessions, err := scanSessionsInDir(repoPath, projectDir, 20*time.Minute)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "the-slug", sessions[0].LastMessage)
}

// scanSessionsInDir is a testable variant that accepts a pre-computed project dir.
func scanSessionsInDir(repoPath, dir string, threshold time.Duration) ([]Session, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []Session
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 6 || entry.Name()[len(entry.Name())-6:] != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		id := entry.Name()[:len(entry.Name())-6]
		filePath := filepath.Join(dir, entry.Name())
		lastMsg, slug, recentFiles, extras := parseJSONL(filePath)
		lastActivity := info.ModTime()
		sessions = append(sessions, Session{
			ID:             id,
			ProjectPath:    repoPath,
			Slug:           slug,
			LastActivity:   lastActivity,
			IsActive:       IsActive2(lastActivity, threshold),
			LastMessage:     lastMsg,
			RecentFiles:    recentFiles,
			CurrentTool:    extras.currentTool,
			CurrentCommand: extras.currentCommand,
			RecentTools:    extras.recentTools,
		})
	}
	sortSessions(sessions)
	return sessions, nil
}

func sortSessions(sessions []Session) {
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].LastActivity.After(sessions[i].LastActivity) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// toolUseLineWithCommand creates an assistant entry with a Bash tool_use.
func toolUseLineWithCommand(command, slug, sessionID string) map[string]any {
	return map[string]any{
		"type":      "assistant",
		"sessionId": sessionID,
		"slug":      slug,
		"timestamp": "2026-03-06T10:00:00Z",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": "Bash",
					"input": map[string]any{
						"command": command,
					},
				},
			},
		},
	}
}

func TestExtractRecentTools(t *testing.T) {
	sid := "test-session"
	slug := "test-slug"

	lines := []string{
		mustMarshal(t, userTextLine("do something", slug, sid)),
		mustMarshal(t, toolUseLine("Read", "/foo/bar.go", slug, sid)),
		mustMarshal(t, toolUseLineWithCommand("go test ./...", slug, sid)),
		mustMarshal(t, toolUseLine("Edit", "/foo/baz.go", slug, sid)),
	}

	tools := extractRecentTools(lines, 5)
	require.Len(t, tools, 3)
	// Most recent first (scanning backwards).
	assert.Equal(t, "Edit", tools[0].Tool)
	assert.Equal(t, "/foo/baz.go", tools[0].Target)
	assert.Equal(t, "Bash", tools[1].Tool)
	assert.Equal(t, "go test ./...", tools[1].Target)
	assert.Equal(t, "Read", tools[2].Tool)
	assert.Equal(t, "/foo/bar.go", tools[2].Target)
}

func TestExtractRecentTools_MaxLimit(t *testing.T) {
	sid := "test-session"
	slug := "test-slug"

	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, mustMarshal(t, toolUseLine("Read", fmt.Sprintf("/file%d.go", i), slug, sid)))
	}

	tools := extractRecentTools(lines, 3)
	assert.Len(t, tools, 3, "should respect maxTools limit")
}

func TestDetermineCurrentTool_ActiveTool(t *testing.T) {
	sid := "test-session"
	slug := "test-slug"

	// Last entry is assistant tool_use (after the user entry) → currently executing.
	lines := []string{
		mustMarshal(t, userTextLine("fix the bug", slug, sid)),
		mustMarshal(t, toolUseLine("Write", "/foo/fix.go", slug, sid)),
	}

	tool, cmd := determineCurrentTool(lines)
	assert.Equal(t, "Write", tool)
	assert.Equal(t, "/foo/fix.go", cmd)
}

func TestDetermineCurrentTool_NoActiveTool(t *testing.T) {
	sid := "test-session"
	slug := "test-slug"

	// Last entry is a user entry → no tool currently executing.
	lines := []string{
		mustMarshal(t, toolUseLine("Read", "/foo/bar.go", slug, sid)),
		mustMarshal(t, userTextLine("looks good", slug, sid)),
	}

	tool, cmd := determineCurrentTool(lines)
	assert.Empty(t, tool)
	assert.Empty(t, cmd)
}

func TestExtractRecentTools_DefensiveParsing(t *testing.T) {
	// Malformed JSON should be skipped gracefully.
	lines := []string{
		"not valid json",
		`{"message":{"role":"assistant","content":[{"type":"tool_use"}]}}`, // missing name
		`{"message":{"role":"assistant","content":[{"type":"tool_use","name":"UnknownTool","input":{}}]}}`,
	}

	tools := extractRecentTools(lines, 5)
	// Only the UnknownTool entry should be extracted (name="" is skipped).
	require.Len(t, tools, 1)
	assert.Equal(t, "UnknownTool", tools[0].Tool)
	assert.Empty(t, tools[0].Target)
}

func TestExtractToolTarget_BashTruncation(t *testing.T) {
	longCmd := strings.Repeat("x", 100)
	target := extractToolTarget("Bash", map[string]any{"command": longCmd})
	assert.Len(t, target, 80, "Bash commands should be truncated to 80 chars")
}

func TestScanSessions_ToolActivityPopulated(t *testing.T) {
	projectDir := t.TempDir()
	repoPath := "/fake/repo"
	sessionID := "aaaabbbb-0000-0000-0000-000000000099"

	lines := []string{
		mustMarshal(t, userTextLine("fix the parser", "cool-session", sessionID)),
		mustMarshal(t, toolUseLine("Read", "/fake/repo/parser.go", "cool-session", sessionID)),
		mustMarshal(t, toolUseLineWithCommand("go build ./...", "cool-session", sessionID)),
	}
	writeFixtureSession(t, projectDir, sessionID, lines)

	sessions, err := scanSessionsInDir(repoPath, projectDir, 20*time.Minute)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	s := sessions[0]
	// Last entry is assistant (Bash) after user → currently executing.
	assert.Equal(t, "Bash", s.CurrentTool)
	assert.Equal(t, "go build ./...", s.CurrentCommand)
	assert.Len(t, s.RecentTools, 2)
}
