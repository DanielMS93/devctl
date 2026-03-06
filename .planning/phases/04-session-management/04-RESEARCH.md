# Phase 4: Session Management - Research

**Researched:** 2026-03-06
**Domain:** CLI session lifecycle, SQLite persistence, fuzzy selection TUI, terminal context switching
**Confidence:** HIGH

## Summary

Phase 4 adds a "devctl session" concept distinct from the existing Claude session scanner. A devctl session is a user-initiated work context that links a repo, worktree, branch, and optional task together with lifecycle tracking (start/stop, activity timestamps, last command). The existing codebase already has: (a) SQLite + sqlx + golang-migrate for storage, (b) Cobra subcommand patterns for CLI, (c) Bubbletea v2 TUI with channel-based state subscription, and (d) Claude session display in the dashboard. Phase 4 extends all four layers.

The `devctl jump` fuzzy selector is the only genuinely new UI pattern. The bubbles/v2 `list` component with its built-in `DefaultFilter` (sahilm/fuzzy) provides exactly what's needed -- a filterable list with keyboard selection. This component is already a transitive dependency (charm.land/bubbles/v2 is in go.mod). For context switching ("jump to worktree"), the codebase already has the AppleScript-based terminal-window pattern used for Claude session launches -- the same approach works for opening a shell in a worktree directory.

**Primary recommendation:** Add a `sessions` table via migration 003, implement `devctl session start/stop/list` as Cobra subcommands following the existing repo/worktree patterns, build `devctl jump` as a standalone Bubbletea program using the bubbles/v2 list component, and surface session state in the existing dashboard poll loop.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/spf13/cobra` | v1.10.2 | CLI subcommands (session start/stop/list, jump) | Already in use; matches existing repo/worktree patterns |
| `github.com/jmoiron/sqlx` | v1.4.0 | Session CRUD against SQLite | Already in use; StructScan pattern throughout |
| `modernc.org/sqlite` | v1.18.1 | Pure-Go SQLite driver | Already in use; no CGO |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | Schema migration 003 for sessions table | Already in use; embedded FS pattern |
| `charm.land/bubbletea/v2` | v2.0.1 | TUI for `devctl jump` fuzzy selector | Already in use; v2 API (KeyPressMsg, tea.View) |
| `charm.land/bubbles/v2` | v2.0.0 | `list` component with fuzzy filtering for jump | Already in go.mod; DefaultFilter uses sahilm/fuzzy |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/google/uuid` | v1.6.0 | Session ID generation | Already in use for repo/worktree IDs |
| `github.com/spf13/viper` | v1.21.0 | Config for session thresholds, auto-stop behavior | Already in use for session.active_threshold_minutes |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| bubbles/v2 list | Custom textinput + manual fuzzy | Much more code for same result; list has pagination, filtering, help built-in |
| bubbles/v2 list | ktr0731/go-fuzzyfinder | Adds dependency; less control over styling; not Bubbletea-native |
| AppleScript terminal switch | tmux session attach | Limits to tmux users; AppleScript covers Terminal.app + iTerm2 already |

**Installation:**
No new dependencies needed. All libraries already in go.mod.

## Architecture Patterns

### Recommended Project Structure
```
cmd/devctl/
  session.go          # devctl session start/stop/list Cobra subcommands
  jump.go             # devctl jump Cobra command + Bubbletea fuzzy selector
pkg/storage/
  migrations/
    003_sessions.up.sql
    003_sessions.down.sql
internal/dashboard/
  manager.go          # Extended: poll loop enriches sessions with devctl session state
pkg/tui/
  tuimsg/messages.go  # Extended: add DevctlSession to WorktreeState or StateSnapshot
```

### Pattern 1: Session Table Schema
**What:** A `sessions` table tracking user work sessions with lifecycle state.
**When to use:** All session CRUD operations.
**Example:**
```sql
-- Migration 003: Session management
CREATE TABLE IF NOT EXISTS sessions (
    id              TEXT PRIMARY KEY,         -- UUID
    repo_id         TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    worktree_id     TEXT REFERENCES worktrees(id) ON DELETE SET NULL,
    branch          TEXT NOT NULL,
    task_id         TEXT,                     -- nullable, links to future Phase 5 tasks table
    status          TEXT NOT NULL DEFAULT 'active'  CHECK(status IN ('active', 'stopped')),
    started_at      INTEGER NOT NULL,         -- Unix timestamp
    stopped_at      INTEGER,                  -- Unix timestamp, NULL while active
    last_activity   INTEGER NOT NULL,         -- Unix timestamp, updated on activity
    last_command    TEXT NOT NULL DEFAULT '',  -- last CLI command recorded
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_repo_id ON sessions(repo_id);
CREATE INDEX IF NOT EXISTS idx_sessions_worktree_id ON sessions(worktree_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
```

### Pattern 2: Cobra Subcommand Group (session start/stop/list)
**What:** Follow the exact pattern used by `repoCmd` and `worktreeCmd` -- parent command with verb subcommands.
**When to use:** All session CLI operations.
**Example:**
```go
// cmd/devctl/session.go
var sessionCmd = &cobra.Command{
    Use:   "session",
    Short: "Manage work sessions",
}

var sessionStartCmd = &cobra.Command{
    Use:   "start",
    Short: "Start a new work session for the current or specified worktree",
    RunE: func(cmd *cobra.Command, args []string) error {
        db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
        // Auto-detect repo + worktree from cwd if not specified
        return runSessionStart(cmd.Context(), db, flags...)
    },
}

func init() {
    sessionStartCmd.Flags().StringP("repo", "r", "", "Repository path or name")
    sessionStartCmd.Flags().StringP("branch", "b", "", "Branch name (default: current branch)")
    sessionStartCmd.Flags().StringP("task", "t", "", "Optional task ID to associate")
    sessionCmd.AddCommand(sessionStartCmd, sessionStopCmd, sessionListCmd)
}
```

### Pattern 3: Auto-detect Context from CWD
**What:** When `devctl session start` is run without explicit flags, detect the repo and worktree from the current working directory by matching against registered repos/worktrees in the DB.
**When to use:** Default behavior for session start and stop.
**Example:**
```go
func detectContext(ctx context.Context, db *sqlx.DB) (repoID, worktreeID, branch string, err error) {
    cwd, _ := os.Getwd()
    // Check if cwd matches a registered worktree path
    var row struct {
        RepoID     string `db:"repo_id"`
        WorktreeID string `db:"id"`
        Branch     string `db:"branch"`
    }
    err = db.GetContext(ctx, &row, `
        SELECT w.id, w.repo_id, w.branch FROM worktrees w WHERE w.path = ?
    `, cwd)
    if err == nil {
        return row.RepoID, row.WorktreeID, row.Branch, nil
    }
    // Fallback: check if cwd matches a repo path
    // Fallback: run git rev-parse to detect branch
    branch, _ = git.CurrentBranch(ctx, cwd)
    ...
}
```

### Pattern 4: Bubbletea List for Jump Selector
**What:** A standalone Bubbletea program using the bubbles/v2 list component with fuzzy filtering for `devctl jump`.
**When to use:** The `devctl jump` command.
**Example:**
```go
// cmd/devctl/jump.go
type jumpItem struct {
    sessionID    string
    repoName     string
    branch       string
    worktreePath string
    lastActivity time.Time
    lastCommand  string
}

// Implement list.Item and list.DefaultItem interfaces
func (i jumpItem) Title() string       { return fmt.Sprintf("%s / %s", i.repoName, i.branch) }
func (i jumpItem) Description() string { return i.lastCommand }
func (i jumpItem) FilterValue() string { return i.repoName + " " + i.branch }

type jumpModel struct {
    list   list.Model
    choice *jumpItem
}

func (m jumpModel) Init() tea.Cmd { return nil }

func (m jumpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        switch msg.String() {
        case "enter":
            if item, ok := m.list.SelectedItem().(jumpItem); ok {
                m.choice = &item
                return m, tea.Quit
            }
        case "q", "esc":
            return m, tea.Quit
        }
    }
    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}
```

### Pattern 5: Activity Tracking via Cobra PersistentPostRunE
**What:** Record last_command and update last_activity on every devctl command execution for the active session.
**When to use:** SESS-04 requires tracking last_activity and last_command.
**Example:**
```go
// In main.go root command PersistentPostRunE
root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
    // Update active session's last_activity and last_command
    fullCmd := cmd.CommandPath() + " " + strings.Join(args, " ")
    _, _ = db.ExecContext(cmd.Context(),
        `UPDATE sessions SET last_activity = ?, last_command = ?, updated_at = ?
         WHERE status = 'active' AND worktree_id IN (
             SELECT id FROM worktrees WHERE path = ?
         )`, time.Now().Unix(), fullCmd, time.Now().Unix(), cwd)
    return nil
}
```

### Pattern 6: Context Switch via AppleScript (reuse existing)
**What:** Reuse the `openClaudeInNewWindow` / `runAppleScript` pattern from viewer.go for jumping to a worktree.
**When to use:** `devctl jump` after selection.
**Example:**
```go
func jumpToWorktree(worktreePath string) error {
    shellCmd := fmt.Sprintf("cd '%s' && exec $SHELL", safePath(worktreePath))
    switch os.Getenv("TERM_PROGRAM") {
    case "iTerm.app":
        return runAppleScript(iterm2SplitScript(shellCmd))
    default:
        return runAppleScript(terminalAppTabScript(shellCmd))
    }
}
```

### Anti-Patterns to Avoid
- **Storing session state in memory only:** Sessions MUST persist to SQLite. The success criteria explicitly require restart persistence.
- **Polling for session activity:** Use the Cobra hook (PersistentPostRunE) to update activity on each command, not a background poller.
- **Making `devctl jump` part of the dashboard TUI:** Jump should be a standalone command that runs its own mini Bubbletea program, exits, then opens the terminal. It's a one-shot selection, not a persistent view.
- **Using `cd` in the current shell:** A subprocess cannot change the parent shell's directory. Must open a new terminal window/tab/pane.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Fuzzy filtering | Custom string matching | `bubbles/v2 list` with `DefaultFilter` (sahilm/fuzzy) | Handles ranking, highlighting, pagination, keyboard nav |
| Schema migration | Manual DDL in Go code | `golang-migrate/migrate/v4` with embedded FS | Already in use; handles versioning, rollback |
| UUID generation | `crypto/rand` + hex encoding | `google/uuid` v1.6.0 | Already in use; consistent format across tables |
| Terminal context switch | Raw shell exec | Existing AppleScript helpers in viewer.go | Already handles iTerm2 vs Terminal.app detection |

**Key insight:** Almost everything Phase 4 needs is an extension of existing patterns. The only new library surface is `bubbles/v2/list` for the jump selector.

## Common Pitfalls

### Pitfall 1: Orphaned Active Sessions
**What goes wrong:** User starts a session, closes terminal without `devctl session stop`, session stays "active" forever.
**Why it happens:** No daemon or cleanup mechanism.
**How to avoid:** Two approaches (implement both):
  1. On `devctl session start`, auto-stop any existing active session for the same worktree (only one active per worktree).
  2. In `devctl session list`, mark sessions as "stale" if last_activity exceeds the configurable threshold (reuse `session.active_threshold_minutes` from viper).
**Warning signs:** `session list` shows dozens of "active" sessions with old timestamps.

### Pitfall 2: CWD Detection Fails for Unregistered Repos
**What goes wrong:** User runs `devctl session start` in a directory not registered in the DB.
**Why it happens:** The repo/worktree detection relies on exact path match.
**How to avoid:** Fall back to `git rev-parse --show-toplevel` to find the repo root, then auto-register it (reuse `ensureRepo` pattern from worktree.go). Also check if `git rev-parse --git-common-dir` points to a worktree.
**Warning signs:** "no repo found" errors when user is clearly in a git directory.

### Pitfall 3: Import Cycle with AppleScript Helpers
**What goes wrong:** Jump command needs AppleScript helpers from `pkg/tui/panels/viewer.go` but `cmd/devctl/` should not import `pkg/tui/panels`.
**Why it happens:** The AppleScript functions are defined inside the panels package.
**How to avoid:** Extract AppleScript terminal helpers into a shared package (e.g., `internal/terminal/` or `pkg/terminal/`) that both `cmd/devctl/jump.go` and `pkg/tui/panels/viewer.go` can import.
**Warning signs:** Circular import error at compile time.

### Pitfall 4: Bubbletea v2 API Gotchas in Jump Selector
**What goes wrong:** Using v1 API patterns that don't exist in v2.
**Why it happens:** Most online examples target v1.
**How to avoid:** Key differences already documented in the codebase:
  - Use `tea.KeyPressMsg` not `tea.KeyMsg`
  - `Init()` returns `tea.Cmd` not `(tea.Model, tea.Cmd)`
  - `View()` returns `tea.View` not `string`
  - `tea.NewView()` for alt screen support
  - Import paths: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2/list`
**Warning signs:** Compile errors referencing missing types.

### Pitfall 5: Single-Writer Contention
**What goes wrong:** Session start/stop commands conflict with background poller writing to DB.
**Why it happens:** SQLite single-writer with `SetMaxOpenConns(1)`.
**How to avoid:** This is already handled -- `busy_timeout=5000` (5s) means concurrent writes will wait rather than fail. The CLI commands and the background poller use separate `*sqlx.DB` instances (CLI opens its own in main.go). The 5s timeout is generous for the fast writes involved.
**Warning signs:** SQLITE_BUSY errors in logs.

### Pitfall 6: Jump Command Needs Active Sessions from DB
**What goes wrong:** `devctl jump` shows stale or empty list.
**Why it happens:** Jump queries sessions from DB but relies on dashboard's poll loop to have populated data.
**How to avoid:** Jump reads directly from DB. Include worktrees that have active devctl sessions OR active Claude sessions (scan on the fly). Don't depend on the dashboard being open.
**Warning signs:** Jump shows nothing even though worktrees and sessions exist.

## Code Examples

### Migration 003 (verified pattern from existing migrations)
```sql
-- 003_sessions.up.sql
CREATE TABLE IF NOT EXISTS sessions (
    id              TEXT PRIMARY KEY,
    repo_id         TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    worktree_id     TEXT REFERENCES worktrees(id) ON DELETE SET NULL,
    branch          TEXT NOT NULL,
    task_id         TEXT,
    status          TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'stopped')),
    started_at      INTEGER NOT NULL,
    stopped_at      INTEGER,
    last_activity   INTEGER NOT NULL,
    last_command    TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_repo_id ON sessions(repo_id);
CREATE INDEX IF NOT EXISTS idx_sessions_worktree_id ON sessions(worktree_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
```

### Session Start (following ensureRepo pattern)
```go
func runSessionStart(ctx context.Context, db *sqlx.DB, repoPath, branch, taskID string) error {
    repoID, err := ensureRepo(ctx, db, repoPath)
    if err != nil {
        return fmt.Errorf("register repo: %w", err)
    }

    // Auto-stop any existing active session for this worktree
    now := time.Now().Unix()
    _, _ = db.ExecContext(ctx, `
        UPDATE sessions SET status = 'stopped', stopped_at = ?, updated_at = ?
        WHERE repo_id = ? AND branch = ? AND status = 'active'
    `, now, now, repoID, branch)

    id := uuid.New().String()
    _, err = db.ExecContext(ctx, `
        INSERT INTO sessions (id, repo_id, worktree_id, branch, task_id, status, started_at, last_activity, last_command, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, 'active', ?, ?, '', ?, ?)
    `, id, repoID, worktreeID, branch, nullString(taskID), now, now, now, now)
    if err != nil {
        return fmt.Errorf("insert session: %w", err)
    }
    fmt.Printf("Session started: %s (%s/%s)\n", id[:8], filepath.Base(repoPath), branch)
    return nil
}
```

### Session List Output Format
```go
func runSessionList(ctx context.Context, db *sqlx.DB) error {
    rows, err := db.QueryxContext(ctx, `
        SELECT s.id, s.status, s.branch, s.last_activity, s.last_command,
               s.started_at, s.stopped_at, r.name as repo_name, r.path as repo_path
        FROM sessions s
        JOIN repos r ON r.id = s.repo_id
        ORDER BY s.last_activity DESC
    `)
    // Format: STATUS  REPO/BRANCH  LAST_ACTIVITY  LAST_COMMAND
    // Active sessions highlighted, stopped sessions dimmed
}
```

### Jump Selector List Items
```go
// Query active sessions + active worktrees for jump
func loadJumpItems(ctx context.Context, db *sqlx.DB) ([]list.Item, error) {
    rows, err := db.QueryxContext(ctx, `
        SELECT s.id, r.name as repo_name, s.branch, w.path as worktree_path,
               s.last_activity, s.last_command
        FROM sessions s
        JOIN repos r ON r.id = s.repo_id
        LEFT JOIN worktrees w ON w.id = s.worktree_id
        WHERE s.status = 'active'
        ORDER BY s.last_activity DESC
    `)
    // ... scan into jumpItem slice, return as []list.Item
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| bubbletea v1 tea.KeyMsg | bubbletea v2 tea.KeyPressMsg | v2.0.0 (2025) | All key handling code uses v2 API |
| bubbles v1 list.New() | bubbles v2 list.New() with options pattern | v2.0.0 (2025) | List construction uses functional options |
| `github.com/charmbracelet/bubbles` | `charm.land/bubbles/v2` | v2.0.0 (2025) | Import path changed |

**Deprecated/outdated:**
- `tea.KeyMsg`: Removed in v2, use `tea.KeyPressMsg`
- `p.Start()`: Removed in v2, use `p.Run()`
- `View() string`: Changed to `View() tea.View` in v2

## Open Questions

1. **Should `devctl jump` also show worktrees without explicit devctl sessions?**
   - What we know: Success criteria says "fuzzy selector across all active worktree sessions"
   - What's unclear: Does "active worktree sessions" mean only devctl-started sessions, or should it also include worktrees with active Claude sessions?
   - Recommendation: Include both. Show all worktrees that have either an active devctl session or active Claude sessions. This makes jump useful immediately even before users adopt `session start`.

2. **How should context switching work cross-platform?**
   - What we know: Existing code uses AppleScript for macOS (iTerm2 + Terminal.app)
   - What's unclear: Linux/Windows support not addressed
   - Recommendation: For Phase 4, keep macOS-only via AppleScript (matches existing pattern). Add a fallback that prints the `cd` command for the user to paste. Cross-platform terminal management is Phase 6+ scope.

3. **Should session auto-stop on worktree delete?**
   - What we know: Worktree delete cascades in DB via FK (sessions.worktree_id SET NULL)
   - What's unclear: Should the session status also change to 'stopped'?
   - Recommendation: Yes. Add a trigger or handle in runWorktreeDelete to stop active sessions for that worktree.

4. **bubbles/v2 list API specifics**
   - What we know: v2 uses options pattern for construction, has built-in fuzzy filter
   - What's unclear: Exact v2 constructor API (list.New signature may differ from v1)
   - Recommendation: LOW confidence on exact API. Verify by checking `charm.land/bubbles/v2/list` package docs during implementation. The general pattern is correct.

## Sources

### Primary (HIGH confidence)
- Existing codebase: `cmd/devctl/repo.go`, `cmd/devctl/worktree.go` -- Cobra subcommand patterns
- Existing codebase: `pkg/storage/` -- SQLite + sqlx + migrate patterns
- Existing codebase: `pkg/tui/panels/viewer.go` -- AppleScript terminal launch pattern
- Existing codebase: `internal/dashboard/manager.go` -- Background poll loop + state emission
- Existing codebase: `pkg/tui/tuimsg/messages.go` -- Shared message types, leaf package pattern

### Secondary (MEDIUM confidence)
- [charm.land/bubbles/v2/list](https://pkg.go.dev/charm.land/bubbles/v2/list) -- List component with fuzzy filtering
- [charm.land/bubbles/v2](https://pkg.go.dev/charm.land/bubbles/v2) -- Bubbles v2 component library
- [Cobra user guide](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md) -- Subcommand patterns

### Tertiary (LOW confidence)
- [go-fuzzyfinder](https://pkg.go.dev/github.com/ktr0731/go-fuzzyfinder) -- Alternative fuzzy library (not recommended)
- [workmux](https://github.com/raine/workmux) -- Reference for worktree + tmux session patterns

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- All libraries already in go.mod, patterns established in codebase
- Architecture: HIGH -- Follows existing Cobra + SQLite + Bubbletea patterns exactly
- Session schema: HIGH -- Straightforward extension of existing migration pattern
- Jump selector: MEDIUM -- bubbles/v2 list API not verified against actual v2 source; general approach is correct
- Pitfalls: HIGH -- Derived from actual codebase analysis (single-writer, import cycles, v2 API)

**Research date:** 2026-03-06
**Valid until:** 2026-04-06 (stable stack, no fast-moving dependencies)
