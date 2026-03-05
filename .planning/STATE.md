# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-05)

**Core value:** A developer can open `devctl dashboard` and immediately see everything happening across all their repos and worktrees — no lost sessions, no forgotten branches, no missed follow-ups.
**Current focus:** Phase 2 - Git Integration

## Current Position

Phase: 2 of 6 (Git Integration) — IN PROGRESS
Plan: 7 of 7 in current phase — COMPLETE
Status: Phase 2 all plans complete; GIT-09 closed; devctl worktree create copies repo_copy_files; devctl config subcommands available
Last activity: 2026-03-05 — Plan 02-07 complete; file copy on worktree create; config set-copy-files/list-copy-files/set subcommands

Progress: [███████░░░] 40%

## Performance Metrics

**Velocity:**
- Total plans completed: 9
- Average duration: 4 min
- Total execution time: 0.27 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-foundation | 3 | ~22 min | ~7 min |
| 02-git-integration | 7 | ~14 min | ~2 min |

**Recent Trend:**
- Last 5 plans: 01-02 (3 min), 01-03 (~10 min), 02-01 (2 min), 02-02 (2 min), 02-03 (2 min)
- Trend: stable

*Updated after each plan completion*

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
- [02-07] File copy failures are non-fatal warnings: worktree create succeeds even if copying repo_copy_files fails
- [02-07] Viper SafeWriteConfig fallback handles first-run case where config file does not yet exist

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 5: TUI graph layout for dependency visualization has limited prior art in Bubbletea; needs research during planning
- Phase 6: Claude Code session file/process structure may have evolved since training cutoff; fsnotify macOS kqueue stability needs verification; both need research during planning

## Session Continuity

Last session: 2026-03-05
Stopped at: Completed 02-07-PLAN.md — GIT-09 file copy on worktree create; devctl config set-copy-files/list-copy-files/set subcommands; Phase 2 complete
Resume file: None
