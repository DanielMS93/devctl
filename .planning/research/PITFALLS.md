# Domain Pitfalls

**Domain:** Go CLI/TUI developer session orchestrator (Bubbletea + SQLite + Git CLI)
**Project:** DevCTL
**Researched:** 2026-03-05
**Confidence note:** Web/Context7 tools unavailable in this session. All findings are from training data (cutoff August 2025) and direct Go/Bubbletea/SQLite ecosystem knowledge. Confidence levels reflect this. Critical claims flagged for validation against official sources before implementation.

---

## Critical Pitfalls

Mistakes that cause rewrites or fundamental architectural breakdowns.

---

### Pitfall 1: Firing Goroutines from `Update()` that Write Back via Channels to `Update()`

**What goes wrong:** Developers familiar with Go concurrency naturally reach for goroutines inside `Update()`. But Bubbletea's model is that `Update()` must be pure and fast — it returns `(Model, tea.Cmd)`. If you spawn goroutines that write back to the model directly (via shared state or direct mutation), you race the runtime. The correct pattern is `tea.Cmd` (a function returning `tea.Msg`), which Bubbletea executes in its own goroutine and channels the result back to `Update()` safely. Bypassing this with raw goroutines causes data races on the model and undefined rendering behavior.

**Why it happens:** Go's goroutine mental model is "just go func() it." Bubbletea's Elm architecture requires a different discipline. New users don't realize Bubbletea itself handles goroutine execution for Cmds.

**Consequences:**
- Silent data races (Go race detector catches these but only if tests run with `-race`)
- Model state corrupted mid-render
- Intermittent, hard-to-reproduce UI glitches
- Potential panic in the render loop

**Prevention:**
- All background work must return `tea.Cmd` (a `func() tea.Msg`)
- Use `tea.Batch()` for multiple concurrent Cmds
- Never mutate model fields from outside `Update()`
- Enable `-race` flag in all tests and CI runs from day one
- Add a linter rule or code review checklist: "no goroutines spawned from Update()"

**Warning signs:**
- Any `go func()` inside `Update()`
- Shared mutable state accessible from both `Update()` and background work
- Race detector findings in TUI-related tests

**Phase to address:** Foundation phase (TUI scaffold). Establish the Cmd pattern in the first working screen before any background polling is added.

**Confidence:** HIGH — core Elm architecture constraint, well-documented in Bubbletea README and examples.

---

### Pitfall 2: Polling Git State in a Tight Loop from `tick` Commands

**What goes wrong:** The natural way to keep git status fresh is a `tea.Every()` or `tea.Tick()` that fires `git status --porcelain` on each repo. With one repo this is fine. With 10+ repos and worktrees, each tick spawns multiple `exec.Command("git", ...)` calls. These fork-exec cycles are expensive: `git status` on a large repo takes 50-300ms. At a 1-second tick interval, 10 repos = 10 subprocesses launched per second. At 500ms, it doubles. The TUI becomes sluggish, CPU spikes, and on slower machines the next tick fires before the previous one finishes, queuing up a backlog of Cmds.

**Why it happens:** The polling model works great for simple cases. The cost only becomes visible with realistic repo counts.

**Consequences:**
- CPU spike visible to the user
- TUI frame rate degrades as Update() queue backs up
- Git commands compete for filesystem locks (especially on macOS where file events are expensive)
- On networked filesystems (NFS, cloud sync overlays), the tool becomes unusable

**Prevention:**
- Use a **staggered polling** approach: don't poll all repos at once; round-robin with a delay between each
- Use **filesystem watchers** (`fsnotify`) for change detection instead of polling; fall back to polling only as backup
- Debounce: don't start a new git poll for a repo until the previous one completes (track in-flight status per repo)
- Set a minimum poll interval floor (e.g., 5 seconds per repo, not 1 second global)
- Run git commands with `--no-optional-locks` to avoid lock contention
- Cache results; only refresh if `mtime` of `.git/index` changed

**Warning signs:**
- `exec.Command("git", ...)` calls not guarded by an in-flight tracker
- Tick interval below 3 seconds for multi-repo setups
- No cap on concurrent git subprocesses

**Phase to address:** Git integration phase. The concurrency strategy must be designed before the polling loop is wired up, not after.

**Confidence:** HIGH — standard subprocess performance problem, well understood in Go tooling.

---

### Pitfall 3: SQLite "database is locked" Errors from Multiple Goroutines

**What goes wrong:** SQLite in Go (via `mattn/go-sqlite3` or `modernc.org/sqlite`) defaults to one writer at a time. When multiple goroutines call `db.Exec()` concurrently without connection pool coordination, SQLite returns `SQLITE_BUSY` or `SQLITE_LOCKED`. This is especially dangerous for session checkpointing: a crash-resilience feature that silently fails is worse than no crash resilience at all.

**Why it happens:** Go's `database/sql` pool creates multiple connections. SQLite in WAL mode allows one writer + many readers concurrently, but requires proper configuration. Without `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout`, and `SetMaxOpenConns(1)` on the writer connection, concurrent writes deadlock or return errors.

**Consequences:**
- Session checkpoints silently fail during heavy background activity
- Session state diverges from DB state after a crash
- Intermittent `database is locked` errors in logs that get ignored
- In the worst case: DB corruption if a crash interrupts a transaction during a write conflict

**Prevention:**
- Set `PRAGMA journal_mode=WAL` on database open
- Set `PRAGMA busy_timeout=5000` (5 second busy wait before returning error)
- **Use a single writer goroutine** with a channel-based write queue — all DB writes are serialized through one goroutine, reads use separate read-only connections
- Set `db.SetMaxOpenConns(1)` on the write connection explicitly
- Wrap all checkpoint writes in explicit transactions with rollback on error
- Test with `-race` and concurrent goroutine write tests

**Warning signs:**
- Multiple goroutines holding a reference to the same `*sql.DB` and calling Exec/Insert concurrently
- No `PRAGMA journal_mode=WAL` in DB initialization
- Checkpoint code that swallows errors (logs and continues)

**Phase to address:** Storage foundation phase. Set WAL mode, busy timeout, and the writer-goroutine pattern before any feature code touches the DB. Retrofitting concurrency patterns is painful.

**Confidence:** HIGH — SQLite concurrency constraints are well-documented; the WAL + single-writer pattern is the established Go solution.

---

### Pitfall 4: Bubbletea Render Blocking on Slow `View()` Computation

**What goes wrong:** `View()` is called synchronously on every model update, in the same goroutine as `Update()`. If `View()` does any non-trivial computation (string formatting of many repos, dependency graph layout, diff rendering), the render loop stalls. At 30fps the render budget per frame is ~33ms. Complex Lipgloss layouts with many styles applied to large strings easily exceed this.

**Why it happens:** `View()` looks like a simple string-returning function. Developers put logic there that should have been precomputed in `Update()`.

**Consequences:**
- Visible frame rate drop
- Key input feels laggy (inputs queue behind slow renders)
- Dashboard with 20+ repos becomes noticeably sluggish

**Prevention:**
- Compute expensive strings in `Update()`, store as model fields, reference in `View()`
- Cache rendered subcomponents: only re-render a panel's string when its data actually changed (use a dirty flag per panel)
- Profile with `go tool pprof` early; add a render time assertion in tests
- For the diff viewer specifically, pre-render the colored diff string when the diff data arrives (via Cmd), not in `View()`

**Warning signs:**
- Any function call in `View()` that isn't string concatenation or Lipgloss `.Render()`
- `View()` method longer than ~50 lines
- No caching of sub-panel rendered strings

**Phase to address:** Dashboard layout phase. Establish the precompute-in-Update pattern when building the first multi-panel layout.

**Confidence:** HIGH — Elm architecture constraint; widely discussed in Bubbletea community.

---

### Pitfall 5: Session Checkpoint Files and DB State Diverging After Crash

**What goes wrong:** DevCTL checkpoints sessions to `~/.devctl/sessions/` files separately from the SQLite DB. The intention is crash resilience. But if the checkpoint file is written after the DB record but the process crashes between these two writes, restore logic sees a checkpoint file for a session the DB doesn't know about (or vice versa). The restore flow fails silently or panics on nil lookups.

**Why it happens:** Two-phase write without a coordinator. The file system and SQLite are two separate stores with no transactional relationship.

**Consequences:**
- `devctl session restore <id>` fails or corrupts state after a crash
- Developer loses context they were relying on
- Trust in the tool evaporates after first failure

**Prevention:**
- **Write to DB first**, then write checkpoint file. On restore, DB is authoritative; file is supplementary data.
- Store a `checkpoint_path` and `checkpoint_written_at` column in the sessions table. If they don't match an existing file, the checkpoint is considered missing (degrade gracefully, don't panic).
- On startup, run a reconciliation pass: scan checkpoint files, cross-reference with DB, log discrepancies without crashing.
- Use atomic file writes: write to a `.tmp` file then `os.Rename()` (rename is atomic on POSIX). This prevents partial checkpoint files.
- Test crash scenarios explicitly: write a test that kills the process mid-checkpoint and verifies restore behavior.

**Warning signs:**
- Checkpoint write order not documented or enforced
- No startup reconciliation step
- Checkpoint write using `os.WriteFile()` directly (not atomic rename)
- Error from checkpoint write being logged-and-ignored

**Phase to address:** Session management phase. Define the write order and reconciliation strategy before implementing checkpoint logic. Do not add crash resilience as an afterthought.

**Confidence:** HIGH — classic two-phase write problem, well understood in storage systems.

---

## Moderate Pitfalls

---

### Pitfall 6: Terminal Width/Height Assumptions Breaking Layouts

**What goes wrong:** Lipgloss layout code that hardcodes widths or assumes a minimum terminal size breaks badly when users run on 80-column terminals, in tmux splits, or in terminal emulators that report unusual dimensions. The dashboard's three-panel layout (repos/sessions left, Claude output right) is particularly vulnerable: if the terminal is narrower than the layout expects, panels overlap or render garbage characters.

**Why it happens:** Layouts are often developed on a single developer's terminal. The edge cases only appear when users have different setups.

**Prevention:**
- Always use `tea.WindowSizeMsg` to get terminal dimensions; never hardcode widths
- In `Update()`, handle `tea.WindowSizeMsg` and recompute all panel widths proportionally
- Define minimum viable dimensions and render a "terminal too small" message below the threshold
- Test in tmux with a split pane, in the default macOS Terminal.app, and in iTerm2
- Lipgloss `Width()` and `Height()` methods should use model-stored dimensions, not constants

**Warning signs:**
- Any hardcoded integer used as a panel width
- Layout not updated in `case tea.WindowSizeMsg:` handler

**Phase to address:** Dashboard layout phase (first multi-panel screen).

**Confidence:** HIGH — standard TUI layout problem.

---

### Pitfall 7: `git status` on Repos with Many Files is Slow Without `--short`

**What goes wrong:** `git status` without flags on a repo with thousands of files and many changes produces large output and takes longer due to formatting. `git status --porcelain` (machine-readable) is consistently faster and easier to parse, but even this can be slow on repos with many untracked files if `.gitignore` isn't optimized.

**Prevention:**
- Always use `git status --porcelain` (or `--porcelain=v2`)
- Add `--untracked-files=no` for repos where untracked file count is irrelevant to the display (opt-in per repo)
- Consider `git diff --name-only --cached` and `git diff --name-only` separately for staged/unstaged detection (faster for targeted queries)
- Set a timeout on each git subprocess (e.g., 10 seconds) and surface a "timeout" indicator rather than hanging

**Warning signs:**
- Using `git status` without `--porcelain`
- No subprocess timeout set on `exec.Command`
- Parsing git output with fragile string splitting instead of structured porcelain format

**Phase to address:** Git integration phase.

**Confidence:** HIGH — git CLI behavior, well documented.

---

### Pitfall 8: Dependency Detection False Positives from Branch Name Heuristics

**What goes wrong:** Inferring task dependencies from git branch names (e.g., `feature/auth` depends on `feature/base`) or file overlap is inherently heuristic. False positives (showing a task as blocked when it isn't) are more damaging than false negatives (missing a real dependency). A developer seeing phantom blockers will distrust the dependency view and stop using it.

**Prevention:**
- Prefer **explicit** dependency declaration (user-defined in task metadata) over inferred dependencies
- If inferring, surface inferred dependencies as "suggested" with a distinct visual indicator, not as authoritative blockers
- Provide a simple "dismiss this suggestion" action so users can correct false positives without penalty
- Start with explicit-only in MVP; add inference as an opt-in feature only after the core is trusted

**Warning signs:**
- Branch name parsing used as the primary dependency signal
- No way for users to override or dismiss inferred dependencies
- Inferred and explicit dependencies displayed identically

**Phase to address:** Dependency tracking phase.

**Confidence:** HIGH — product design principle, validated by common developer tooling failure modes.

---

### Pitfall 9: Goroutine Leaks from Uncancelled Background Pollers

**What goes wrong:** Background goroutines launched for git polling, AI session observation, or idle detection that don't respect a `context.Context` cancellation will outlive the program's intent. In Bubbletea, when the user quits (`tea.Quit`), the program exits — but goroutines that are blocked on `exec.Cmd.Wait()` or a `time.Sleep` continue running until the OS kills the process. In tests, this causes goroutine leaks that accumulate across test runs, eventually causing test timeouts.

**Prevention:**
- Every background Cmd function must accept and check a `context.Context`
- Use `exec.CommandContext(ctx, "git", ...)` so git subprocesses are killed when context is cancelled
- On `tea.Quit`, issue a global context cancellation before the program exits
- Use `goleak` in tests to detect goroutine leaks: `defer goleak.VerifyNone(t)` in integration tests
- Maintain a `WaitGroup` for all background work so shutdown can confirm all goroutines have exited

**Warning signs:**
- `exec.Command(...)` instead of `exec.CommandContext(ctx, ...)` in any background work
- No global cancellation signal on program exit
- Test suite that never runs `goleak`

**Phase to address:** Foundation phase (before any polling is added). The context propagation pattern must be established at the scaffold stage.

**Confidence:** HIGH — standard Go context/goroutine pattern.

---

### Pitfall 10: Idle Detection Drift Over Long Sessions

**What goes wrong:** Implementing idle detection with `time.After()` or `time.Tick()` inside Bubbletea Cmds that chain to themselves (the common "tick loop" pattern) accumulates drift over long sessions. Each tick is scheduled relative to when the previous one completed, not relative to a wall-clock anchor. Over hours, the 20-minute idle threshold becomes inaccurate. Additionally, if the machine sleeps/hibernates, the tick resumes from where it left off, meaning a machine asleep for 2 hours might trigger idle detection immediately on wake (or never, depending on implementation).

**Prevention:**
- Store the **absolute timestamp** of last activity in model state, not a countdown
- On each tick (which can be coarse, e.g., every 30 seconds), check `time.Since(lastActivityAt) > idleThreshold`
- This is immune to tick drift and handles sleep/wake correctly
- Update `lastActivityAt` on any user keypress, mouse event, or detected file change
- Test by manually setting `lastActivityAt` in tests rather than waiting real time

**Warning signs:**
- Idle detection implemented as a countdown (`remainingSeconds--`)
- `time.After(20 * time.Minute)` used as the sole idle signal
- No handling of system sleep/wake events

**Phase to address:** Idle detection and agent action phase.

**Confidence:** HIGH — standard time handling pitfall in long-running processes.

---

## Minor Pitfalls

---

### Pitfall 11: Lipgloss ANSI Codes Breaking Non-ANSI Terminal Output

**What goes wrong:** Lipgloss applies ANSI escape codes for colors and styles. Piped output, non-interactive terminals, or terminals with `TERM=dumb` receive raw escape codes, producing garbage. This affects `devctl` subcommands that are meant to be scripted (e.g., `devctl session list` piped to another tool).

**Prevention:**
- Detect `os.Getenv("TERM") == "dumb"` or `!term.IsTerminal(os.Stdout.Fd())` and disable styles
- Lipgloss's `lipgloss.ColorProfile()` handles this automatically when used correctly — ensure the profile is set from `termenv.ColorProfile()` at startup, not hardcoded
- TUI (`devctl dashboard`) is always interactive; non-TUI subcommands should output plain text by default with `--color` flag to force color

**Warning signs:**
- Lipgloss styles applied unconditionally in non-TUI command output
- No `isTerminal` check in CLI output functions

**Phase to address:** CLI command structure phase (before adding styled output to non-TUI commands).

**Confidence:** HIGH — standard terminal compatibility issue.

---

### Pitfall 12: SQLite `modernc.org/sqlite` vs `mattn/go-sqlite3` CGO Mismatch

**What goes wrong:** `mattn/go-sqlite3` requires CGO, which complicates cross-compilation and CI environments without a C toolchain. `modernc.org/sqlite` is pure Go (no CGO) and preferred for DevCTL's "easy cross-platform binary" goal. However, the two drivers have subtle behavioral differences in error types, connection string parameters, and DSN syntax. Teams that start with one and switch later face non-trivial migration work.

**Prevention:**
- Decide at project start: use `modernc.org/sqlite` for CGO-free builds
- Encapsulate all DB initialization (pragma setup, connection string) in a single `storage.Open()` function so driver-specific details are in one place
- Document the driver choice in the storage package README

**Warning signs:**
- `import _ "github.com/mattn/go-sqlite3"` in any file (signals CGO dependency crept in)
- DB initialization code scattered across multiple packages

**Phase to address:** Storage foundation phase.

**Confidence:** MEDIUM — both drivers are well-known; modernc preference for CGO-free is community consensus but verify current modernc stability before committing.

---

### Pitfall 13: Fuzzy Selector (`devctl jump`) Blocking the TUI Event Loop

**What goes wrong:** `devctl jump` as a fast context switcher needs to display a fuzzy-find overlay and filter results as the user types. If the fuzzy matching algorithm runs synchronously in `Update()` on a large dataset (many worktrees, sessions, branches), it introduces latency between keypress and visible filter update. Users expect sub-16ms response.

**Prevention:**
- For the initial MVP with <100 items, synchronous fuzzy match in `Update()` is fine — don't over-engineer early
- If item count grows, move matching to a Cmd that runs asynchronously; debounce input so matching only triggers after 50ms of no typing
- Use a well-tested fuzzy match library (`sahilm/fuzzy` or `junegunn/fzf` algorithm) rather than implementing from scratch

**Warning signs:**
- Custom fuzzy match implementation
- Fuzzy match running on every keypress without debounce when item count is large

**Phase to address:** Fast context switching phase.

**Confidence:** MEDIUM — performance threshold depends on dataset size, which is unknown for MVP.

---

### Pitfall 14: Worktree Path Resolution Errors Across Symlinks

**What goes wrong:** `git worktree list` outputs absolute paths. On macOS, `/private/var` is symlinked to `/var`, and similar symlink structures exist in various setups. If DevCTL stores the path from `git worktree list` output and later uses it for `chdir` or file operations, path comparisons fail when one path goes through the symlink and another doesn't (e.g., paths from `os.Getwd()` vs paths from git output).

**Prevention:**
- Normalize all paths through `filepath.EvalSymlinks()` before storing in DB or comparing
- Apply normalization consistently at ingestion time (when reading from git), not at comparison time
- Add a test that constructs a path via symlink and verifies equality with the resolved path

**Warning signs:**
- Storing raw output from `git worktree list` without path normalization
- Path equality checks using string comparison instead of `filepath.EvalSymlinks`

**Phase to address:** Git integration phase (worktree management).

**Confidence:** MEDIUM — macOS-specific symlink issue; confirm current macOS behavior in testing.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| TUI scaffold / foundation | Raw goroutines in Update() (Pitfall 1) | Enforce tea.Cmd pattern before any async work |
| Storage foundation | SQLite locked errors (Pitfall 3), driver choice (Pitfall 12) | WAL mode + single-writer goroutine at DB open |
| Git integration | Subprocess performance (Pitfall 2, 7), symlink paths (Pitfall 14) | Staggered polling, porcelain flags, path normalization |
| Dashboard layout | View() slowness (Pitfall 4), terminal size (Pitfall 6) | Precompute in Update(), handle WindowSizeMsg |
| Session management | Checkpoint divergence (Pitfall 5) | DB-first write order, atomic file writes, reconcile on startup |
| Context switching (jump) | Fuzzy match blocking (Pitfall 13) | Sync match is fine for MVP; add debounce if item count grows |
| Idle detection | Timer drift (Pitfall 10), goroutine leaks (Pitfall 9) | Absolute timestamp comparison, context cancellation |
| Dependency tracking | False positives (Pitfall 8) | Explicit dependencies first, inferred as "suggested" only |
| CLI non-TUI commands | ANSI in piped output (Pitfall 11) | isTerminal check before applying Lipgloss styles |

---

## Sources

**Confidence note:** Web search and WebFetch tools were unavailable during this research session. All findings draw on training data through August 2025, covering:

- Bubbletea GitHub source and examples (charmbracelet/bubbletea)
- Lipgloss documentation (charmbracelet/lipgloss)
- SQLite WAL mode documentation (sqlite.org)
- Go `database/sql` documentation (pkg.go.dev)
- Go `exec.CommandContext` patterns (pkg.go.dev/os/exec)
- `goleak` goroutine leak detection (go.uber.org/goleak)
- `modernc.org/sqlite` vs `mattn/go-sqlite3` community discussions
- Standard Go concurrency patterns (The Go Blog)

**Validate before implementing (LOW-to-MEDIUM confidence items):**
- `modernc.org/sqlite` current stability and WAL mode support — check pkg.go.dev for current version
- Bubbletea current Cmd batching API (may have changed since training data) — check charmbracelet/bubbletea releases
- `fsnotify` current macOS stability — known historical issues with kqueue on macOS
