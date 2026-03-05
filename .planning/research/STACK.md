# Technology Stack

**Project:** DevCTL — Terminal Developer Session Orchestrator
**Researched:** 2026-03-05
**Research Mode:** Ecosystem (Stack dimension)

> **Note on confidence:** All external research tools (WebSearch, WebFetch, Context7) were unavailable during this session. All findings come from training data (knowledge cutoff August 2025). Versions marked LOW confidence **must be verified** against official sources before pinning in go.mod.

---

## Recommended Stack

### Core TUI Framework

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `github.com/charmbracelet/bubbletea` | v1.x (verify) | TUI event loop, Model/Update/View | The de facto standard for Go TUIs. Elm architecture keeps state predictable. Mature ecosystem with first-party components (Bubbles). Used by gh, gum, many production CLIs. |
| `github.com/charmbracelet/lipgloss` | v1.x (verify) | Terminal styling, layout | First-party companion to Bubbletea. Declarative style definitions. Handles adaptive colors for light/dark terminals. |
| `github.com/charmbracelet/bubbles` | v0.20+ (verify) | Pre-built TUI components | First-party component library: viewport, list, table, textinput, textarea, progress, spinner. Avoid rebuilding these from scratch. |

**Confidence:** MEDIUM — Library choices are well-established; versions need official verification.

**Version verification:** Check https://github.com/charmbracelet/bubbletea/releases

### CLI Framework

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `github.com/spf13/cobra` | v1.8+ (verify) | Subcommand routing, flag parsing | Dominant Go CLI framework. Used by kubectl, gh, hugo, docker CLI. Best-in-class help generation, shell completion, and persistent flags. Integrates cleanly with Bubbletea — cobra handles routing, bubbletea handles interactive views. |

**Alternative considered and rejected:** `github.com/urfave/cli/v2` — Less idiomatic for subcommand-heavy CLIs. Cobra's nested command tree matches DevCTL's command structure better.

**Alternative considered and rejected:** `github.com/spf13/pflag` standalone — Too low-level without cobra's scaffolding.

**Note:** Cobra + Bubbletea is the dominant pattern for production Go developer tools that mix scriptable commands with interactive TUI modes. gh (GitHub CLI) uses this exact pattern.

**Confidence:** HIGH — Cobra is the clear ecosystem winner for this use case.

### Configuration

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `github.com/spf13/viper` | v1.18+ (verify) | Config file management | Natural cobra companion. Supports TOML/YAML/JSON config at `~/.devctl/config.toml`. Env var override support. Cobra + Viper is a standard pairing. |

**Confidence:** MEDIUM — Viper is standard; verify it's still actively maintained (it had a slow period).

### Database / Local State

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `modernc.org/sqlite` | v1.29+ (verify) | SQLite driver (pure Go) | **Recommended over mattn/go-sqlite3.** No CGO dependency means simpler cross-compilation, no need for a C toolchain in CI, easier static binaries. DevCTL is a developer tool that should `go install` cleanly. Performance is sufficient for local state (not a server). |

**Why NOT `github.com/mattn/go-sqlite3`:** Requires CGO. This makes cross-compilation painful (`GOARCH=arm64 GOOS=linux` from a Mac requires a C cross-compiler). For a developer tool distributed as a single binary, CGO is a significant operational burden.

**Companion:** `github.com/jmoiron/sqlx` v1.3+ — Thin wrapper over `database/sql` that adds struct scanning, named queries. Reduces boilerplate without the overhead of a full ORM.

**ORM considered and rejected:** GORM — Too heavy for local state. Schema evolution, raw query control, and debuggability are more important than object-relational magic for this use case.

**Migration tool:** `github.com/golang-migrate/migrate/v4` — Embed SQL migrations in the binary using `embed.FS`. Run on startup. Keeps schema evolution explicit and auditable.

**Confidence:** MEDIUM-HIGH — modernc vs mattn recommendation is well-established in the Go community; specific versions need verification.

### Git Integration

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `os/exec` (stdlib) | — | Git CLI subprocess calls | **Deliberate choice over libgit2 bindings.** Git CLI is the ground truth — output matches what engineers see in their terminals. No CGO dependency. Git's porcelain commands are stable and well-documented. Subprocesses are fast enough for local developer-tool usage. |

**Pattern for git subprocess calls:**
```go
cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain=v1")
out, err := cmd.Output()
```

Always use `-C <path>` rather than `os.Chdir` — it's goroutine-safe.

**Why NOT `github.com/go-git/go-git`:** Pure-Go git implementation. Sounds ideal but has subtle behavioral differences from the git CLI (especially around config, hooks, worktrees). For a tool that sits alongside engineer workflows, behavioral fidelity to git CLI matters more than avoiding a subprocess.

**Why NOT `libgit2` (via `git2go`):** CGO dependency, same cross-compilation problems as mattn/go-sqlite3. Not worth it.

**Confidence:** HIGH — This rationale is well-established in the Go developer tooling community.

### Worktree Management

Git worktrees are core to DevCTL's value proposition. All worktree operations go through git CLI:

```
git worktree add <path> <branch>
git worktree list --porcelain
git worktree remove <path>
git worktree prune
```

No library wraps these adequately; call them directly.

### Structured Logging

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `log/slog` (stdlib) | Go 1.21+ | Structured logging | Added to stdlib in Go 1.21. Zero external dependency. For a local CLI tool, slog to a log file (`~/.devctl/devctl.log`) is sufficient. Don't add zerolog or zap for a local tool. |

**Confidence:** HIGH — slog is stdlib since Go 1.21.

### Testing

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `testing` (stdlib) | — | Unit tests | Standard Go testing. |
| `github.com/stretchr/testify` | v1.9+ (verify) | Assertions, test suites | `require` and `assert` packages reduce test boilerplate significantly. Ecosystem standard. |

**Confidence:** HIGH — testify is ubiquitous in Go projects.

### Go Version

| Requirement | Version | Why |
|-------------|---------|-----|
| Minimum Go | 1.22+ | Range-over-integer (Go 1.22), slog (1.21), embed (1.16). 1.22 is the floor for modern idiomatic Go. |

**Confidence:** MEDIUM — Go 1.22 is the recommended floor; verify Go 1.23/1.24 are stable and whether any features are worth targeting.

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| TUI framework | Bubbletea | `tview` (rivo) | tview is immediate-mode; harder to reason about state. Bubbletea's Elm architecture is better for complex multi-pane dashboards. |
| TUI framework | Bubbletea | `termui` | Abandoned/unmaintained. |
| CLI framework | Cobra | `urfave/cli v2` | Cobra has better subcommand nesting, better ecosystem integration with viper, better help generation. |
| CLI framework | Cobra | `kong` | Less ecosystem adoption; cobra is the clear standard. |
| SQLite driver | `modernc.org/sqlite` | `mattn/go-sqlite3` | CGO requirement makes cross-compilation painful for a developer tool. |
| Git | CLI subprocess | `go-git` | Behavioral differences from real git; worktree support is incomplete. |
| Git | CLI subprocess | `libgit2/git2go` | CGO + C library dependency. |
| ORM | `sqlx` + raw SQL | GORM | GORM adds magic that makes debugging harder; raw SQL is more auditable. |
| Config | Viper | `koanf` | Viper has better cobra integration and broader ecosystem adoption. |
| Logging | `log/slog` | `zerolog`, `zap` | Stdlib is sufficient for a local tool; external deps add no value here. |

---

## Bubbletea Patterns for DevCTL

### Model/Update/View Architecture

Bubbletea enforces the Elm architecture. Every screen is a `tea.Model`:

```go
type Model struct {
    // state
}

func (m Model) Init() tea.Cmd { ... }
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { ... }
func (m Model) View() string { ... }
```

### Commands vs Messages

- **Commands** (`tea.Cmd`): functions that run outside the event loop (I/O, subprocess calls, timers). Return a `tea.Msg`.
- **Messages** (`tea.Msg`): events dispatched to `Update`. Define custom types for domain events.

```go
type gitStatusMsg struct {
    repos []RepoStatus
    err   error
}

func fetchGitStatus(paths []string) tea.Cmd {
    return func() tea.Msg {
        // run git status subprocesses
        return gitStatusMsg{repos: results}
    }
}
```

### Subscriptions / Polling

For a dashboard that auto-refreshes, use `tea.Tick`:

```go
func (m Model) Init() tea.Cmd {
    return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

### Multi-Pane Layout Pattern

For DevCTL's global dashboard, compose models: a root model holds child models (sidebar, main panel, status bar). Route messages to the active child.

```go
type RootModel struct {
    sidebar    SidebarModel
    main       MainModel
    statusBar  StatusBarModel
    activePane Pane
}
```

### Key Binding Pattern

Use `github.com/charmbracelet/bubbles/key` for declarative key maps:

```go
type keyMap struct {
    Up    key.Binding
    Down  key.Binding
    Enter key.Binding
    Quit  key.Binding
}
```

This enables help rendering via `bubbles/help` automatically.

### TUI vs CLI Mode

DevCTL will have both interactive TUI and scriptable commands. Pattern:

```go
// cobra command
var dashboardCmd = &cobra.Command{
    Use:   "dashboard",
    Short: "Open the interactive dashboard",
    RunE: func(cmd *cobra.Command, args []string) error {
        m := dashboard.NewModel(db, cfg)
        p := tea.NewProgram(m, tea.WithAltScreen())
        _, err := p.Run()
        return err
    },
}
```

Non-interactive commands (e.g., `devctl status --json`) skip Bubbletea entirely and write to stdout.

---

## SQLite Schema Patterns for DevCTL

### Recommended Schema Approach

Use `database/sql` + `sqlx` directly. Define schema in embedded SQL files:

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS
```

### WAL Mode

Always enable WAL mode for SQLite in a developer tool — it's faster for read-heavy workloads and allows concurrent reads:

```go
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
db.Exec("PRAGMA foreign_keys=ON")
```

### Connection Setup

```go
db, err := sqlx.Open("sqlite", filepath.Join(home, ".devctl", "state.db"))
db.SetMaxOpenConns(1) // SQLite is single-writer; avoid contention
```

---

## Installation

```bash
# Core TUI
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest

# CLI framework
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest

# SQLite (pure Go, no CGO)
go get modernc.org/sqlite@latest

# SQL utilities
go get github.com/jmoiron/sqlx@latest
go get github.com/golang-migrate/migrate/v4@latest

# Testing
go get github.com/stretchr/testify@latest
```

**After running:** Pin exact versions in go.mod. Replace `@latest` with verified version tags.

---

## What to NOT Use

| Library | Why Avoid |
|---------|-----------|
| `mattn/go-sqlite3` | CGO required; breaks `go install` portability |
| `go-git/go-git` | Behavioral drift from real git; poor worktree support |
| `libgit2/git2go` | CGO + C library; cross-compilation nightmare |
| GORM | Too much magic for a local state DB; makes debugging harder |
| `tview` | Immediate-mode; state management harder than Bubbletea for complex dashboards |
| `termui` | Abandoned |
| `zerolog`/`zap` | Overkill for a local CLI tool; stdlib slog is sufficient |
| `urfave/cli` | Weaker subcommand ergonomics than Cobra for this use case |

---

## Confidence Assessment

| Area | Confidence | Reason |
|------|------------|--------|
| Bubbletea/Lipgloss/Bubbles choice | HIGH | Clear ecosystem winner; used in production by major tools |
| Bubbletea/Lipgloss versions | LOW | Versions not verified; training data only; must check GitHub releases |
| Cobra as CLI framework | HIGH | Dominant Go CLI framework; overwhelming ecosystem adoption |
| Cobra version | LOW | Specific version not verified |
| modernc.org/sqlite over mattn | HIGH | Well-established recommendation in Go community for no-CGO builds |
| modernc.org/sqlite version | LOW | Specific version not verified |
| sqlx for SQL utilities | HIGH | Standard Go companion to database/sql |
| go-git rejection rationale | HIGH | Behavioral fidelity issues are well-documented in Go community |
| Git CLI subprocess pattern | HIGH | Standard pattern in Go developer tooling |
| slog for logging | HIGH | Stdlib since Go 1.21; no external dep needed |
| Go 1.22 minimum | MEDIUM | Reasonable floor; verify whether 1.23/1.24 add compelling features |

---

## Sources

> All sources are from training data (knowledge cutoff August 2025). External tools were unavailable during this research session.

**Verify the following before pinning versions:**
- https://github.com/charmbracelet/bubbletea/releases
- https://github.com/charmbracelet/lipgloss/releases
- https://github.com/charmbracelet/bubbles/releases
- https://github.com/spf13/cobra/releases
- https://pkg.go.dev/modernc.org/sqlite
- https://github.com/spf13/viper/releases
- https://github.com/jmoiron/sqlx/releases
- https://github.com/golang-migrate/migrate/releases
- https://go.dev/doc/devel/release (for current Go version)

**Reference projects using this stack:**
- `github.com/cli/cli` (gh) — Cobra + custom TUI components
- `github.com/charmbracelet/soft-serve` — Bubbletea + SQLite + Cobra
- `github.com/jesseduffield/lazygit` — Go TUI for git (different framework but same git-CLI-over-subprocess pattern)
