# Phase 6: AI Observability - Research

**Researched:** 2026-03-06
**Domain:** Claude Code session monitoring, idle detection, agent patch management, TUI split-pane
**Confidence:** MEDIUM-HIGH

## Summary

Phase 6 adds three major capability layers to devctl: (1) real-time Claude Code session monitoring with file/command visibility and a live split-pane viewer, (2) idle-branch detection that triggers configurable agent analysis workflows, and (3) a full draft-patch lifecycle (generate, review, apply, revert) backed by git patches stored in SQLite.

The existing codebase provides strong foundations: `internal/claude/scanner.go` already parses JSONL files and extracts sessions, recent files, and active/idle status. The 5-second poll loop in `internal/dashboard/manager.go` already delivers `WorktreeState` with `ClaudeSession` data to the TUI. The right panel already renders sessions with status dots, file lists, and supports launching Claude sessions in new terminal windows. The core work is: (a) enriching the scanner to extract tool_use names + commands being executed in real-time, (b) adding a streaming log viewer panel, (c) building an idle-detection goroutine, (d) implementing agent workflow triggering and patch management via CLI + DB + TUI.

**Primary recommendation:** Keep the poll-based architecture (no fsnotify) for session monitoring. Enhance the existing scanner to extract richer data from JSONL entries. Build the patch system around `git format-patch` / `git am` / `git apply --reverse` with patches stored as blobs in SQLite for portability.

## Standard Stack

### Core (Already in Project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `charm.land/bubbletea/v2` | v2.0.1 | TUI framework | Already in use; v2 API |
| `charm.land/lipgloss/v2` | v2.0.0 | TUI styling | Already in use |
| `charm.land/bubbles/v2` | v2.0.0 | Viewport, etc. | Already in use for viewer panel |
| `modernc.org/sqlite` | v1.18.1 | SQLite (no CGO) | Already in use |
| `github.com/jmoiron/sqlx` | v1.4.0 | SQL helper | Already in use |
| `github.com/spf13/cobra` | v1.10.2 | CLI commands | Already in use |
| `github.com/spf13/viper` | v1.21.0 | Configuration | Already in use |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | DB migrations | Already in use |

### No New Dependencies Needed

This phase requires no new Go dependencies. All capabilities can be built with:
- Standard library `encoding/json` for JSONL parsing (already used)
- Standard library `os/exec` for git CLI subprocesses (already used, pattern in `internal/git/`)
- Standard library `time.Ticker` for idle detection polling
- Existing `bubbles/v2/viewport` for the streaming log viewer

## Architecture Patterns

### Claude Code JSONL File Structure (Verified from Disk)

**Location:** `~/.claude/projects/{encoded-path}/{session-uuid}.jsonl`

**Path encoding:** `strings.ReplaceAll(repoPath, "/", "-")` (already implemented)

**Entry types discovered in current JSONL files:**
| Type | Keys (notable) | Contains |
|------|---------------|----------|
| `user` | `message.content[].type="text"`, `cwd`, `gitBranch` | User prompts |
| `assistant` | `message.content[].type="tool_use"`, `message.content[].name` | Tool invocations with `input.command`, `input.file_path`, `input.path` |
| `progress` | `data.type` (hook_progress, agent_progress) | Intermediate progress events |
| `system` | `content`, `subtype`, `level` | System messages |
| `file-history-snapshot` | `snapshot.trackedFileBackups` | File backup snapshots |
| `last-prompt` | `lastPrompt` | Session last prompt marker |

**Key fields on every entry:** `sessionId`, `cwd`, `gitBranch`, `slug`, `uuid`, `parentUuid`, `timestamp`, `version`

**Tool use extraction pattern (from assistant entries):**
```go
// message.content is []any, each element can be:
// {"type": "tool_use", "name": "Bash", "input": {"command": "...", "description": "..."}}
// {"type": "tool_use", "name": "Read", "input": {"file_path": "..."}}
// {"type": "tool_use", "name": "Write", "input": {"file_path": "..."}}
// {"type": "tool_use", "name": "Edit", "input": {"file_path": "..."}}
// {"type": "tool_use", "name": "Agent", "input": {"prompt": "...", "subagent_type": "..."}}
```

**Subagents:** Sessions can have `subagents/` subdirectory with `agent-{id}.jsonl` and `agent-{id}.meta.json` files. The meta.json contains `{"agentType": "..."}`.

**JSONL write frequency:** During active sessions, entries are written every 1-5 seconds. The file mtime is already used for `IsActive` detection.

**Global history:** `~/.claude/history.jsonl` has one entry per session with `sessionId`, `display`, `project`, `timestamp`. Useful for discovery but project-level JSONLs are the real-time source.

### Recommended Project Structure
```
internal/
  claude/
    scanner.go          # EXISTING - enhance with richer extraction
    scanner_test.go     # EXISTING - extend
    watcher.go          # NEW - tail-based JSONL watcher for live streaming
  agent/
    config.go           # NEW - agent workflow types + viper config
    idle.go             # NEW - idle detection goroutine
    patch.go            # NEW - patch generation, storage, apply, revert
    patch_test.go       # NEW
    store.go            # NEW - SQLite CRUD for patches + agent_runs
    store_test.go       # NEW
  dashboard/
    manager.go          # MODIFY - add idle detection, agent state to snapshot
cmd/devctl/
    agent.go            # NEW - devctl agent {review,apply,revert} commands
pkg/tui/
    tuimsg/messages.go  # MODIFY - add AgentPatch, LiveSessionData types
    panels/
      session_viewer.go # NEW - split-pane live session log viewer
      patches.go        # NEW - patch review panel
    root.go             # MODIFY - wire new panels
pkg/storage/
    migrations/
      005_agent_patches.up.sql    # NEW
      005_agent_patches.down.sql  # NEW
```

### Pattern 1: Enhanced Session Data Extraction

**What:** Extend the existing `parseJSONL` function to extract currently-executing tool names and commands, not just recent files.

**When to use:** Every 5s poll cycle, when building `ClaudeSession` data for the TUI.

**Example:**
```go
// Extend Session struct with live activity info
type Session struct {
    // ... existing fields ...
    CurrentTool    string   // e.g. "Bash", "Read", "Write", "Edit"
    CurrentCommand string   // for Bash: the command being run; for Read/Write: the file path
    ActiveTools    []ToolActivity // last N tool_use events from recent entries
}

type ToolActivity struct {
    Tool      string    // "Bash", "Read", "Write", "Edit", "Agent"
    Target    string    // command or file path
    Timestamp time.Time
}
```

**Implementation:** Read the last ~20 lines of the JSONL file (same approach as existing parseJSONL, which already reads recent lines for file extraction). Extract `tool_use` entries from `assistant` type messages. The last tool_use with no corresponding `tool_result` is the "currently executing" tool.

### Pattern 2: JSONL Tail Watcher for Live Streaming

**What:** A goroutine that tails a specific session's JSONL file and sends new entries to the TUI via a channel.

**When to use:** When user selects a session and opens the split-pane live viewer.

**Why not fsnotify:** The poll-based architecture is already proven. For the live viewer, we need faster updates than 5s, but a simple `time.Ticker` at 500ms that stats the file and reads new bytes from the last-known offset is simpler and more reliable than fsnotify on macOS. JSONL files are append-only, making seek-based tailing trivial.

**Example:**
```go
type JSONLTailer struct {
    path       string
    offset     int64
    ctx        context.Context
    cancel     context.CancelFunc
    entries    chan JSONLEntry
}

func (t *JSONLTailer) Run() {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-t.ctx.Done():
            return
        case <-ticker.C:
            t.readNewEntries()
        }
    }
}

func (t *JSONLTailer) readNewEntries() {
    f, err := os.Open(t.path)
    if err != nil { return }
    defer f.Close()

    info, _ := f.Stat()
    if info.Size() <= t.offset { return }

    f.Seek(t.offset, io.SeekStart)
    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 2<<20), 2<<20)
    for scanner.Scan() {
        var entry JSONLEntry
        if json.Unmarshal(scanner.Bytes(), &entry) == nil {
            t.entries <- entry
        }
    }
    t.offset = info.Size()
}
```

### Pattern 3: Idle Branch Detection

**What:** A goroutine that tracks per-branch activity and fires when a branch goes idle (no commits, no session activity for configurable threshold).

**When to use:** Runs continuously in the Manager, checking state on each poll cycle.

**Key design decisions:**
- Use the existing 5s poll as the check cadence (no separate goroutine needed)
- Track "last activity" per branch as `max(lastCommitTime, lastSessionActivity)`
- `lastSessionActivity` comes from JSONL file mtime (already tracked)
- `lastCommitTime` comes from `git log -1 --format=%ct <branch>` (add to git package)
- Store idle-trigger state in SQLite to avoid re-triggering on restart

**Example:**
```go
type IdleTracker struct {
    // branchKey -> last known activity time
    lastActivity map[string]time.Time
    // branchKey -> whether we already triggered for this idle period
    triggered    map[string]bool
    threshold    time.Duration
}

func (t *IdleTracker) Check(states []tuimsg.WorktreeState) []IdleBranch {
    var idle []IdleBranch
    for _, ws := range states {
        key := ws.RepoPath + ":" + ws.Branch
        latest := ws.PolledAt // fallback
        for _, s := range ws.Sessions {
            if s.LastActivity.After(latest) {
                latest = s.LastActivity
            }
        }
        t.lastActivity[key] = latest
        if time.Since(latest) > t.threshold && !t.triggered[key] {
            t.triggered[key] = true
            idle = append(idle, IdleBranch{RepoPath: ws.RepoPath, Branch: ws.Branch})
        }
        // Reset trigger when activity resumes
        if time.Since(latest) < t.threshold {
            t.triggered[key] = false
        }
    }
    return idle
}
```

### Pattern 4: Git Patch Lifecycle

**What:** Generate, store, review, apply, and revert patches using git CLI.

**Commands used:**
| Operation | Git Command | Notes |
|-----------|-------------|-------|
| Generate diff patch | `git diff > patch.diff` | For uncommitted changes |
| Generate format-patch | `git format-patch -1 HEAD --stdout` | For committed changes |
| Apply patch | `git apply <patch-file>` | Applies without committing |
| Apply with commit | `git am <patch-file>` | Applies and creates commit |
| Revert patch | `git apply --reverse <patch-file>` | Reverses the patch |
| Check applicability | `git apply --check <patch-file>` | Dry run |

**Storage:** Patches stored as text blobs in SQLite (they're typically small, < 100KB). This avoids filesystem management and makes the patch DB portable.

### Pattern 5: TUI Split-Pane Live Viewer

**What:** A viewport-based panel that shows streaming JSONL entries formatted as a readable log.

**When to use:** User selects a Claude session and presses a key to open the live viewer.

**Implementation approach:** Follow the existing `ViewerModel` pattern in `panels/viewer.go`:
- Overlay the right panel (same as file viewer does now)
- Use `bubbles/v2/viewport` for scrollable content
- Format JSONL entries as: `[HH:MM:SS] tool_name target` lines
- Auto-scroll to bottom unless user has scrolled up
- Receive entries via tea.Cmd that reads from the tailer channel

### Anti-Patterns to Avoid

- **fsnotify for JSONL watching:** macOS kqueue has file descriptor limits and reliability issues with rapidly-written files. The JSONL files are append-only and small; seek-based tailing with a 500ms timer is simpler and proven.
- **Spawning agent processes from the TUI goroutine:** Agent workflows (code review, test gen) should run in background goroutines managed by Manager, not spawned from Update(). Use tea.Cmd to receive results.
- **Storing patches on filesystem:** Patches in `~/.devctl/patches/` would need cleanup, atomicity, and path management. SQLite blob storage is atomic, queryable, and already the project's persistence layer.
- **Parsing JSONL with rigid struct types:** Claude's JSONL format evolves across versions. Use `map[string]any` for parsing (as the existing scanner does) with defensive nil checks, not strict struct unmarshaling.
- **Blocking the poll loop for agent execution:** Agent workflows can take minutes. They must run in separate goroutines with results delivered via channels to the Manager.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Patch generation | Custom diff algorithm | `git diff --stdout` / `git format-patch` | Git handles binary files, renames, permissions |
| Patch application | Manual file patching | `git apply` / `git am` | Handles context, conflicts, atomicity |
| Patch reversal | Track original file contents | `git apply --reverse` | Git's built-in reverse is reliable |
| JSONL parsing | Strict struct deserialization | `map[string]any` + type assertions | Format evolves; defensive parsing is resilient |
| Scrollable text viewer | Custom scroll implementation | `bubbles/v2/viewport` | Already used in ViewerModel |
| Terminal window opening | Direct process exec | Existing `openClaudeInNewWindow` pattern | AppleScript iTerm2/Terminal.app support already built |

## Common Pitfalls

### Pitfall 1: JSONL Format Evolution
**What goes wrong:** Claude Code updates change JSONL entry structure, breaking the parser.
**Why it happens:** Claude Code is actively developed; new entry types, fields, and structures appear regularly.
**How to avoid:** Parse with `map[string]any`, never fail on unknown types, log warnings for unexpected structures. The existing scanner already follows this pattern -- continue it.
**Warning signs:** New entry `type` values appearing in logs; nil pointer panics from type assertions.

### Pitfall 2: File Descriptor Exhaustion from Tailing
**What goes wrong:** Opening too many JSONL files simultaneously for tailing.
**Why it happens:** Each active session viewer could hold an open file handle.
**How to avoid:** Open-read-close pattern on each 500ms tick (no persistent file handles). Only tail one session at a time (the user-selected one). Close the tailer when the viewer is dismissed.
**Warning signs:** "too many open files" errors in slog output.

### Pitfall 3: Agent Workflow Runaway
**What goes wrong:** Idle detection triggers agent workflows repeatedly, or agent processes hang.
**Why it happens:** Branch stays idle, trigger fires again after restart. Agent process has no timeout.
**How to avoid:** Store trigger timestamps in SQLite with `triggered_at`. Use `context.WithTimeout` for all agent subprocesses (e.g., 5-minute max). Add cooldown period (e.g., don't re-trigger same branch within 1 hour).
**Warning signs:** Multiple agent_runs records for same branch in quick succession.

### Pitfall 4: Race Between Patch Apply and User Edits
**What goes wrong:** User is editing files when `devctl agent apply` runs, causing conflicts.
**Why it happens:** Draft patches are generated against a snapshot; working tree may have diverged.
**How to avoid:** Always run `git apply --check` before actual apply. If check fails, mark patch as "conflicted" and show user the conflict. Require clean working tree for apply (or use `git stash` + apply + `git stash pop`).
**Warning signs:** `git apply` exit code 1 with conflict messages.

### Pitfall 5: SQLite Patch Blob Size
**What goes wrong:** Very large patches (large file additions) bloat the database.
**Why it happens:** Agent generates patches for entire new files or large refactors.
**How to avoid:** Set a max patch size (e.g., 1MB). Reject patches larger than limit with a warning. For agent-generated patches, the agent should be instructed to keep patches focused and small.
**Warning signs:** Database file growing significantly between polls.

### Pitfall 6: Idle Detection False Positives
**What goes wrong:** Branch marked idle while user is actively thinking/reading docs.
**Why it happens:** No commits and no Claude session activity during a coding break.
**How to avoid:** Default threshold of 20 minutes is reasonable. Make it configurable via `agent.idle_threshold_minutes`. Allow per-branch or per-repo disable via `agent.disabled_repos` config list. Show "idle detected" status in TUI before triggering so user can dismiss.
**Warning signs:** Agent workflows triggering while user is actively working.

## Code Examples

### Enhanced Session Scanning (extend existing parseJSONL)
```go
// Add to internal/claude/scanner.go

type ToolActivity struct {
    Tool      string
    Target    string // file path or command
    Timestamp time.Time
}

// extractRecentTools returns the last N tool activities from JSONL entries.
// Reads from the end of the file for efficiency.
func extractRecentTools(lines []string, maxTools int) []ToolActivity {
    var tools []ToolActivity
    for i := len(lines) - 1; i >= 0 && len(tools) < maxTools; i-- {
        var obj map[string]any
        if json.Unmarshal([]byte(lines[i]), &obj) != nil {
            continue
        }
        if obj["type"] != "assistant" {
            continue
        }
        msg, _ := obj["message"].(map[string]any)
        content, _ := msg["content"].([]any)
        ts, _ := obj["timestamp"].(string)
        parsed, _ := time.Parse(time.RFC3339Nano, ts)

        for _, c := range content {
            cm, _ := c.(map[string]any)
            if cm["type"] != "tool_use" {
                continue
            }
            name, _ := cm["name"].(string)
            input, _ := cm["input"].(map[string]any)
            target := ""
            switch name {
            case "Bash":
                target, _ = input["command"].(string)
                if len(target) > 80 { target = target[:80] }
            case "Read", "Write", "Edit":
                target, _ = input["file_path"].(string)
                if target == "" {
                    target, _ = input["path"].(string)
                }
            case "Agent":
                target, _ = input["subagent_type"].(string)
            }
            tools = append(tools, ToolActivity{
                Tool: name, Target: target, Timestamp: parsed,
            })
        }
    }
    return tools
}
```

### SQLite Schema for Agent Patches
```sql
-- 005_agent_patches.up.sql

CREATE TABLE IF NOT EXISTS agent_runs (
    id          TEXT PRIMARY KEY,
    repo_path   TEXT NOT NULL,
    branch      TEXT NOT NULL,
    workflow     TEXT NOT NULL,  -- code_review, test_generation, refactor, docs, dep_review
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending, running, completed, failed
    triggered_at INTEGER NOT NULL,
    completed_at INTEGER,
    error_msg   TEXT
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_branch ON agent_runs(repo_path, branch);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status);

CREATE TABLE IF NOT EXISTS agent_patches (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    repo_path   TEXT NOT NULL,
    branch      TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT,
    patch_data  TEXT NOT NULL,  -- git patch content
    status      TEXT NOT NULL DEFAULT 'draft',  -- draft, approved, applied, rejected, reverted
    created_at  INTEGER NOT NULL,
    reviewed_at INTEGER,
    applied_at  INTEGER
);

CREATE INDEX IF NOT EXISTS idx_agent_patches_status ON agent_patches(status);
CREATE INDEX IF NOT EXISTS idx_agent_patches_run ON agent_patches(run_id);
```

### Agent CLI Commands Pattern
```go
// cmd/devctl/agent.go - follows existing task.go pattern

var agentCmd = &cobra.Command{
    Use:   "agent",
    Short: "Manage AI agent workflows and patches",
}

var agentReviewCmd = &cobra.Command{
    Use:   "review [patch-id]",
    Short: "Review draft patches (interactive or by ID)",
    RunE: func(cmd *cobra.Command, args []string) error {
        db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
        // List draft patches, show diff, prompt for approve/reject
        return runAgentReview(cmd, db, args)
    },
}

var agentApplyCmd = &cobra.Command{
    Use:   "apply <patch-id>",
    Short: "Apply an approved agent patch",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
        return runAgentApply(cmd, db, args[0])
    },
}

var agentRevertCmd = &cobra.Command{
    Use:   "revert <patch-id>",
    Short: "Revert a previously applied agent patch",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
        return runAgentRevert(cmd, db, args[0])
    },
}
```

### Viper Configuration for Agent Workflows
```yaml
# ~/.devctl/config.yaml
agent:
  enabled: true
  idle_threshold_minutes: 20
  cooldown_minutes: 60
  max_patch_size_kb: 1024
  workflows:
    code_review: true
    test_generation: true
    refactor_suggestions: false
    documentation: false
    dependency_review: false
  disabled_repos: []
```

```go
// internal/agent/config.go
func LoadConfig() AgentConfig {
    return AgentConfig{
        Enabled:          viper.GetBool("agent.enabled"),
        IdleThreshold:    time.Duration(viper.GetInt("agent.idle_threshold_minutes")) * time.Minute,
        Cooldown:         time.Duration(viper.GetInt("agent.cooldown_minutes")) * time.Minute,
        MaxPatchSizeKB:   viper.GetInt("agent.max_patch_size_kb"),
        EnabledWorkflows: loadWorkflowConfig(),
        DisabledRepos:    viper.GetStringSlice("agent.disabled_repos"),
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| fsnotify file watching | Poll-based with seek tailing | Project convention | Simpler, no macOS kqueue issues |
| Store patches on filesystem | SQLite blob storage | This phase design | Atomic, portable, queryable |
| Rigid JSONL parsing | Defensive `map[string]any` parsing | Phase 2.1 | Resilient to Claude format changes |

**Claude Code JSONL format notes:**
- The format has been stable for core fields (`type`, `message`, `sessionId`, `cwd`, `gitBranch`)
- New fields are added without removing old ones (additive evolution)
- Subagent support (`subagents/` directory) is relatively recent
- The `slug` field provides human-readable session names

## Open Questions

1. **Agent workflow execution mechanism**
   - What we know: The system needs to trigger code review, test generation, etc. on idle branches
   - What's unclear: Should these be Claude Code CLI invocations (`claude --print`), custom Go code, or configurable shell scripts?
   - Recommendation: Start with configurable shell commands per workflow type. This gives users maximum flexibility and avoids coupling to any specific AI provider. The default config can ship with Claude CLI commands but users can swap in other tools.

2. **Live session viewer scope**
   - What we know: User wants to see "live streamed output with scroll history"
   - What's unclear: Should this show raw JSONL entries, formatted tool activity log, or actual Claude text output?
   - Recommendation: Show formatted tool activity (timestamp + tool name + target), not raw JSONL. Filter to `assistant` and `user` type entries. Show tool_use names and their targets (file paths, commands). This is actionable information without being overwhelming.

3. **Multi-session monitoring**
   - What we know: AI-01 says "all active sessions with current file modifications and commands"
   - What's unclear: The left panel already shows sessions; is the requirement to show per-session file lists in a new view, or enrich existing session rows?
   - Recommendation: Enrich existing session rows in the right panel to show current tool activity inline. The split-pane viewer (AI-02) is for deep-diving into one session.

4. **Agent workflow output as patches**
   - What we know: Agent results should be draft patches
   - What's unclear: How does an agent workflow produce a patch? If the agent modifies files, we need to capture the diff before/after.
   - Recommendation: Agent workflow runs in a temporary git worktree or stash context. Capture diff with `git diff` after the agent runs. Store as patch. Clean up the worktree/stash.

## Sources

### Primary (HIGH confidence)
- Direct filesystem inspection of `~/.claude/projects/` JSONL files on local machine
- Existing codebase: `internal/claude/scanner.go`, `internal/dashboard/manager.go`, `pkg/tui/root.go`
- Existing codebase: `pkg/tui/panels/right.go`, `pkg/tui/panels/viewer.go`
- go.mod for exact dependency versions

### Secondary (MEDIUM confidence)
- [Git apply documentation](https://git-scm.com/docs/git-apply) - patch apply/reverse semantics
- [Git format-patch docs](https://www.stefaanlippens.net/git-format-patch-and-am.html) - format-patch and am workflow
- [fsnotify GitHub](https://github.com/fsnotify/fsnotify) - kqueue limitations documented in README

### Tertiary (LOW confidence)
- Claude Code JSONL format stability assumption - based on observed format as of March 2026, no official format specification exists

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all libraries already in use, no new dependencies
- Architecture (session monitoring): HIGH - extending proven patterns from Phase 2.1
- Architecture (idle detection): MEDIUM - straightforward but edge cases around false positives need validation
- Architecture (patch lifecycle): MEDIUM - git patch mechanics are well-understood but agent workflow execution strategy has open questions
- Pitfalls: HIGH - based on direct observation of JSONL format and existing codebase patterns

**Research date:** 2026-03-06
**Valid until:** 2026-04-06 (30 days - core tech is stable)
