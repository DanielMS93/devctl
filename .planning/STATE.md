# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-05)

**Core value:** A developer can open `devctl dashboard` and immediately see everything happening across all their repos and worktrees — no lost sessions, no forgotten branches, no missed follow-ups.
**Current focus:** Phase 6 - AI Observability

## Current Position

Phase: 6 of 6 (AI Observability)
Plan: 3 of 6 in current phase — COMPLETE
Status: Live session viewer with JSONL tailer and auto-scrolling formatted log panel
Last activity: 2026-03-06 — Plan 06-03 complete; JSONLTailer, SessionViewer panel, 'l' key wiring

Progress: [██████████████] 78%

## Performance Metrics

**Velocity:**
- Total plans completed: 11
- Average duration: 4 min
- Total execution time: 0.34 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-foundation | 3 | ~22 min | ~7 min |
| 02-git-integration | 7 | ~14 min | ~2 min |

**Recent Trend:**
- Last 5 plans: 01-02 (3 min), 01-03 (~10 min), 02-01 (2 min), 02-02 (2 min), 02-03 (2 min)
- Trend: stable

*Updated after each plan completion*

| Phase 02-git-integration P05 | 5 min | 2 tasks | 3 files |
| Phase 02-git-integration P06 | 2 | 2 tasks | 5 files |
| Phase 03-dashboard-tui P01 | 1 min | 2 tasks | 2 files |
| Phase 05-tasks-and-dependencies P01 | 3 min | 2 tasks | 4 files |
| Phase 05-tasks-and-dependencies P03 | 2 min | 2 tasks | 3 files |
| Phase 05-tasks-and-dependencies P02 | 2 min | 2 tasks | 3 files |
| Phase 05-tasks-and-dependencies P04 | 3 min | 2 tasks | 4 files |
| Phase 06-ai-observability P01 | 2 min | 2 tasks | 5 files |
| Phase 06-ai-observability P02 | 4 min | 2 tasks | 5 files |
| Phase 06-ai-observability P03 | 4 min | 2 tasks | 5 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Stack: Go + Bubbletea/Lipgloss + Cobra + modernc.org/sqlite (no CGO) + git CLI subprocesses
- Architecture: Two-layer (State Manager goroutines -> buffered channel -> Bubbletea TUI); all async work via tea.Cmd, never raw goroutines in Update()
- Storage: WAL mode + single-writer goroutine pattern must be established in Phase 1 before any feature touches the DB
- [01-01] modernc.org/sqlite (not mattn/go-sqlite3): no CGO, pure Go, enables cross-platform binary distribution
- [01-01] golang-migrate/database/sqlite (not sqlite3): sqlite driver wraps modernc; sqlite3 pulls CGO
- [01-01] tools.go build-tag pattern: retains TUI/CLI deps in go.mod before they have importers in actual code
- [01-01] RunMigrations() takes dbPath string not *sqlx.DB: golang-migrate needs its own DSN connection
- [01-02] pkg/tui/tuimsg leaf package: breaks import cycle between pkg/tui and pkg/tui/panels; both import tuimsg without circular dependency
- [01-02] Type aliases in pkg/tui/messages.go: StateEvent = tuimsg.StateEvent preserves public API while resolving cycle
- [01-02] Bubbletea v2 Init() returns tea.Cmd (not (Model,Cmd)); AltScreen is a View field, not a command
- [01-02] Recursive subscription: subscribeToStateEvents() re-armed on every StateEvent; exactly one goroutine blocks at a time
- [01-03] Log to ~/.devctl/devctl.log: slog output must not reach stdout/stderr while TUI owns the terminal
- [01-03] storage.Open() before RunMigrations(): Open configures WAL and max-one-writer before migrations touch the DB; order is load-bearing
- [01-03] Manager.Start(ctx) before cobra.Execute(): Manager alive before any subcommand runs; future non-TUI commands also benefit
- [02-01] git CLI subprocesses not go-git: go-git v5 lacks linked worktree support; subprocess is correct path
- [02-01] Behind=-1 sentinel: git omits branch.ab line entirely when no upstream; -1 distinguishes "no upstream" from "zero behind"
- [02-01] Diff returns []byte not string: raw ANSI bytes passed directly to viewport SetContent(); conversion deferred to caller
- [02-02] worktree_state.behind defaults to -1: sentinel for no upstream tracking branch, consistent with internal/git PollState convention
- [02-02] repo_copy_files.pattern stores exact relative paths: glob expansion deferred to later phase
- [02-03] dbKey{} context key in worktree.go: DB passed via cobra context, not global variable
- [02-03] PersistentPreRunE on root command: single DB injection point covers all current and future subcommands
- [02-03] ensureRepo auto-registers repos on first worktree create: no separate repo add command needed
- [02-03] sanitizeBranch replaces path-unsafe chars with dash: branch feature/add-login becomes directory feature-add-login
- [02-04] tuimsg must NOT import internal/git: Manager owns git->tuimsg mapping to preserve TUI layer independence from subprocess layer
- [02-04] nil DB guard in Manager: loadCachedSnapshot and pollAllWorktrees return empty snapshot when db is nil (test-safe pattern)
- [02-04] Drop-on-full emit: state event channel full means TUI is lagging; drop tick rather than block poller goroutine
- [02-07] File copy failures are non-fatal warnings: worktree create succeeds even if copying repo_copy_files fails
- [02-07] Viper SafeWriteConfig fallback handles first-run case where config file does not yet exist
- [Phase 02-git-integration]: Selection propagated to rightPanel immediately on navigation and on every StateEvent to keep panels in sync
- [Phase 02-git-integration]: ViewerModel is plain struct not tea.Model: drives sub-model from root.go Update() to avoid nested Bubbletea program complexity
- [Phase 02-git-integration]: chroma quick.Highlight with terminal256/monokai: graceful degradation to plain text on highlight error
- [03-01] Status derived purely from existing WorktreeState fields (Sessions, ChangedFiles) -- no new tuimsg types needed
- [03-01] Styled indicator rendered inline rather than plain-text-then-recolor to keep ANSI handling simple
- [05-01] Migration 004 (not 003): 003 reserved for Phase 04 session management
- [05-01] Task state stores only queued/running/completed; blocked is computed by dependency resolver
- [05-03] Branch ref not found returns true (assumed merged post-cleanup) to avoid false blocking
- [05-03] branchMerged map uses explicit false to block; missing key means no branch check needed
- [05-04] Manager owns TaskStore/DepStore initialization and resolves task DAG on each poll cycle (same pattern as git state polling)
- [05-04] Panel overlay pattern: showTaskGraph bool + View() conditional swaps right panel content
- [05-04] Fixed 24-char box width per layer for clean column alignment in task graph rendering
- [05-02] Partial update via Get-then-Update: CLI fetches current task, merges flags, calls Update with full record
- [05-02] Cycle detection in CLI layer (deps.go) not store: keeps DepStore as pure data layer
- [05-01] TaskStore.Get supports UUID prefix match for ergonomic CLI short IDs
- [06-01] Nullable DB columns (completed_at, error_msg, etc.) use Go pointer types for proper NULL roundtrip
- [06-01] AgentRunStore.UpdateStatus auto-sets completed_at on completed/failed transitions
- [06-01] Known workflows loaded explicitly from viper rather than dynamic mapstructure unmarshalling
- [06-02] sessionExtras struct extends parseJSONL return values rather than growing positional returns
- [06-02] Currently executing tool determined by last assistant tool_use index vs last user entry index
- [06-02] Tool activity rendered as dim yellow third line only when CurrentTool is non-empty
- [06-03] Open-read-close pattern per tick avoids stale file handles (per research pitfall 2)
- [06-03] Non-blocking channel send prevents tailer from blocking on slow TUI consumption
- [06-03] Offset initialized to file size: only NEW entries stream, no history replay
- [06-03] tea.Cmd polling pattern for async channel reads — re-arm pollTailer on each entry

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 6: Claude Code session file/process structure may have evolved since training cutoff; fsnotify macOS kqueue stability needs verification; both need research during planning

## Session Continuity

Last session: 2026-03-06
Stopped at: Completed 06-03-PLAN.md
Resume file: None
