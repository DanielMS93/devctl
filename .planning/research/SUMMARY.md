# Project Research Summary

**Project:** DevCTL — Terminal Developer Session Orchestrator
**Domain:** Go CLI/TUI developer session orchestrator / multi-repo worktree manager
**Researched:** 2026-03-05
**Confidence:** MEDIUM (stack choice HIGH; AI observability patterns MEDIUM; version pins LOW)

## Executive Summary

DevCTL sits in an underserved niche between terminal multiplexers (tmux, zellij) and git TUIs (lazygit, gitui). Neither category provides what DevCTL targets: a structured, git-aware control plane that spans multiple repos and worktrees, tracks AI agent sessions, and manages task dependencies. The recommended approach is a two-layer Go application — a background state layer continuously polling git, filesystem, and AI processes, coupled to a Bubbletea TUI layer via a buffered event channel. This is the same architectural split used by production Go developer tools (soft-serve, lazygit) and maps cleanly to DevCTL's polling-heavy, multi-panel dashboard design. The full stack is Cobra (CLI routing) + Bubbletea/Lipgloss/Bubbles (TUI) + modernc.org/sqlite (no-CGO persistence) + git CLI subprocesses (behavioral fidelity over library bindings).

The most important early decision is establishing correct concurrency discipline before any feature code is written. Bubbletea's Elm architecture prohibits direct goroutine mutation of model state — all async work must go through `tea.Cmd` returning `tea.Msg`. Violating this produces silent data races that are hard to reproduce and expensive to fix. Equally critical: the SQLite storage layer must be initialized with WAL mode and a single-writer goroutine pattern before any feature touches the database, or session checkpoint reliability is compromised from the start. Both of these are foundation-phase concerns, not implementation details.

DevCTL's most valuable differentiators — cross-repo dependency graphs, live AI session observability, and proactive idle-triggered agent actions — are all high-complexity features that depend on a stable core (session model, worktree manager, git integration) being in place first. The research strongly supports a layered build: establish storage and git fundamentals, build the interactive dashboard, then add session management and context switching, before tackling dependencies, AI observability, and the idle/agent workflow. This order matches both the architectural dependency graph and the trust-building sequence that developer tool adoption requires.

## Key Findings

### Recommended Stack

The Go ecosystem has a clear, well-adopted stack for this class of tool. Bubbletea (Elm architecture TUI) + Cobra (CLI routing) + Lipgloss (layout/styling) is the same combination used by `gh`, `soft-serve`, and other production Charm-stack tools. For SQLite, `modernc.org/sqlite` is preferred over `mattn/go-sqlite3` because it requires no CGO, which is critical for a developer tool distributed as a single binary via `go install`. Git integration uses `os/exec` subprocess calls rather than `go-git` — behavioral fidelity to the git CLI matters more than avoiding a subprocess, and `go-git` has known gaps in worktree support. All logging uses stdlib `log/slog` (Go 1.21+); no external logging library is warranted for a local CLI tool.

**Core technologies:**
- `github.com/charmbracelet/bubbletea`: TUI event loop (Elm MVU) — de facto standard; used in production by major CLIs
- `github.com/charmbracelet/lipgloss`: terminal layout/styling — first-party Bubbletea companion
- `github.com/charmbracelet/bubbles`: pre-built TUI components (viewport, list, textinput) — avoid reimplementing these
- `github.com/spf13/cobra`: CLI subcommand routing — dominant Go CLI framework; best-in-class help and completions
- `github.com/spf13/viper`: config management — natural cobra companion; supports `~/.devctl/config.toml`
- `modernc.org/sqlite`: pure-Go SQLite driver — no CGO; critical for cross-platform binary distribution
- `github.com/jmoiron/sqlx`: SQL utilities — thin wrapper adding struct scanning without ORM magic
- `github.com/golang-migrate/migrate/v4`: schema migrations — embed SQL in binary; explicit and auditable
- `os/exec` (stdlib): git integration — subprocess calls; behavioral fidelity over library bindings
- `log/slog` (stdlib): structured logging — Go 1.21+; no external dep needed
- Go 1.22+: minimum version — enables range-over-integer, slog, embed

### Expected Features

No existing tool covers DevCTL's combination of worktree management, session tracking, and AI observability. The competitive landscape (tmux, zellij, lazygit, mprocs) establishes the UX baseline for table stakes, while DevCTL's differentiators are genuinely novel in the developer tooling space.

**Must have (table stakes):**
- Dashboard with session list, git status per worktree, branch name — the baseline every session/git tool provides
- Worktree list, create, switch, delete — core unit of work in DevCTL's model
- Keyboard-only navigation (arrow keys, hjkl, tab, esc) — non-negotiable for terminal power users
- Fuzzy search / fast context switch (`devctl jump`) — fzf popularized this; engineers expect it everywhere
- Changed files list and inline diff per worktree — lazygit and gitui set this expectation
- Persistent state across restarts (SQLite-backed) — session state that disappears on restart kills trust
- Fast startup (<500ms) — tools that are slow to start get abandoned
- Clear error states with visible messages — silent failures destroy trust faster than visible ones

**Should have (differentiators):**
- Session resurrection / crash recovery — tmux and zellij lose sessions on server restart; checkpoint-based resilience is rare
- Cross-repo dependency graph with blocked task surfacing — nothing in the market does this
- Live AI session observability (Claude Code docked window) — no tool provides a structured view into what an AI agent is doing
- Idle branch detection with draft agent action workflow — novel; proactively closes the "forgot about that branch" loop
- Worktree-first design as the primary unit of work — lazygit has minimal worktree support; devctl makes it first-class

**Defer (v2+):**
- Cloud sync / team sharing — local-only is a feature, not a limitation; cloud adds auth, infra, privacy
- Plugin/extension system — premature before core is stable; adds API surface to maintain
- CI/CD status integration — requires per-provider integrations; out of scope
- Remote repository management — not a session problem; use git clone directly
- Interactive rebase / per-file git blame — lazygit owns this; defer and document it

### Architecture Approach

DevCTL uses a strict two-layer architecture: a **State Manager** layer (background goroutines polling git, filesystem, AI processes, idle timers) feeds events via a buffered channel to a **TUI Root Model** (Bubbletea), which renders state and handles user input. The TUI never calls external systems directly from `Update()` — all I/O is wrapped in `tea.Cmd` functions that execute off the main goroutine and return typed messages. The Storage Layer (SQLite) is accessed by both layers: background pollers persist state, panels read it via repository interfaces. Internal packages communicate through interfaces, not direct imports, to avoid coupling and enable isolated testing.

**Major components:**
1. `cmd/devctl` — Cobra entry point; routes to TUI mode or scriptable subcommands
2. `pkg/tui` — Bubbletea root model; composes three panels (repos/worktrees, sessions/tasks, diff/agent) via sub-model composition pattern; subscribes to state channel via recursive subscription
3. `internal/dashboard` (State Manager) — orchestrates all pollers; owns the buffered event channel; distributes full `StateSnapshot` on each event
4. `internal/git` — git CLI runner; parses porcelain output; returns structured `GitState`; uses `-C <path>` for goroutine safety
5. `internal/session` — session CRUD; checkpoint write order (DB first, file second); reconciliation on startup
6. `internal/task` + `internal/dependency` — task CRUD; dependency graph computation; blocked/ready state derivation
7. `internal/agent` — Claude Code session monitoring via process/file watching
8. `pkg/storage` — SQLite repository interfaces; WAL mode; single-writer goroutine; embedded migrations

**Build order (bottom-up):** storage → git/session/task → dependency/diff/agent → dashboard state manager → TUI → cobra commands

### Critical Pitfalls

1. **Raw goroutines inside `Update()` mutating model state** — Bubbletea's model is not thread-safe; all async work must return `tea.Cmd`; enforce from day one with `-race` in CI and a code review rule prohibiting `go func()` inside `Update()`

2. **SQLite lock contention from concurrent goroutine writes** — without WAL mode + `busy_timeout` + single-writer goroutine, session checkpoints silently fail under load; set these at DB open in `storage.Open()` before any feature code touches the database

3. **Git polling thundering herd on multi-repo setups** — polling all repos simultaneously at a short interval spikes CPU and queues up git subprocesses; stagger intervals (git status: 5s per repo, idle check: 60s, agent output: 500ms), track in-flight status per repo, and use `--no-optional-locks`

4. **Session checkpoint / DB state divergence after crash** — writing checkpoint file and DB record in separate operations without a coordinator causes restore failures; always write DB record first, file second; use atomic `os.Rename()` for checkpoint files; reconcile on startup

5. **Goroutine leaks from uncancelled background pollers** — goroutines that don't check `context.Context` outlive the program in tests and cause cumulative timeout failures; use `exec.CommandContext(ctx, "git", ...)` everywhere; cancel root context on `tea.Quit`; add `goleak` to integration tests

## Implications for Roadmap

Based on the architectural dependency graph and pitfall analysis, a six-phase build is recommended. Phases 1-2 establish the foundations that everything else depends on. Phases 3-4 deliver the core product value. Phases 5-6 build the novel differentiators.

### Phase 1: Foundation — Storage, CLI Scaffold, TUI Scaffold

**Rationale:** Storage and TUI concurrency patterns must be established before any feature code is written. Retrofitting WAL mode, single-writer goroutines, or the `tea.Cmd` pattern is significantly more expensive than establishing them at the start. This phase has no user-visible output but prevents architectural debt that would require rewrites.

**Delivers:** Working binary with Cobra routing; SQLite DB with WAL mode + migration framework; Bubbletea root model with panel scaffold and correct subscription pattern; `context.Context` propagation for all background work; `-race` in CI from day one.

**Addresses:** Table stakes "fast startup" and "persistent state" infrastructure.

**Avoids:** Pitfalls 1 (goroutines in Update), 3 (SQLite locking), 9 (goroutine leaks), 12 (driver choice).

**Research flag:** Standard patterns — no additional research needed.

### Phase 2: Git Integration and Worktree Manager

**Rationale:** Git integration is the most foundational data source. Every subsequent component (sessions, tasks, dependency graph, idle detection) reads git state. Worktree management is DevCTL's core unit of work identity — it must be solid before sessions or tasks are layered on.

**Delivers:** `git worktree` CRUD via CLI subprocesses; git status polling with staggered intervals; porcelain output parsing; path normalization via `filepath.EvalSymlinks`; subprocess timeout and in-flight deduplication.

**Addresses:** Table stakes "git status per worktree," "worktree list + switch," "changed files list."

**Avoids:** Pitfalls 2 (polling thundering herd), 7 (slow git status), 14 (symlink path mismatches).

**Research flag:** Standard patterns — git CLI porcelain format and `exec.CommandContext` usage are well-documented.

### Phase 3: Dashboard TUI

**Rationale:** With storage and git working, the three-panel dashboard becomes buildable. This phase delivers DevCTL's "north star" — the single pane of glass that justifies the tool's existence. It is the visual surface on which all future features appear.

**Delivers:** Three-panel Bubbletea layout (repos/worktrees, sessions placeholder, diff/detail); `tea.WindowSizeMsg` responsive layout; keyboard navigation (Tab, hjkl, Enter, Esc); viewport-based scrollable diff panel; dirty-flag panel caching for render performance; status bar.

**Addresses:** Table stakes "dashboard," "keyboard-only navigation," "branch name visible at all times," "inline file diff."

**Avoids:** Pitfalls 4 (slow View() computation), 6 (terminal width assumptions), 11 (ANSI in non-TUI output).

**Research flag:** Standard patterns — Bubbletea sub-model composition and Lipgloss layout are well-documented. May benefit from a quick research phase for viewport component edge cases.

### Phase 4: Session Management and Context Switching

**Rationale:** Sessions are the model that connects worktrees to tasks and AI agents. Fast context switching (`devctl jump`) delivers the highest daily-use value of any single feature. This phase establishes the session model that Phases 5-6 build on.

**Delivers:** Session create/start/stop/list linked to worktrees; checkpoint files with DB-first write order and startup reconciliation; `devctl jump` fuzzy context switcher; session panel in dashboard.

**Addresses:** Table stakes "session list with status," "session attach/detach," "persistent state." Differentiator "session resurrection."

**Avoids:** Pitfall 5 (checkpoint/DB divergence), 13 (fuzzy match blocking — sync match is fine for MVP item counts).

**Research flag:** Standard patterns for session CRUD and file checkpointing. No additional research needed.

### Phase 5: Task Model and Dependency Graph

**Rationale:** The dependency graph is DevCTL's strongest competitive differentiator. It requires the task model, git integration, and session model all to be stable before it can be built reliably. Starting with explicit user-declared dependencies (not inferred) avoids false-positive trust damage.

**Delivers:** Task CRUD with status transitions; explicit dependency declarations; blocked/ready/done state derivation; dependency graph visualization in dashboard; branch ancestry detection for dependency inference (labeled "suggested," not authoritative).

**Addresses:** Differentiators "cross-repo dependency graph," "blocked task surfacing," "work planning engine."

**Avoids:** Pitfall 8 (dependency false positives — explicit first, inferred as optional).

**Research flag:** Needs research phase — dependency graph rendering in a TUI has limited prior art; graph layout algorithms for terminal displays are niche.

### Phase 6: AI Observability and Idle Agent Actions

**Rationale:** The highest-complexity, most novel features. Requires stable session model (Phase 4), task model (Phase 5), and git integration (Phase 2) all working correctly. Building last ensures the core tool has earned user trust before adding the proactive AI workflow.

**Delivers:** Claude Code session monitoring via process/file watching; live output streaming in docked dashboard panel; idle detection via absolute-timestamp comparison (not countdown); draft patch generation trigger; in-TUI patch approval workflow; reversible patch storage as git patches.

**Addresses:** Differentiators "live AI session observability," "idle branch detection + auto-actions," "draft patch approval workflow."

**Avoids:** Pitfall 10 (idle timer drift — use absolute timestamp), Pitfall 9 (goroutine leaks in file watcher).

**Research flag:** Needs research phase — Claude Code session file/process structure may have evolved since training cutoff (August 2025); file watching on macOS with `fsnotify` has known kqueue stability issues that need current verification.

### Phase Ordering Rationale

- Storage before everything: the SQLite WAL + single-writer pattern must be in place before any goroutine touches the DB.
- Git before sessions: sessions are linked to worktrees; worktrees require git integration to be authoritative.
- Dashboard before session detail: the TUI surface must exist before features can appear in it.
- Sessions before tasks: tasks reference sessions; the session model is the anchor for the task graph.
- Tasks before dependency graph: the dependency engine requires tasks to exist.
- AI observability last: the most novel, least-documented feature category; benefits most from the core being stable and trusted first.

### Research Flags

Needs deeper research during planning:
- **Phase 5 (Dependency graph):** TUI graph layout for terminal displays is a niche problem; minimal prior art in the Bubbletea ecosystem; needs research into rendering strategies.
- **Phase 6 (AI observability):** Claude Code session structure (process names, output file locations, log formats) may have evolved since August 2025 training cutoff; `fsnotify` macOS stability with kqueue needs current validation.

Standard patterns (skip research phase):
- **Phase 1 (Foundation):** SQLite WAL setup, Cobra + Bubbletea scaffold — well-documented, established patterns.
- **Phase 2 (Git integration):** `exec.CommandContext`, porcelain parsing, worktree CLI — stable, well-documented.
- **Phase 3 (Dashboard TUI):** Bubbletea sub-model composition, Lipgloss layout — core framework patterns.
- **Phase 4 (Sessions):** CRUD + checkpoint files — standard storage patterns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | MEDIUM-HIGH | Technology choices (Bubbletea, Cobra, modernc SQLite, git CLI) are HIGH confidence. Specific version pins are LOW — all versions must be verified against GitHub releases before pinning in go.mod. |
| Features | MEDIUM | Well-established tools (tmux, lazygit) landscape is HIGH confidence. AI observability feature patterns are MEDIUM — newer domain with patterns still emerging as of training cutoff. |
| Architecture | HIGH | Bubbletea MVU composition, recursive subscription, context cancellation, State Manager pattern — all well-documented with charmbracelet's own examples as reference. |
| Pitfalls | HIGH | All critical pitfalls are well-understood Go/SQLite/Bubbletea patterns documented in official sources and community. Minor pitfalls (fsnotify macOS, modernc WAL stability) are MEDIUM. |

**Overall confidence:** MEDIUM-HIGH

### Gaps to Address

- **Library versions:** All versions in STACK.md are marked for verification. Before writing go.mod, check current releases for bubbletea, lipgloss, bubbles, cobra, viper, modernc.org/sqlite, sqlx, golang-migrate. Use `go get [pkg]@latest` then pin exact tags.
- **Claude Code session structure:** For Phase 6 AI observability, the specific file paths, process names, and output formats that Claude Code uses for session state must be verified against current Claude Code behavior — not training data. This is the highest-risk unknown in the project.
- **fsnotify macOS stability:** Historical kqueue issues on macOS mean file-watching reliability needs current testing. May need a polling fallback for Phase 6 agent tracking.
- **modernc.org/sqlite WAL mode:** Community consensus favors modernc over mattn for no-CGO builds, but WAL mode support and current stability should be confirmed via pkg.go.dev before committing to the driver.
- **Go version ceiling:** Go 1.22 is the recommended floor. Check whether Go 1.23 or 1.24 features are worth targeting (toolchain directive in go.mod enables this without breaking older toolchains).

## Sources

### Primary (HIGH confidence)
- Bubbletea source and examples (charmbracelet/bubbletea) — MVU pattern, recursive subscription, Cmd/Msg architecture
- Go stdlib documentation — `os/exec`, `context`, `log/slog`, `embed`, `database/sql`
- SQLite official documentation — WAL mode, PRAGMA behavior, SQLITE_BUSY semantics
- git CLI documentation — worktree subcommands, `--porcelain` output format, `-C` flag

### Secondary (MEDIUM confidence)
- lazygit source (training data, mid-2024 observation) — focus stack pattern, RefreshHelper architecture
- charmbracelet/bubbles — viewport, list, textinput component interfaces
- charmbracelet/soft-serve — reference for Bubbletea + SQLite + Cobra production pattern
- tmux, zellij, mprocs, gitui feature sets — competitive landscape baseline (training data through August 2025)

### Tertiary (LOW confidence — verify before use)
- Specific version numbers for all dependencies — training data only; must check GitHub releases
- Claude Code session file/process structure — newer domain; training data may be stale
- fsnotify current macOS kqueue behavior — known historical issues; needs current testing
- modernc.org/sqlite current WAL mode stability — check pkg.go.dev for current version notes

---
*Research completed: 2026-03-05*
*Ready for roadmap: yes*
