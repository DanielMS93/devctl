# Architecture Patterns

**Domain:** Go CLI/TUI developer session orchestrator
**Researched:** 2026-03-05
**Confidence:** HIGH (Bubbletea MVU pattern, Go concurrency idioms) / MEDIUM (cross-component wiring specifics)

---

## Recommended Architecture

DevCTL has two distinct runtime layers: a **background state layer** that continuously collects data from external systems (git, filesystem, AI sessions), and a **TUI presentation layer** that renders that state and handles user input. These layers are coupled through a single message bus: Go channels feeding Bubbletea `tea.Cmd` subscriptions.

```
┌─────────────────────────────────────────────────────────┐
│                     cmd/devctl                          │
│              (entrypoint, cobra routing)                │
└───────────────────────┬─────────────────────────────────┘
                        │ starts
        ┌───────────────┴───────────────┐
        │                               │
        ▼                               ▼
┌───────────────────┐        ┌──────────────────────┐
│  State Manager    │        │   TUI Root Model      │
│  (background)     │        │   (Bubbletea)         │
│                   │        │                       │
│  ┌─────────────┐  │        │  ┌────────────────┐  │
│  │ GitPoller   │  │        │  │  PanelLeft     │  │
│  │ SessionWatcher│ │        │  │  (repos+trees) │  │
│  │ AgentTracker│  │        │  ├────────────────┤  │
│  │ IdleDetector│  │        │  │  PanelCenter   │  │
│  └──────┬──────┘  │        │  │  (sessions+   │  │
│         │ events  │        │  │   tasks)       │  │
│         ▼         │        │  ├────────────────┤  │
│  chan StateEvent ─┼────────┼─▶│  PanelRight    │  │
└───────────────────┘        │  │  (diff/agent)  │  │
                             │  └────────────────┘  │
┌───────────────────┐        │  ┌────────────────┐  │
│  Storage Layer    │◀───────┼─▶│  StatusBar     │  │
│  (SQLite)         │        │  └────────────────┘  │
│  ~/.devctl/       │        └──────────────────────┘
│  state.db         │
└───────────────────┘
```

---

## Component Boundaries

| Component | Package | Responsibility | Communicates With |
|-----------|---------|---------------|-------------------|
| **Entry Point** | `cmd/devctl` | CLI routing (cobra), program bootstrap, config load | Instantiates all other components |
| **Root TUI Model** | `pkg/tui` | Top-level Bubbletea model, panel layout, key routing, subscription loop | All panel sub-models, State Manager (via channel) |
| **Panel Sub-Models** | `pkg/tui/panels` | Individual Bubbletea sub-models for left/center/right panes | Root model (child relationship), Storage Layer (read) |
| **State Manager** | `internal/dashboard` | Orchestrates all background pollers, owns the event channel, maintains in-memory snapshot | All pollers, Storage Layer, TUI (via channel) |
| **Git Integration** | `internal/git` | Runs git CLI commands, parses output, returns structured `GitState` | State Manager (called on poll tick), Storage Layer |
| **Session Manager** | `internal/session` | Creates/reads/updates session records, writes checkpoint files | Storage Layer, State Manager |
| **Task Manager** | `internal/task` | CRUD for tasks, task status transitions | Storage Layer, Dependency Engine |
| **Dependency Engine** | `internal/dependency` | Computes dependency graph, detects blocked tasks, evaluates readiness | Task Manager, Git Integration |
| **Agent Orchestrator** | `internal/agent` | Monitors Claude Code sessions (process/file watching), captures output streams | State Manager, Storage Layer |
| **Diff Engine** | `internal/diff` | Runs `git diff` variants, produces structured diff output for rendering | Git Integration, TUI panel (on demand) |
| **Storage Layer** | `pkg/storage` | SQLite read/write via `database/sql`, schema migrations | All components that need persistence |

---

## Data Flow

### Background State Collection (write path)

```
External World                State Manager              Storage
─────────────                 ─────────────              ───────
git repos/worktrees ──poll──▶ GitPoller
                              │ GitState{}
                              ▼
AI process filesystem ──────▶ AgentTracker
                              │ AgentEvent{}
                              ▼
idle timer (20 min) ────────▶ IdleDetector
                              │ IdleEvent{}
                              ▼
                         StateEvent{} ──persist──▶ SQLite
                              │
                              ▼
                         chan StateEvent (buffered)
                              │
                              ▼ (Bubbletea subscription)
                         TUI Root Model.Update()
                              │
                              ▼
                         Panel sub-model state updates
                              │
                              ▼
                         View() re-render
```

### User Action (read/command path)

```
Keyboard input
      │
      ▼
TUI Root Model.Update(KeyMsg)
      │ routes to active panel
      ▼
Panel.Update(KeyMsg)
      │ returns tea.Cmd
      ▼
tea.Cmd executes (off main goroutine)
      │ calls internal/* package
      ▼
Storage / Git / Session / Task / Dependency
      │ returns result msg
      ▼
Root Model.Update(ResultMsg)
      │ updates model state
      ▼
View() re-render
```

---

## Bubbletea Structure for a Multi-Panel Complex TUI

### The Core Insight: Composition, Not Inheritance

Bubbletea's MVU (Model/Update/View) pattern scales to complex UIs by **composing sub-models**, not by putting all state in one giant struct. Each panel is its own Bubbletea model that satisfies the `tea.Model` interface.

**Confidence: HIGH** — this is the canonical pattern used by charmbracelet/wish, charmbracelet's own examples, and tools like Soft Serve.

```go
// pkg/tui/root.go
type RootModel struct {
    // Layout
    width, height int
    activePanel   PanelID

    // Sub-models (each is a tea.Model)
    leftPanel   panels.RepoPanel
    centerPanel panels.SessionPanel
    rightPanel  panels.DiffPanel
    statusBar   panels.StatusBar

    // State snapshot from background layer
    state       StateSnapshot
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Route to active panel; collect returned cmd
        cmd := m.routeKeyToActivePanel(msg)
        cmds = append(cmds, cmd)

    case StateEvent:
        // Background poller pushed a new state
        m.state = msg.Snapshot
        // Propagate to panels that need it
        m.leftPanel, _ = m.leftPanel.Update(msg)
        m.centerPanel, _ = m.centerPanel.Update(msg)

    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        cmds = append(cmds, m.relayout())
    }

    return m, tea.Batch(cmds...)
}
```

### Subscription Pattern for Background Events

Bubbletea provides `tea.Program.Send()` for external goroutines to inject messages, and `Init()` returns a `tea.Cmd` that can start a subscription loop.

```go
// pkg/tui/root.go
func (m RootModel) Init() tea.Cmd {
    return tea.Batch(
        m.subscribeToStateEvents(),  // starts the goroutine listener
        m.leftPanel.Init(),
        m.centerPanel.Init(),
    )
}

// subscribeToStateEvents returns a tea.Cmd that blocks waiting for the next
// event from the background state manager, then returns it as a message.
// The returned message triggers another call to subscribeToStateEvents, creating
// a continuous subscription loop.
func (m RootModel) subscribeToStateEvents() tea.Cmd {
    return func() tea.Msg {
        event := <-m.stateChan  // blocks until next event
        return event            // returned as tea.Msg to Update()
    }
}

// In Update(), when a StateEvent arrives, re-subscribe:
case StateEvent:
    m.state = msg.Snapshot
    return m, m.subscribeToStateEvents()  // re-arm the subscription
```

This is the **"recursive subscription" pattern** — safe, idiomatic, and used in charmbracelet's own examples. It ensures exactly one goroutine is listening at any time.

### Panel Focus and Key Routing

```go
type PanelID int

const (
    PanelLeft   PanelID = iota  // repos + worktrees
    PanelCenter                 // sessions + tasks
    PanelRight                  // diff / agent output
)

func (m *RootModel) routeKeyToActivePanel(msg tea.KeyMsg) tea.Cmd {
    // Global keys handled first
    switch msg.String() {
    case "q", "ctrl+c":
        return tea.Quit
    case "tab":
        m.activePanel = (m.activePanel + 1) % 3
        return nil
    }

    // Delegate to active panel
    switch m.activePanel {
    case PanelLeft:
        updated, cmd := m.leftPanel.Update(msg)
        m.leftPanel = updated.(panels.RepoPanel)
        return cmd
    case PanelCenter:
        updated, cmd := m.centerPanel.Update(msg)
        m.centerPanel = updated.(panels.SessionPanel)
        return cmd
    case PanelRight:
        updated, cmd := m.rightPanel.Update(msg)
        m.rightPanel = updated.(panels.DiffPanel)
        return cmd
    }
    return nil
}
```

### View Composition with Lipgloss

```go
func (m RootModel) View() string {
    leftContent  := m.leftPanel.View()
    centerContent := m.centerPanel.View()
    rightContent  := m.rightPanel.View()

    // Lipgloss joins panels horizontally, then appends status bar
    body := lipgloss.JoinHorizontal(
        lipgloss.Top,
        panelStyle(m.activePanel == PanelLeft).Render(leftContent),
        panelStyle(m.activePanel == PanelCenter).Render(centerContent),
        panelStyle(m.activePanel == PanelRight).Render(rightContent),
    )

    return lipgloss.JoinVertical(lipgloss.Left, body, m.statusBar.View())
}
```

**Width allocation pattern:** pass `tea.WindowSizeMsg` down to all sub-models so each computes its own width. Avoid hardcoding column widths.

---

## Background State Manager Architecture

### Structure

```go
// internal/dashboard/manager.go
type Manager struct {
    db       *storage.DB
    events   chan StateEvent     // buffered (size ~32), written by pollers
    snapshot StateSnapshot      // protected by sync.RWMutex
    mu       sync.RWMutex
    pollers  []Poller
}

type StateEvent struct {
    Type     EventType
    Snapshot StateSnapshot      // full snapshot (cheap to copy for small datasets)
    // OR delta fields for high-frequency updates
}

type StateSnapshot struct {
    Repos      []RepoState
    Worktrees  []WorktreeState
    Sessions   []SessionState
    Tasks      []TaskState
    AgentSessions []AgentState
    UpdatedAt  time.Time
}
```

### Poller Interface

```go
type Poller interface {
    Start(ctx context.Context, out chan<- StateEvent)
    Stop()
}
```

Each poller runs in its own goroutine, uses `context.Context` for cancellation, and writes to the shared event channel. The Manager starts all pollers on `Start()` and stops them via context cancellation on shutdown.

### GitPoller Implementation Pattern

```go
// internal/git/poller.go
type GitPoller struct {
    repos  []string
    ticker *time.Ticker
}

func (p *GitPoller) Start(ctx context.Context, out chan<- StateEvent) {
    go func() {
        p.ticker = time.NewTicker(5 * time.Second)
        defer p.ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-p.ticker.C:
                states, err := p.collectAll()
                if err != nil {
                    continue  // log error, don't crash
                }
                out <- StateEvent{Type: EventGitRefresh, GitStates: states}
            }
        }
    }()
}
```

### Idle Detection Pattern

```go
// internal/dashboard/idle.go
type IdleDetector struct {
    threshold   time.Duration   // 20 min default
    lastActivity map[string]time.Time  // worktree -> last git activity
    mu          sync.Mutex
}

// Called by GitPoller when it sees a change
func (d *IdleDetector) RecordActivity(worktree string) {
    d.mu.Lock()
    d.lastActivity[worktree] = time.Now()
    d.mu.Unlock()
}

// Background goroutine checks thresholds, emits IdleEvent
func (d *IdleDetector) Watch(ctx context.Context, out chan<- StateEvent) {
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                d.mu.Lock()
                for wt, last := range d.lastActivity {
                    if time.Since(last) >= d.threshold {
                        out <- StateEvent{Type: EventIdle, Worktree: wt}
                        delete(d.lastActivity, wt)  // fire once per idle period
                    }
                }
                d.mu.Unlock()
            }
        }
    }()
}
```

---

## Patterns to Follow

### Pattern 1: Thin Commands, Fat Internal Packages

`tea.Cmd` functions should be thin — they call into `internal/*` packages and return a message. Business logic lives in `internal/*`, not in `Update()`.

```go
// Good: thin cmd wrapper
func loadWorktreesCmd(repo string, db *storage.DB) tea.Cmd {
    return func() tea.Msg {
        wts, err := git.ListWorktrees(repo)
        if err != nil {
            return ErrorMsg{Err: err}
        }
        return WorktreesLoadedMsg{Worktrees: wts}
    }
}
```

### Pattern 2: Full Snapshot Distribution (for DevCTL's scale)

For a single-developer local tool with ~10-50 repos, distributing a full `StateSnapshot` on every event is simpler and correct. Delta updates add complexity without meaningful performance benefit at this scale.

### Pattern 3: Context Cancellation for Goroutine Lifecycle

Every background goroutine accepts `context.Context`. The root context is cancelled on `tea.Quit`, which propagates shutdown to all pollers cleanly.

```go
// cmd/devctl/main.go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

manager := dashboard.NewManager(db)
manager.Start(ctx)  // starts all pollers

p := tea.NewProgram(tui.NewRootModel(manager.Events(), ...))
```

### Pattern 4: Message Types as Discriminated Union

Define all messages in `pkg/tui/messages.go` or in each sub-package. Use type assertions in `Update()` — not string switching.

```go
type StateEvent struct { Snapshot StateSnapshot }
type WorktreesLoadedMsg struct { Worktrees []git.Worktree }
type ErrorMsg struct { Err error }
type IdleDetectedMsg struct { Worktree string }
type AgentOutputMsg struct { SessionID string; Lines []string }
```

### Pattern 5: Viewport Component for Scrollable Content

Use `github.com/charmbracelet/bubbles/viewport` for diff output, agent log streaming, and file viewers. It handles scroll state, mouse wheel, and keyboard navigation internally.

```go
import "github.com/charmbracelet/bubbles/viewport"

type DiffPanel struct {
    viewport viewport.Model
    content  string
}

func (p DiffPanel) Update(msg tea.Msg) (DiffPanel, tea.Cmd) {
    var cmd tea.Cmd
    p.viewport, cmd = p.viewport.Update(msg)
    return p, cmd
}
```

### Pattern 6: Storage Layer as Pure Repository

The `pkg/storage` package exposes typed repository interfaces (not raw SQL). Business logic does not construct queries.

```go
type SessionRepository interface {
    Create(s Session) (int64, error)
    Get(id int64) (Session, error)
    ListActive() ([]Session, error)
    UpdateCheckpoint(id int64, path string) error
}
```

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Single Monolithic Model

**What:** One giant `AppModel` struct with all state, one enormous `Update()` switch.
**Why bad:** Unnavigable at DevCTL's complexity level (8 sub-systems). Every keypress touches all state. Impossible to test panels in isolation.
**Instead:** Compose sub-models. Each panel is `tea.Model`. Root model delegates.

### Anti-Pattern 2: Direct State Mutation from Goroutines

**What:** Background goroutines directly mutating fields on the model struct.
**Why bad:** Bubbletea's model is not thread-safe. Races corrupt display state.
**Instead:** Background goroutines write to a buffered channel; the Bubbletea event loop reads the channel via subscription and applies updates in `Update()` (single goroutine).

### Anti-Pattern 3: Blocking Operations in Update()

**What:** Running `git status` or a DB query synchronously inside `Update()`.
**Why bad:** Blocks the TUI event loop. UI freezes during the operation.
**Instead:** Return a `tea.Cmd` that runs the operation off the main goroutine and sends back a result message.

```go
// Bad
case selectWorktreeMsg:
    wts, _ := git.ListWorktrees(repo)  // BLOCKS
    m.worktrees = wts

// Good
case selectWorktreeMsg:
    return m, loadWorktreesCmd(repo, m.db)  // async

case WorktreesLoadedMsg:
    m.worktrees = msg.Worktrees
```

### Anti-Pattern 4: Unbuffered State Channel

**What:** Using `make(chan StateEvent)` (unbuffered) for the state manager event channel.
**Why bad:** If the TUI is busy rendering, pollers block and miss their next tick, causing cascading latency.
**Instead:** Buffered channel: `make(chan StateEvent, 32)`. Pollers can write without waiting. TUI drains at its own pace.

### Anti-Pattern 5: Polling Everything at the Same Interval

**What:** All pollers tick at the same rate (e.g., 5 seconds).
**Why bad:** Thundering herd — all goroutines wake simultaneously, hit git CLI and DB at once.
**Instead:** Stagger intervals. Git status: 5s. Idle check: 60s. Agent output: 500ms. Dependency graph: 30s.

### Anti-Pattern 6: Tight Coupling Between internal/ Packages

**What:** `internal/session` importing `internal/git` directly.
**Why bad:** Creates import cycles; makes testing harder; changes in git ripple everywhere.
**Instead:** Use interfaces. `internal/session` depends on a `GitReader` interface, not the concrete `internal/git` package. The State Manager wires them together.

---

## Component Build Order

Dependencies flow from bottom to top. Build in this order:

```
Layer 0 (no dependencies):
  pkg/storage — SQLite schema, migrations, repository interfaces

Layer 1 (depends on storage only):
  internal/git     — git CLI runner + state parser
  internal/session — session CRUD
  internal/task    — task CRUD

Layer 2 (depends on Layer 1):
  internal/dependency — dependency graph (uses task + git)
  internal/diff       — diff engine (uses git)
  internal/agent      — agent tracker (uses session + storage)

Layer 3 (depends on Layer 2):
  internal/dashboard  — state manager, pollers, idle detector

Layer 4 (depends on Layer 3):
  pkg/tui             — Bubbletea root model, panels, subscriptions

Layer 5 (depends on Layer 4):
  cmd/devctl          — cobra commands, program bootstrap
```

**Build order rationale:**
- Storage layer must exist before anything can persist.
- Git integration is the most foundational data source — all other components read git state.
- Dependency engine needs task + git to compute the graph.
- Dashboard/state manager must exist before TUI because TUI subscribes to its event channel.
- TUI comes last because it depends on everything providing clean interfaces.

---

## Reference Architecture: How lazygit Does It

**Confidence: MEDIUM** (based on source observation as of mid-2024; verify if critical)

lazygit uses a custom GUI framework (not Bubbletea), but its component boundaries are instructive:

- **Gui struct** — root model, owns all panels and state
- **Context stack** — tracks which panel has focus; each context handles its own keybindings
- **RefreshHelper** — background goroutine that calls `git status`, `git log`, etc., then posts to a refresh channel
- **State** — in-memory struct updated from refresh results; panels read from state on render

The key lesson: **focus management as a stack**, not a flat enum. When a modal opens, it pushes onto the stack. When dismissed, it pops. This generalises better than `activePanel PanelID` for DevCTL's diff/agent split-pane use case.

**DevCTL adaptation:** Implement a `FocusStack []PanelID` on `RootModel`. The status bar shows the active context. `Tab` rotates through primary panels; `Enter` on a worktree pushes the diff panel onto the stack; `Esc` pops it.

---

## Scalability Considerations

| Concern | At MVP (1-5 repos) | At Growth (10-30 repos) | At Scale (100+ repos) |
|---------|-------------------|------------------------|----------------------|
| Git polling | Synchronous serial poll, 5s tick | Parallel goroutine per repo | File watch (fsnotify) replaces polling |
| State snapshot size | Full copy per event, fine | Full copy still fine (<1MB) | Delta events + incremental rendering |
| SQLite writes | Synchronous writes fine | WAL mode, batch writes | Still fine; SQLite handles 100 repos easily |
| Rendering | Full re-render on state change | Selective panel updates | Viewport virtualization for long lists |
| Agent session tracking | Poll process list | inotify on session dirs | Same approach, just more goroutines |

**Immediate action (MVP):** Design the `StateSnapshot` struct to be copy-safe (no pointers to shared mutable state). This makes the scale-up to delta events purely additive — no redesign needed.

---

## Sources

- Bubbletea architecture and MVU pattern: training data (HIGH confidence — stable since v0.20, design is fundamental to the framework)
- Recursive subscription pattern (`func() tea.Msg` re-arming): charmbracelet official examples (HIGH confidence)
- `bubbles/viewport` for scrollable content: charmbracelet bubbles library (HIGH confidence)
- lazygit component model: source code observation (MEDIUM confidence — may have evolved)
- mprocs multi-pane structure: training data (MEDIUM confidence)
- Go context cancellation for goroutine lifecycle: standard Go idiom (HIGH confidence)
- SQLite WAL mode for concurrent reads: SQLite official documentation (HIGH confidence)
- Thundering herd / staggered polling: distributed systems common knowledge (HIGH confidence as a pattern)
