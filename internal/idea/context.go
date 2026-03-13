package idea

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// DefaultMaxTranscriptChars is the default max characters for transcript context.
const DefaultMaxTranscriptChars = 50000

// maxTranscriptChars returns the configured max transcript length.
func maxTranscriptChars() int {
	n := viper.GetInt("idea.max_transcript_chars")
	if n <= 0 {
		return DefaultMaxTranscriptChars
	}
	return n
}

// ReadTranscript reads a Claude session JSONL file and extracts user/assistant
// text content (skipping tool_use blocks). Returns the transcript truncated to
// the configured max chars.
func ReadTranscript(sessionID, repoPath string) (string, error) {
	dir := claudeProjectDir(repoPath)
	path := filepath.Join(dir, sessionID+".jsonl")

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var parts []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2<<20), 2<<20) // 2MB per line

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}

		entryType, _ := obj["type"].(string)

		// Handle user entries.
		if entryType == "user" {
			msg, _ := obj["message"].(map[string]any)
			content, _ := msg["content"].([]any)
			for _, c := range content {
				cm, _ := c.(map[string]any)
				if cm["type"] == "text" {
					if text, _ := cm["text"].(string); text != "" {
						parts = append(parts, "User: "+strings.TrimSpace(text))
					}
				}
			}
			continue
		}

		// Handle assistant entries.
		msg, _ := obj["message"].(map[string]any)
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		content, _ := msg["content"].([]any)
		for _, c := range content {
			cm, _ := c.(map[string]any)
			if cm["type"] == "text" {
				if text, _ := cm["text"].(string); text != "" {
					parts = append(parts, "Assistant: "+strings.TrimSpace(text))
				}
			}
			// Skip tool_use blocks — too verbose for context.
		}
	}

	transcript := strings.Join(parts, "\n\n")

	// Truncate from the beginning, keeping the most recent context.
	maxChars := maxTranscriptChars()
	if len(transcript) > maxChars {
		transcript = transcript[len(transcript)-maxChars:]
		// Trim to the first newline to avoid partial messages.
		if idx := strings.Index(transcript, "\n"); idx != -1 {
			transcript = transcript[idx+1:]
		}
	}

	return transcript, nil
}

// BuildPromptWithContext combines the parent session transcript with the idea prompt.
// If the transcript can't be read, it warns and proceeds with prompt only.
func BuildPromptWithContext(prompt, parentSessionID, repoPath string) string {
	if parentSessionID == "" {
		return prompt
	}

	transcript, err := ReadTranscript(parentSessionID, repoPath)
	if err != nil {
		slog.Warn("could not read parent transcript, proceeding without context",
			"session", parentSessionID, "err", err)
		return prompt
	}

	if transcript == "" {
		return prompt
	}

	return fmt.Sprintf(`## Context from parent session
<transcript>
%s
</transcript>

## Your Task
%s`, transcript, prompt)
}

// claudeProjectDir returns the ~/.claude/projects/ directory for the given repo path.
// Uses the same path encoding as internal/claude/scanner.go.
func claudeProjectDir(repoPath string) string {
	encoded := strings.ReplaceAll(repoPath, "/", "-")
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects", encoded)
}
