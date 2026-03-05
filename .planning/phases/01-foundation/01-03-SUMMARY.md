---
phase: 01-foundation
plan: 03
subsystem: cli
tags: [go, cobra, sqlite, wal, bubbletea, race-detector, integration]

# Dependency graph
requires:
  - phase: 01-01
    provides: WAL-mode SQLite storage.Open/RunMigrations and Go module with full Charm v2 stack
  - phase: 01-02
    provides: dashboard.Manager with Start/Stop/Events; pkg/tui.NewRootModel accepting Manager.Events()

provides:
  - cmd/devctl/main.go — Cobra root command wiring storage init, Manager lifecycle, and TUI into a single binary
  - devctl dashboard subcommand opening three-panel Bubbletea v2 TUI
  - Race-detector smoke test for Manager goroutine start/stop lifecycle
  - Confirmed WAL mode and schema (repos, worktrees, schema_migrations) on first run
  - Complete Phase 1 foundation: race-free, buildable, human-verified runnable binary

affects: [02-worktree-manager, 03-tui-shell, 04-session-tracking, 05-dependency-graph, 06-agent-integration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Log to file not stderr: slog writes to ~/.devctl/devctl.log so structured output never corrupts TUI alt screen"
    - "storage.Open before RunMigrations: Open sets WAL pragmas and single-connection pool; migrations run on already-configured connection"
    - "mgr.Start(ctx) before root.ExecuteContext(ctx): Manager alive for full command lifetime; defer mgr.Stop() cleans up on panic"
    - "tea.NewProgram(m) with no options: AltScreen declared inside View() per v2 API; tea.WithAltScreen() not used"
    - "p.Run() not p.Start(): Start() removed in Bubbletea v2"

key-files:
  created:
    - cmd/devctl/main.go
    - internal/dashboard/manager_test.go
  modified: []

key-decisions:
  - "Log file at ~/.devctl/devctl.log: slog output must not reach stdout/stderr while TUI owns the terminal; file logging is the only safe path"
  - "storage.Open() before RunMigrations(): Open configures WAL and max-one-writer before migrations touch the DB; order is load-bearing"
  - "Manager.Start(ctx) before cobra Execute(): ensures state manager is alive before any subcommand runs, including future non-TUI commands"

patterns-established:
  - "Binary startup sequence: MkdirAll -> Open DB -> RunMigrations -> mgr.Start(ctx) -> cobra.Execute -> defer mgr.Stop()"
  - "TUI launch: tui.NewRootModel(mgr.Events()) -> tea.NewProgram(m) -> p.Run() — no options, no raw goroutines"

# Metrics
duration: ~10min
completed: 2026-03-05
---

# Phase 1 Plan 03: CLI Wiring and Integration Summary

**Cobra CLI wiring storage.Open + RunMigrations + Manager.Start + Bubbletea v2 TUI into a single race-detector-clean binary; human-verified three-panel layout, WAL mode, and clean q-exit**

## Performance

- **Duration:** ~10 min (including human verification checkpoint)
- **Started:** 2026-03-05
- **Completed:** 2026-03-05
- **Tasks:** 3 (2 auto + 1 human-verify checkpoint)
- **Files modified:** 2

## Accomplishments

- Full Cobra CLI entrypoint: root command + `dashboard` subcommand with correct v2 `p.Run()` call
- Structured log file at `~/.devctl/devctl.log` keeps slog output off the terminal while TUI runs
- Race-detector smoke test (`TestManagerStartStop`, `TestManagerChannelBufferSize`) confirms goroutine lifecycle is race-free under `-race`
- Human-verified: three-panel TUI renders, Tab cycles focus, q exits cleanly, `journal_mode=wal` confirmed, tables exist, race-detector run produces no DATA RACE output

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement cmd/devctl/main.go with full Cobra + storage + TUI wiring** - `2ee328e` (feat)
2. **Task 2: Race detector smoke test** - `f18c15a` (test)
3. **Task 3: Verify TUI renders correctly and confirms WAL mode** - human-verify checkpoint, no code commit

**Plan metadata:** (docs commit follows this summary)

## Files Created/Modified

- `cmd/devctl/main.go` — Cobra root + dashboard subcommand; openLogFile helper; full startup sequence
- `internal/dashboard/manager_test.go` — TestManagerStartStop and TestManagerChannelBufferSize under race detector

## Decisions Made

- **Log to file, not stderr:** While the TUI holds alt screen, any write to stderr corrupts the display. All slog output is redirected to `~/.devctl/devctl.log` via `openLogFile`.
- **storage.Open() before RunMigrations():** Open() sets `SetMaxOpenConns(1)` and WAL pragmas on the connection pool. RunMigrations() must operate on an already-configured connection — calling them in reverse order would run migrations without WAL mode set.
- **Manager.Start(ctx) before cobra.Execute():** The Manager must be alive before any subcommand runs so that future non-TUI commands (e.g. `devctl status`) can also access state without special-casing startup order.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - compilation succeeded on first build; race detector clean on first run; human verification passed all seven steps.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 1 complete: Go module, WAL-mode SQLite, background state manager, Bubbletea v2 TUI, Cobra CLI — all race-free and human-verified
- Phase 2 (worktree manager) can import `storage.Open`/`RunMigrations`, extend the Manager's `pollLoop` to populate `StateSnapshot.Repos`, and add repo/worktree list rendering to `RepoPanel`
- The startup sequence in `main.go` requires no changes for Phase 2 — new functionality plugs into Manager and panels, not the entrypoint

## Self-Check: PASSED

- `cmd/devctl/main.go` exists at correct path
- `internal/dashboard/manager_test.go` exists at correct path
- Commits 2ee328e (feat) and f18c15a (test) verified in git log
- Human verified: TUI renders, WAL mode confirmed, race detector clean

---
*Phase: 01-foundation*
*Completed: 2026-03-05*
