package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"time"
)

// JSONLEntry represents one parsed entry from a Claude session JSONL file.
type JSONLEntry struct {
	Type       string    // "user", "assistant", "system", "progress"
	Timestamp  time.Time // parsed from the entry's timestamp field
	ToolName   string    // for assistant entries with tool_use
	ToolTarget string    // command or file path
	UserText   string    // for user entries, the text content
	Raw        string    // original line for fallback display
}

// JSONLTailer tails a Claude session JSONL file, delivering new entries on a channel.
// It uses an open-read-close pattern per tick (no persistent file handles).
type JSONLTailer struct {
	path    string
	offset  int64
	ctx     context.Context
	cancel  context.CancelFunc
	Entries chan JSONLEntry
}

// NewJSONLTailer creates a tailer that will deliver new JSONL entries on its Entries channel.
// The offset is initialized to the current file size so only NEW entries are delivered.
// Call Run() in a goroutine to start tailing.
func NewJSONLTailer(ctx context.Context, path string) *JSONLTailer {
	childCtx, cancel := context.WithCancel(ctx)

	// Initialize offset to current file size — only show new entries.
	var offset int64
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	}

	return &JSONLTailer{
		path:    path,
		offset:  offset,
		ctx:     childCtx,
		cancel:  cancel,
		Entries: make(chan JSONLEntry, 64),
	}
}

// Run polls the JSONL file at 500ms intervals, parsing and sending new entries.
// It exits when the context is cancelled. Run this in a goroutine.
func (t *JSONLTailer) Run() {
	defer close(t.Entries)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.poll()
		}
	}
}

// poll checks for new data appended to the file since the last read.
func (t *JSONLTailer) poll() {
	info, err := os.Stat(t.path)
	if err != nil || info.Size() <= t.offset {
		return
	}

	f, err := os.Open(t.path)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(t.offset, 0); err != nil {
		return
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2<<20), 2<<20) // 2MB per line, same as scanner.go
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		entry := parseEntry(line)
		// Non-blocking send: skip if channel is full.
		select {
		case t.Entries <- entry:
		default:
		}
	}

	t.offset = info.Size()
}

// Stop cancels the tailer's context, causing Run() to exit.
func (t *JSONLTailer) Stop() {
	t.cancel()
}

// parseEntry parses a single JSONL line into a JSONLEntry.
func parseEntry(line string) JSONLEntry {
	entry := JSONLEntry{Raw: line}

	var obj map[string]any
	if json.Unmarshal([]byte(line), &obj) != nil {
		return entry
	}

	// Extract type.
	if tp, ok := obj["type"].(string); ok {
		entry.Type = tp
	}

	// Extract timestamp.
	if ts, ok := obj["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			entry.Timestamp = parsed
		}
	}

	// Extract content based on message structure.
	msg, _ := obj["message"].(map[string]any)
	role, _ := msg["role"].(string)
	content, _ := msg["content"].([]any)

	switch {
	case entry.Type == "user" || role == "user":
		entry.Type = "user"
		// Extract user text from content blocks.
		for _, c := range content {
			cm, _ := c.(map[string]any)
			if cm["type"] == "text" {
				if text, ok := cm["text"].(string); ok && text != "" {
					entry.UserText = text
					break
				}
			}
		}

	case role == "assistant":
		entry.Type = "assistant"
		// Find the last tool_use in this message.
		for i := len(content) - 1; i >= 0; i-- {
			cm, _ := content[i].(map[string]any)
			if cm["type"] == "tool_use" {
				if name, ok := cm["name"].(string); ok && name != "" {
					entry.ToolName = name
					input, _ := cm["input"].(map[string]any)
					entry.ToolTarget = extractToolTarget(name, input)
					break
				}
			}
		}
	}

	return entry
}
