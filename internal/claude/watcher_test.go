package claude

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEntry_UserText(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":      "user",
		"timestamp": "2026-03-06T10:00:00Z",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "fix the bug"},
			},
		},
	})
	entry := parseEntry(line)
	assert.Equal(t, "user", entry.Type)
	assert.Equal(t, "fix the bug", entry.UserText)
	assert.Equal(t, "2026-03-06T10:00:00Z", entry.Timestamp.Format(time.RFC3339Nano))
	assert.Equal(t, line, entry.Raw)
}

func TestParseEntry_AssistantToolUse(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":      "assistant",
		"timestamp": "2026-03-06T10:01:00Z",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":  "tool_use",
					"name":  "Bash",
					"input": map[string]any{"command": "go test ./..."},
				},
			},
		},
	})
	entry := parseEntry(line)
	assert.Equal(t, "assistant", entry.Type)
	assert.Equal(t, "Bash", entry.ToolName)
	assert.Equal(t, "go test ./...", entry.ToolTarget)
}

func TestParseEntry_AssistantMultipleToolUse(t *testing.T) {
	// Should pick the LAST tool_use in a multi-content message.
	line := mustMarshal(t, map[string]any{
		"timestamp": "2026-03-06T10:02:00Z",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{"file_path": "/a.go"}},
				map[string]any{"type": "tool_use", "name": "Write", "input": map[string]any{"file_path": "/b.go"}},
			},
		},
	})
	entry := parseEntry(line)
	assert.Equal(t, "Write", entry.ToolName)
	assert.Equal(t, "/b.go", entry.ToolTarget)
}

func TestParseEntry_MalformedJSON(t *testing.T) {
	entry := parseEntry("not json at all")
	assert.Equal(t, "not json at all", entry.Raw)
	assert.Empty(t, entry.Type)
}

func TestParseEntry_SystemType(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":      "system",
		"timestamp": "2026-03-06T10:00:00Z",
	})
	entry := parseEntry(line)
	assert.Equal(t, "system", entry.Type)
}

func TestNewJSONLTailer_OffsetAtFileSize(t *testing.T) {
	tmpFile := createTempJSONL(t, []string{
		mustMarshal(t, map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{}}}),
		mustMarshal(t, map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{}}}),
	})

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)

	tailer := NewJSONLTailer(context.Background(), tmpFile)
	defer tailer.Stop()

	assert.Equal(t, info.Size(), tailer.offset, "offset should be initialized to file size")
}

func TestJSONLTailer_DetectsAppendedLines(t *testing.T) {
	tmpFile := createTempJSONL(t, []string{
		mustMarshal(t, map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{}}}),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tailer := NewJSONLTailer(ctx, tmpFile)
	go tailer.Run()

	// Append a new line to the file.
	newLine := mustMarshal(t, map[string]any{
		"type":      "user",
		"timestamp": "2026-03-06T10:05:00Z",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "hello from append"},
			},
		},
	})
	f, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(newLine + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Wait for the entry to arrive (with timeout).
	select {
	case entry := <-tailer.Entries:
		assert.Equal(t, "user", entry.Type)
		assert.Equal(t, "hello from append", entry.UserText)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for appended entry")
	}
}

func TestJSONLTailer_StopClosesChannel(t *testing.T) {
	tmpFile := createTempJSONL(t, nil)

	tailer := NewJSONLTailer(context.Background(), tmpFile)
	go tailer.Run()

	tailer.Stop()

	// Channel should be closed after Run exits.
	select {
	case _, ok := <-tailer.Entries:
		if ok {
			// Got a spurious entry; drain and check again.
			_, ok = <-tailer.Entries
		}
		assert.False(t, ok, "channel should be closed after Stop")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

// createTempJSONL writes lines to a temp file and returns the path.
func createTempJSONL(t *testing.T, lines []string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "session-*.jsonl")
	require.NoError(t, err)
	for _, line := range lines {
		_, err := f.WriteString(line + "\n")
		require.NoError(t, err)
	}
	require.NoError(t, f.Close())
	return f.Name()
}
