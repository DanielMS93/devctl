# Phase 1: Foundation - Research

**Researched:** 2026-03-05
**Domain:** Go binary build, SQLite WAL + migrations, Bubbletea v2 TUI scaffold, background concurrency with context
**Confidence:** HIGH (architecture patterns, pitfalls) / MEDIUM (exact API shapes for v2 Charm stack) / LOW (specific version pins — verify before go.mod)

---

## Summary

Phase 1 establishes the irreversible architectural foundation for everything else: a single-binary Go CLI, a WAL-mode SQLite store with embedded migrations, and a three-panel Bubbletea TUI skeleton with correct concurrency from day one. All three must be correct before any feature code is written — retrofitting WAL mode, the single-writer goroutine, or the `tea.Cmd` pattern is significantly more expensive than establishing them at the start.

**Critical state-of-the-art finding:** The entire Charm terminal UI ecosystem (Bubbletea, Lipgloss, Bubbles) released stable v2.0.0 in February 2026. **Import paths changed** from `github.com/charmbracelet/*` to `charm.land/*/v2`. The `View()` method now returns a `tea.View` struct instead of a `string`, and key event types split into `tea.KeyPressMsg`/`tea.KeyReleaseMsg`. Start with v2 — building on v1 today means a forced migration before any feature reaches users.

The background state manager (a separate goroutine feeding the TUI via buffered channel) must be wired correctly in this phase. The recursive-subscription pattern remains idiomatic in v2: a `tea.Cmd` that blocks on the channel and re-arms itself on each event. The single-writer goroutine for SQLite must own all DB writes — concurrent `db.Exec()` from multiple goroutines without this causes silent SQLITE_BUSY failures in the checkpoint path.

**Primary recommendation:** Use the Charm v2 stack (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`), `modernc.org/sqlite` v1.46.1 (no CGO), `golang-migrate/v4` with the `sqlite` database driver and `iofs` source driver, and establish the single-writer goroutine + WAL pragmas in `storage.Open()` before anything else touches the database.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `charm.land/bubbletea/v2` | v2.0.1 | TUI event loop, Model/Update/View (Elm MVU) | Stable v2 released Feb 2026; the de facto Go TUI framework; used by gh, soft-serve, gum |
| `charm.land/lipgloss/v2` | v2.0.0 | Terminal layout and styling | First-party Bubbletea companion; same v2 release cycle |
| `charm.land/bubbles/v2` | v2.0.0 | Pre-built TUI components (viewport, list, textinput) | First-party component library; saves reimplementing scroll, list navigation |
| `github.com/spf13/cobra` | v1.9.0 | Subcommand routing, flag parsing | Dominant Go CLI framework; used by kubectl, docker; best-in-class help and shell completions |
| `modernc.org/sqlite` | v1.46.1 | Pure-Go SQLite driver, no CGO | Enables `go install` / `go build` without a C toolchain; critical for single-binary distribution |
| `github.com/jmoiron/sqlx` | v1.4.0 | Struct scanning, named queries over database/sql | Reduces boilerplate without ORM magic; direct SQL stays auditable |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | Schema migrations from embedded SQL files | Explicit, auditable schema evolution; supports `embed.FS` via `iofs` source driver |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/spf13/viper` | v1.18+ | Config file (`~/.devctl/config.toml`) + env vars | Natural Cobra companion; Cobra + Viper is the ecosystem standard pairing |
| `github.com/stretchr/testify` | v1.9+ | Test assertions (`require`, `assert`) | Ubiquitous in Go projects; reduces test boilerplate significantly |
| `log/slog` (stdlib) | Go 1.21+ | Structured logging to `~/.devctl/devctl.log` | No external dep needed; sufficient for a local CLI tool |
| `os/exec` (stdlib) | — | Git CLI subprocesses | Behavioral fidelity to real git; no CGO; `-C <path>` flag makes it goroutine-safe |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `charm.land/bubbletea/v2` | `github.com/charmbracelet/bubbletea` (v1) | v1 is not actively maintained; forced migration before shipping; start on v2 |
| `modernc.org/sqlite` | `github.com/mattn/go-sqlite3` | mattn requires CGO; breaks `go install` portability; not acceptable for this use case |
| `golang-migrate/v4` | `pressly/goose` | goose is also viable; golang-migrate is more widely used; either works with embed.FS |
| `github.com/spf13/cobra` | `github.com/urfave/cli/v2` | Cobra has better subcommand nesting and viper integration; urfave/cli is a valid but lesser choice |

### Installation

```bash
go get charm.land/bubbletea/v2@latest
go get charm.land/lipgloss/v2@latest
go get charm.land/bubbles/v2@latest
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get modernc.org/sqlite@latest
go get github.com/jmoiron/sqlx@latest
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/stretchr/testify@latest
```

After running: pin exact versions in go.mod. Verify the `modernc.org/libc` version matches the one in `modernc.org/sqlite`'s own go.mod (it requires the exact same version to avoid subtle incompatibilities).

---

## Architecture Patterns

### Recommended Project Structure

```
cmd/
└── devctl/
    └── main.go              # cobra root command, program bootstrap

internal/
├── dashboard/               # state manager, background pollers, event channel
│   └── manager.go
├── git/                     # git CLI runner, porcelain parsing (Phase 2)
├── session/                 # session CRUD, checkpoint logic (Phase 4)
└── task/                    # task CRUD (Phase 5)

pkg/
├── storage/                 # SQLite open, WAL setup, migrate, repository interfaces
│   ├── storage.go
│   └── migrations/
│       ├── 001_initial.up.sql
│       └── 001_initial.down.sql
└── tui/                     # Bubbletea root model, panels
    ├── root.go              # RootModel — composes sub-models
    ├── messages.go          # all tea.Msg types in one place
    └── panels/
        ├── left.go          # repo tree panel (RepoPanel)
        ├── right.go         # detail panel (DetailPanel)
        └── logs.go          # log/status bar (LogBar)
```

**Build order within this phase:**
1. `pkg/storage` — SQLite open + WAL pragmas + migration runner
2. `internal/dashboard` — state manager struct + buffered event channel (goroutines are stubs)
3. `pkg/tui` — root model with three-panel skeleton + recursive subscription
4. `cmd/devctl` — cobra root + `dashboard` subcommand that wires everything together

### Pattern 1: Bubbletea v2 Model Interface

**What:** Every TUI screen implements `tea.Model`. The interface in v2 is functionally identical to v1 except `View()` returns `tea.View` instead of `string`.

**Source:** pkg.go.dev/charm.land/bubbletea/v2 (verified Feb 2026)

```go
// pkg/tui/root.go
// Import path changed in v2 — use charm.land, not github.com
import tea "charm.land/bubbletea/v2"

type RootModel struct {
    width, height int
    activePanel   PanelID

    leftPanel   panels.RepoPanel
    rightPanel  panels.DetailPanel
    logBar      panels.LogBar

    stateChan <-chan StateEvent
}

func (m RootModel) Init() tea.Cmd {
    return tea.Batch(
        m.subscribeToStateEvents(),
        m.leftPanel.Init(),
        m.rightPanel.Init(),
    )
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd
    switch msg := msg.(type) {
    case tea.KeyPressMsg:            // v2: was tea.KeyMsg
        // global keys first
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "tab":
            m.activePanel = (m.activePanel + 1) % 3
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
    case StateEvent:
        m.leftPanel.SetState(msg.Snapshot)
        cmds = append(cmds, m.subscribeToStateEvents()) // re-arm
    }
    return m, tea.Batch(cmds...)
}

// v2: View returns tea.View, not string
func (m RootModel) View() tea.View {
    body := lipgloss.JoinHorizontal(
        lipgloss.Top,
        m.leftPanel.View(),
        m.rightPanel.View(),
    )
    content := lipgloss.JoinVertical(lipgloss.Left, body, m.logBar.View())
    v := tea.NewView(content)
    v.AltScreen = true        // v2: declarative, no tea.EnterAltScreen command
    return v
}
```

### Pattern 2: Recursive Subscription for Background State

**What:** A `tea.Cmd` that blocks on the event channel and re-arms itself every time an event arrives. This is the idiomatic Bubbletea pattern for subscribing to an external event source. It works identically in v2.

**Source:** charmbracelet official examples, ARCHITECTURE.md prior research (HIGH confidence)

```go
// subscribeToStateEvents blocks until the next event from the background
// state manager, returns it as a tea.Msg, and is re-armed in Update().
func (m RootModel) subscribeToStateEvents() tea.Cmd {
    return func() tea.Msg {
        return <-m.stateChan // blocks in its own goroutine; safe
    }
}

// In Update(), when StateEvent arrives:
case StateEvent:
    m.leftPanel.SetState(msg.Snapshot)
    cmds = append(cmds, m.subscribeToStateEvents()) // re-arm: exactly one goroutine blocks at a time
```

**Why this is safe:** Bubbletea executes each `tea.Cmd` in its own goroutine. When that goroutine returns a `tea.Msg`, it is delivered to `Update()` on the event loop's goroutine. No shared state is mutated outside `Update()`.

### Pattern 3: SQLite Open with WAL Pragmas

**What:** All critical SQLite configuration happens at `storage.Open()` before any caller touches the database. WAL mode, busy timeout, foreign keys, and single-writer connection are established once.

**Source:** SQLite official documentation (WAL mode), modernc.org/sqlite pkg.go.dev (driver name `"sqlite"`), prior STACK.md research

```go
// pkg/storage/storage.go
import (
    "database/sql"
    _ "modernc.org/sqlite"  // registers driver name "sqlite"
    "github.com/jmoiron/sqlx"
)

func Open(path string) (*sqlx.DB, error) {
    db, err := sqlx.Open("sqlite", path)  // driver name is "sqlite", not "sqlite3"
    if err != nil {
        return nil, err
    }

    // Single writer: SQLite is single-writer; avoid lock contention
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)

    // Critical pragmas — set before any other operation
    pragmas := []string{
        "PRAGMA journal_mode=WAL",         // enables concurrent reads + single writer
        "PRAGMA synchronous=NORMAL",       // safe with WAL; better performance than FULL
        "PRAGMA foreign_keys=ON",          // enforce FK constraints
        "PRAGMA busy_timeout=5000",        // 5s wait on lock before returning SQLITE_BUSY
    }
    for _, p := range pragmas {
        if _, err := db.Exec(p); err != nil {
            return nil, fmt.Errorf("pragma %q: %w", p, err)
        }
    }

    return db, nil
}
```

### Pattern 4: Embedded Schema Migrations with golang-migrate

**What:** SQL migration files embedded in the binary using `embed.FS`, applied on startup via `golang-migrate`. Uses the `sqlite` database driver (for modernc) and `iofs` source driver.

**CRITICAL NOTE:** The golang-migrate `sqlite3` database driver uses `mattn/go-sqlite3` (CGO). For `modernc.org/sqlite`, use the `sqlite` database driver at `github.com/golang-migrate/migrate/v4/database/sqlite`.

**Source:** golang-migrate GitHub (PR #555 adding modernc support), pkg.go.dev/golang-migrate/migrate/v4/source/iofs (verified)

```go
// pkg/storage/migrate.go
import (
    "embed"
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/sqlite"  // modernc driver, not sqlite3
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(db *sqlx.DB, dataSourceName string) error {
    d, err := iofs.New(migrationsFS, "migrations")
    if err != nil {
        return fmt.Errorf("create iofs source: %w", err)
    }

    m, err := migrate.NewWithSourceInstance("iofs", d, "sqlite://"+dataSourceName)
    if err != nil {
        return fmt.Errorf("create migrator: %w", err)
    }
    defer m.Close()

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("apply migrations: %w", err)
    }
    return nil
}
```

Migration files naming convention: `001_initial.up.sql`, `001_initial.down.sql`.

### Pattern 5: Context-Propagated Background State Manager

**What:** The state manager owns the event channel and all background goroutines. It accepts a root `context.Context` for clean shutdown.

**Source:** Standard Go concurrency idiom (HIGH confidence)

```go
// internal/dashboard/manager.go
type Manager struct {
    db     *sqlx.DB
    events chan StateEvent  // buffered, size 32
    cancel context.CancelFunc
}

func NewManager(db *sqlx.DB) *Manager {
    return &Manager{
        db:     db,
        events: make(chan StateEvent, 32),
    }
}

func (m *Manager) Start(ctx context.Context) {
    ctx, m.cancel = context.WithCancel(ctx)
    // Phase 1: stub goroutine that sends a single empty snapshot
    // to validate the channel and subscription wiring
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                m.events <- StateEvent{Snapshot: StateSnapshot{UpdatedAt: time.Now()}}
            }
        }
    }()
}

func (m *Manager) Events() <-chan StateEvent { return m.events }
func (m *Manager) Stop() { m.cancel() }
```

### Pattern 6: Cobra → Bubbletea Wiring in main.go

**What:** Cobra handles CLI routing; the `dashboard` subcommand creates and runs a Bubbletea program. Non-interactive subcommands skip Bubbletea entirely.

**Source:** Pattern from prior STACK.md research + pkg.go.dev/charm.land/bubbletea/v2 (verified)

```go
// cmd/devctl/main.go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    db, err := storage.Open(filepath.Join(os.UserHomeDir()/*...*/))
    // handle err

    if err := storage.RunMigrations(db, dbPath); err != nil {
        log.Fatal(err)
    }

    manager := dashboard.NewManager(db)
    manager.Start(ctx)
    defer manager.Stop()

    root := cobra.Command{Use: "devctl"}
    root.AddCommand(dashboardCmd(manager))
    root.Execute()
}

func dashboardCmd(mgr *dashboard.Manager) *cobra.Command {
    return &cobra.Command{
        Use:   "dashboard",
        Short: "Open the interactive dashboard",
        RunE: func(cmd *cobra.Command, args []string) error {
            m := tui.NewRootModel(mgr.Events())
            p := tea.NewProgram(m)  // v2: AltScreen is set in View(), not here
            _, err := p.Run()
            return err
        },
    }
}
```

### Anti-Patterns to Avoid

- **Raw goroutines in `Update()`:** Never `go func()` inside `Update()`. All async work must be a `tea.Cmd` (a `func() tea.Msg`). Bubbletea executes Cmds in their own goroutines and delivers results via the event loop. Violating this creates data races on the model struct.
- **`p.Start()` instead of `p.Run()`:** Removed in v2. Use `p.Run()` always.
- **`tea.EnterAltScreen` as a command:** Removed in v2. Set `v.AltScreen = true` in `View()`.
- **`tea.KeyMsg` type assertion:** Replaced in v2. Match on `tea.KeyPressMsg` (or `tea.KeyReleaseMsg`). The string `" "` (space) is now `"space"` when calling `.String()`.
- **`msg.Type` and `msg.Runes` on key events:** Replaced. Use `msg.Code` and `msg.Text` in v2.
- **Concurrent `db.Exec()` from multiple goroutines:** Without `SetMaxOpenConns(1)` and WAL mode, SQLite returns SQLITE_BUSY. Route all writes through a single goroutine or use the single-connection pattern established at `Open()`.
- **`_ "github.com/mattn/go-sqlite3"` import:** This brings in CGO. Use `_ "modernc.org/sqlite"` only. The golang-migrate database driver for modernc is `"sqlite"`, not `"sqlite3"`.
- **Hardcoded terminal widths:** All panel widths must derive from `tea.WindowSizeMsg`. Store received dimensions in the model, pass down to sub-models.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Schema migrations | Custom SQL version table + init code | `golang-migrate/migrate/v4` with `iofs` | Version tracking, dirty state handling, up/down are genuinely complex edge cases |
| SQLite driver | CGO bindings or forked driver | `modernc.org/sqlite` | Correct WAL support, maintained, CGO-free |
| TUI component list with scroll | Custom scroll state + key handling | `charm.land/bubbles/v2/list` | Mouse wheel, keyboard navigation, filtering already built and tested |
| TUI scrollable text area | Custom viewport | `charm.land/bubbles/v2/viewport` | Scroll position, mouse wheel, keyboard (PgUp/PgDn) already handled |
| Terminal style layout | String padding + ANSI codes | `charm.land/lipgloss/v2` | Color profile detection, adaptive light/dark, alignment math already done |

**Key insight:** The Charm v2 ecosystem is cohesive — bubbletea, lipgloss, and bubbles are designed as a unit. Mixing v1 and v2 components within a project causes import conflicts; use all v2.

---

## Common Pitfalls

### Pitfall 1: v1 vs v2 Charm Stack Confusion (NEW — critical)
**What goes wrong:** Import paths from training data, blog posts, and most search results reference `github.com/charmbracelet/*`. These are v1 paths. The v2 stable release (Feb 2026) uses `charm.land/*/v2`. Mixing v1 and v2 in go.mod causes compilation failures or subtle API mismatches.
**Why it happens:** v2 was in beta until very recently; all existing documentation and examples show v1 paths.
**How to avoid:** Use `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2` from the start. Do not import `github.com/charmbracelet/*` in any new code.
**Warning signs:** Any import beginning with `github.com/charmbracelet/bubbletea` or `github.com/charmbracelet/lipgloss` in new code.

### Pitfall 2: Raw Goroutines in `Update()` (architecture-fatal)
**What goes wrong:** Spawning `go func()` directly inside `Update()` bypasses Bubbletea's safety model. The model struct is accessed from multiple goroutines, producing data races. The race detector catches these, but only if tests run with `-race`.
**Why it happens:** Go's goroutine mental model is familiar; Bubbletea's Elm discipline is not obvious to newcomers.
**How to avoid:** All async work returns a `tea.Cmd` (a `func() tea.Msg`). Enable `-race` in all tests and CI runs from day one.
**Warning signs:** Any `go func()` inside `Update()`; any shared mutable state accessible from both `Update()` and a goroutine.

### Pitfall 3: SQLite SQLITE_BUSY from Concurrent Writers (data-loss risk)
**What goes wrong:** Multiple goroutines calling `db.Exec()` concurrently on the same `*sql.DB` without `SetMaxOpenConns(1)` returns SQLITE_BUSY. Checkpoint writes silently fail. Session state diverges from disk state.
**Why it happens:** `database/sql` creates a connection pool by default; SQLite is not a multi-writer database.
**How to avoid:** Set `db.SetMaxOpenConns(1)` in `storage.Open()`. Set `PRAGMA busy_timeout=5000`. Establish WAL mode. All DB writes through one goroutine.
**Warning signs:** No `SetMaxOpenConns(1)` in storage init; `PRAGMA journal_mode=WAL` not confirmed in `storage.Open()`; checkpoint code that swallows errors.

### Pitfall 4: Wrong golang-migrate SQLite Driver (CGO contamination)
**What goes wrong:** The `database/sqlite3` driver in golang-migrate imports `mattn/go-sqlite3`, which requires CGO. This silently breaks `go build` on systems without a C toolchain and defeats the no-CGO goal.
**Why it happens:** `sqlite3` is the historically dominant SQLite driver name; most tutorials reference it.
**How to avoid:** Import `github.com/golang-migrate/migrate/v4/database/sqlite` (without the `3`). This is the modernc-compatible driver.
**Warning signs:** Any `import _ "github.com/golang-migrate/migrate/v4/database/sqlite3"` in the codebase.

### Pitfall 5: Goroutine Leaks from Uncancelled Pollers
**What goes wrong:** Background goroutines that don't check `context.Context` outlive `tea.Quit`. In tests, they accumulate and cause timeouts. The TUI exits but goroutines block on channel reads or subprocess waits.
**Why it happens:** Easy to forget to thread context through every goroutine.
**How to avoid:** Every goroutine in the state manager uses `select { case <-ctx.Done(): return; ... }`. Pass `exec.CommandContext(ctx, "git", ...)` not `exec.Command(...)`. Cancel root context on shutdown.
**Warning signs:** `exec.Command(...)` instead of `exec.CommandContext(ctx, ...)` in any background goroutine.

### Pitfall 6: `db.SetMaxOpenConns(1)` Not Applied Before Migration
**What goes wrong:** If migrations run before the connection is configured with `SetMaxOpenConns(1)`, the migration itself may open multiple connections, causing lock errors on the very first startup.
**How to avoid:** Apply all connection configuration in `storage.Open()` before returning the `*sqlx.DB`. `RunMigrations()` receives the already-configured DB.

---

## Code Examples

### Bubbletea v2 Key Event Handling

```go
// Source: UPGRADE_GUIDE_V2.md + pkg.go.dev/charm.land/bubbletea/v2 (verified)
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:  // v2: NOT tea.KeyMsg
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "tab":
            m.activePanel = (m.activePanel + 1) % 3
        case "j", "down":
            // delegate to active panel...
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        // propagate to sub-models
    }
    return m, nil
}
```

### Lipgloss v2 Three-Panel Layout

```go
// Source: pkg.go.dev/charm.land/lipgloss/v2 (verified)
import "charm.land/lipgloss/v2"

func (m RootModel) View() tea.View {
    leftWidth  := m.width / 4      // 25%
    rightWidth := m.width - leftWidth

    leftContent  := lipgloss.NewStyle().Width(leftWidth).Height(m.height - 1).Render(m.leftPanel.View())
    rightContent := lipgloss.NewStyle().Width(rightWidth).Height(m.height - 1).Render(m.rightPanel.View())

    body := lipgloss.JoinHorizontal(lipgloss.Top, leftContent, rightContent)
    full := lipgloss.JoinVertical(lipgloss.Left, body, m.logBar.View())

    v := tea.NewView(full)
    v.AltScreen = true
    return v
}
```

### modernc.org/sqlite WAL Setup with sqlx

```go
// Source: pkg.go.dev/modernc.org/sqlite (verified v1.46.1, Feb 2026)
import (
    "database/sql"
    "github.com/jmoiron/sqlx"
    _ "modernc.org/sqlite"   // driver name: "sqlite"
)

func Open(dbPath string) (*sqlx.DB, error) {
    // Ensure parent directory exists
    if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
        return nil, err
    }

    db, err := sqlx.Open("sqlite", dbPath)
    if err != nil {
        return nil, err
    }

    db.SetMaxOpenConns(1)

    for _, pragma := range []string{
        "PRAGMA journal_mode=WAL",
        "PRAGMA synchronous=NORMAL",
        "PRAGMA foreign_keys=ON",
        "PRAGMA busy_timeout=5000",
    } {
        if _, err := db.Exec(pragma); err != nil {
            return nil, fmt.Errorf("storage init pragma %q: %w", pragma, err)
        }
    }

    return db, nil
}
```

### Migrations with iofs + modernc sqlite driver

```go
// Source: pkg.go.dev/github.com/golang-migrate/migrate/v4/source/iofs (verified)
// + golang-migrate PR #555 confirming modernc "sqlite" driver
import (
    "embed"
    migrate "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/sqlite"  // modernc, NOT sqlite3
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(dbPath string) error {
    src, err := iofs.New(migrationsFS, "migrations")
    if err != nil {
        return err
    }
    m, err := migrate.NewWithSourceInstance("iofs", src, "sqlite://"+dbPath)
    if err != nil {
        return err
    }
    defer m.Close()

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return err
    }
    return nil
}
```

### Goroutine-Safe Git Subprocess (for Phase 2 reference)

```go
// Source: stdlib os/exec documentation + prior STACK.md research (HIGH confidence)
// Use -C flag instead of os.Chdir — goroutine-safe
func gitStatus(ctx context.Context, repoPath string) ([]byte, error) {
    cmd := exec.CommandContext(ctx, "git",
        "-C", repoPath,
        "status", "--porcelain=v1",
        "--no-optional-locks",
    )
    return cmd.Output()
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `github.com/charmbracelet/bubbletea` | `charm.land/bubbletea/v2` | Feb 2026 (stable) | All examples/docs show old path; must use new |
| `View() string` | `View() tea.View` | Feb 2026 (v2) | View struct carries AltScreen, mouse mode declaratively |
| `tea.EnterAltScreen` command | `v.AltScreen = true` in `View()` | Feb 2026 (v2) | Old commands removed from API |
| `tea.KeyMsg` with `msg.Type`, `msg.Runes` | `tea.KeyPressMsg` with `msg.Code`, `msg.Text` | Feb 2026 (v2) | All key handling code must update |
| `p.Start()` | `p.Run()` | Feb 2026 (v2) | `Start()` removed |
| `tea.Sequentially()` | `tea.Sequence()` | Feb 2026 (v2) | Rename only |

**Deprecated/outdated (do not use):**
- `github.com/charmbracelet/bubbletea` (v1): maintenance-only; no new features
- `github.com/mattn/go-sqlite3`: requires CGO; incompatible with single-binary distribution goal
- `github.com/golang-migrate/migrate/v4/database/sqlite3`: imports mattn/go-sqlite3; use `database/sqlite` instead
- `tea.WithAltScreen()` program option: removed in v2; use `v.AltScreen = true` in `View()`

---

## Open Questions

1. **`charm.land/bubbles/v2` viewport API in v2**
   - What we know: `bubbles/v2` is stable (v2.0.0, Feb 2026); viewport component exists at `charm.land/bubbles/v2/viewport`
   - What's unclear: Whether the `viewport.Model.Update()` / `viewport.Model.View()` signatures changed from v1 (v1 returns `string`; v2 must return `tea.View` to be composable)
   - Recommendation: Check `pkg.go.dev/charm.land/bubbles/v2/viewport` before implementing the log panel; sub-model composition may require adapting viewport's `View()` output before wrapping in `tea.NewView()`

2. **golang-migrate `database/sqlite` driver import path stability**
   - What we know: PR #555 added modernc support to golang-migrate; the driver package is `github.com/golang-migrate/migrate/v4/database/sqlite`
   - What's unclear: Whether this driver is included in `v4.19.1` or requires a specific version
   - Recommendation: Run `go get github.com/golang-migrate/migrate/v4@latest` and verify `database/sqlite` package exists in the downloaded module before pinning versions

3. **`modernc.org/libc` version pinning**
   - What we know: modernc.org/sqlite's own go.mod pins an exact version of modernc.org/libc; using a different version can cause subtle issues
   - What's unclear: Whether `go get modernc.org/sqlite@latest` automatically resolves the correct libc version
   - Recommendation: After `go get`, check that `go.mod` shows modernc.org/libc at the version specified in sqlite's go.mod; run `go mod tidy` to confirm no conflicts

4. **`tea.View` struct for sub-models**
   - What we know: Root model's `View()` returns `tea.View`; sub-model panels are not `tea.Model` directly — they're composed
   - What's unclear: Whether sub-panels should return `string` (built by the parent into `tea.NewView(content)`) or also return `tea.View`
   - Recommendation: Sub-panels (RepoPanel, DetailPanel, LogBar) return `string` from their `View()` method; the root model assembles them via Lipgloss layout and wraps in `tea.NewView(full)`. This preserves composability without fighting the v2 API.

---

## Sources

### Primary (HIGH confidence)
- `pkg.go.dev/charm.land/bubbletea/v2` — current Model interface, View type, v2.0.1 published Feb 27, 2026
- `github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md` — breaking changes: KeyMsg → KeyPressMsg, View() return type, removed commands
- `pkg.go.dev/charm.land/lipgloss/v2` — v2.0.0 stable, Feb 24, 2026; JoinHorizontal/JoinVertical signatures unchanged
- `pkg.go.dev/modernc.org/sqlite` — v1.46.1, Feb 18, 2026; driver name `"sqlite"`; WAL support confirmed
- `pkg.go.dev/github.com/golang-migrate/migrate/v4/source/iofs` — iofs source driver API (stable)
- Prior project research: STACK.md, ARCHITECTURE.md, PITFALLS.md (2026-03-05, training data HIGH for patterns)

### Secondary (MEDIUM confidence)
- `github.com/charmbracelet/bubbletea/releases/tag/v2.0.0` — release announcement confirming stable status
- `pkg.go.dev/github.com/charmbracelet/bubbles/v2` — v2.0.0 stable, Feb 2026 (release date reported by WebFetch)
- `github.com/golang-migrate/migrate/pull/555` — modernc sqlite driver addition; confirms `database/sqlite` package name
- `pkg.go.dev/github.com/spf13/cobra` — v1.9.0, December 2025

### Tertiary (LOW confidence — verify before use)
- Exact version of `charm.land/bubbles/v2` viewport component API — not directly verified; assume v1 patterns hold until inspected
- `modernc.org/libc` version pinning behavior with `go get` — mentioned in modernc docs; behavior with `go mod tidy` not verified
- `golang-migrate` `database/sqlite` vs `database/sqlite3` import availability in v4.19.1 — PR #555 found but v4.19.1 release notes not inspected

---

## Metadata

**Confidence breakdown:**
- Standard stack (library choices): HIGH — Charm v2 stable release confirmed; modernc SQLite confirmed; Cobra v1.9 confirmed
- Standard stack (version pins): MEDIUM — versions verified on pkg.go.dev as of 2026-03-05; pin with `go get` then `go mod tidy`
- Architecture patterns: HIGH — Bubbletea MVU, recursive subscription, single-writer SQLite are well-documented canonical patterns
- v2 API specifics (tea.View fields, sub-model composition): MEDIUM — View() return type and AltScreen field confirmed; sub-model View() pattern inferred
- Pitfalls: HIGH — all identified pitfalls are well-documented Go/SQLite/Bubbletea patterns

**Research date:** 2026-03-05
**Valid until:** 2026-04-05 (30 days) for architecture; 2026-03-19 (14 days) for version pins (Charm releases frequently)
