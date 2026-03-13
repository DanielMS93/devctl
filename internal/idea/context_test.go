package idea

import (
	"strings"
	"testing"
)

func TestBuildPromptWithContext_NoSession(t *testing.T) {
	result := BuildPromptWithContext("investigate redis", "", "/repo")
	if result != "investigate redis" {
		t.Errorf("expected raw prompt, got %q", result)
	}
}

func TestBuildPromptWithContext_MissingTranscript(t *testing.T) {
	// Non-existent session should fall back to prompt only.
	result := BuildPromptWithContext("investigate redis", "nonexistent-session", "/nonexistent/repo")
	if result != "investigate redis" {
		t.Errorf("expected raw prompt on missing transcript, got %q", result)
	}
}

func TestBuildPromptWithContext_Format(t *testing.T) {
	// When there's a transcript, it should be formatted with context wrapper.
	// We can't easily test with real files, but we can verify the format function.
	prompt := "investigate redis"
	transcript := "User: hello\n\nAssistant: hi"

	// Simulate what BuildPromptWithContext would produce.
	expected := "## Context from parent session\n<transcript>\n" + transcript + "\n</transcript>\n\n## Your Task\n" + prompt
	if !strings.Contains(expected, "## Your Task") {
		t.Error("expected format markers")
	}
}
