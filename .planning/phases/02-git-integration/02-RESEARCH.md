# Phase 2: Git Integration - Research

**Researched:** 2026-03-05
**Domain:** Git worktree management, git state polling, TUI diff/preview rendering, editor integration
**Confidence:** HIGH (all key APIs verified against source code or official docs)

## Summary

Phase 2 adds real git integration to devctl: worktree CRUD via CLI subcommands, live git state polling per worktree (ahead/behind, staged/unstaged/untracked counts, changed files), and an inline viewer with syntax-highlighted file preview and colored diff display. The existing architecture (Manager goroutine -> buffered channel -> Bubbletea v2 TUI) is well-suited to this work. The git state work goes in the Manager's `pollLoop`, the viewer goes in a new overlay model within the TUI, and worktree CRUD goes into new `cobra` subcommands.

**Primary recommendation:** Use git CLI subprocesses for all git operations (not go-git; it lacks linked worktree support). Use `charm.land/bubbles/v2` viewport for scrolling, `github.com/alecthomas/chroma/v2` for syntax highlighting, and `tea.ExecProcess` for editor launch.

The biggest cross-cutting decision is the StateSnapshot expansion: the current `tuimsg.StateSnapshot` holds only `UpdatedAt`. Phase 2 replaces it with a rich snapshot containing per-worktree git state, driving everything the TUI renders. All expansions flow through that single struct.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| git CLI subprocess | system | All git operations (worktree, status, rev-list, diff) | go-git lacks linked worktree support; git CLI is stable and featureful |
| `charm.land/bubbles/v2` | v2.0.0 (already in go.mod) | viewport scroll model for diff/preview | Official Charm component; v2 API matches bubbletea v2 |
| `github.com/alecthomas/chroma/v2` | v2.23.1 | Syntax highlighting to ANSI for file preview | Pure Go, 250+ languages, terminal formatters built in |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `os/exec` (stdlib) | - | Run git subprocesses | Every git operation |
| `strings`, `bytes` (stdlib) | - | Parse git porcelain output | Parsing `git status --porcelain=v2`, `git worktree list --porcelain` |
| `tea.ExecProcess` | bubbletea v2 | Launch editor, suspend/resume TUI | Opening files in $EDITOR from viewer |
| `github.com/google/uuid` | v1.6.0 (already in go.mod indirect) | IDs for new worktree records | Already transitive dep |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| git CLI subprocess | go-git v5 | go-git has no linked worktree (git worktree add) support; subprocess is correct choice |
| chroma/v2 | `bat` subprocess | bat is nicer for end-users but adds a runtime dep; chroma is pure Go, no subprocess needed |
| chroma/v2 | lipgloss manual styling | Chroma handles 250+ languages and token-level coloring; manual is impractical |

**Installation (new deps only):**
```bash
go get github.com/alecthomas/chroma/v2
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── git/
│   ├── worktree.go        # git worktree list/add/remove subprocess wrappers
│   ├── state.go           # per-worktree state: ahead/behind, staged/unstaged counts
│   ├── diff.go            # git diff output (unstaged, staged, branch vs main, vs origin)
│   └── git.go             # shared Run() helper: exec.Cmd + output capture + error wrap
├── dashboard/
│   ├── manager.go         # existing; pollLoop expanded to call git package
│   └── poller.go          # (new) per-worktree polling goroutine, or fan-out from manager
pkg/
├── tui/
│   ├── tuimsg/
│   │   └── messages.go    # StateSnapshot expanded with WorktreeState slice
│   ├── panels/
│   │   ├── left.go        # existing; now renders repo+worktree list with git badges
│   │   ├── right.go       # existing; now renders changed files list
│   │   └── viewer.go      # (new) inline viewer model: file preview + diff; overlays right panel
│   └── root.go            # existing; routes keys to viewer overlay when open
cmd/devctl/
└── worktree.go            # cobra worktree subcommand: list, create, delete
pkg/storage/migrations/
└── 002_git_phase.up.sql   # worktree_state cache table + repo_config table
```

### Pattern 1: Git Subprocess Wrapper
**What:** A thin `internal/git` package runs `exec.CommandContext` calls and returns parsed structs. No raw strings leak outside the package.
**When to use:** Every git operation.
**Example:**
```go
// internal/git/git.go
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil {
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {
            return nil, fmt.Errorf("git %s: %w\nstderr: %s", args[0], err, exitErr.Stderr)
        }
        return nil, fmt.Errorf("git %s: %w", args[0], err)
    }
    return out, nil
}
```

### Pattern 2: git worktree list --porcelain Parsing
**What:** Parse the stanza-based porcelain format. Each worktree is a block of key-value lines separated by a blank line.
**When to use:** `devctl worktree list` and polling for known worktrees.

Porcelain output format:
```
worktree /path/to/main
HEAD abc123...
branch refs/heads/main

worktree /path/to/feature-branch
HEAD def456...
branch refs/heads/feature/add-login
locked reason text here
```

Fields per stanza: `worktree` (path, always first), `HEAD` (sha), `branch` (full ref like `refs/heads/name`), `bare` (boolean label, present only if true), `detached` (boolean label), `locked` (label or `locked <reason>`), `prunable` (label or `prunable <reason>`).

Parsing approach: split on `\n\n`, then split each stanza on `\n`, read `key value` pairs. Strip `refs/heads/` prefix to get short branch name.

```go
// internal/git/worktree.go
type Worktree struct {
    Path    string
    Head    string
    Branch  string // short name, empty if detached
    Bare    bool
    Locked  bool
    Prunable bool
}

func ListWorktrees(ctx context.Context, repoPath string) ([]Worktree, error) {
    out, err := run(ctx, repoPath, "worktree", "list", "--porcelain")
    // parse stanzas...
}
```

### Pattern 3: git status --porcelain=v2 Parsing for Counts
**What:** Use `git status --porcelain=v2 --branch` to get both branch info (ahead/behind) and file status in one subprocess call per worktree.
**When to use:** Polling per-worktree git state every N seconds.

Output lines:
- `# branch.ab +N -N` — present only when upstream tracking exists; N = ahead count, N = behind count
- `1 XY ...` — ordinary changed entry; X = staged status char, Y = unstaged status char
- `2 XY ...` — renamed/copied entry
- `? path` — untracked file
- `! path` — ignored file (only with `--ignored`)

Status characters: `.` = unmodified, `M` = modified, `A` = added, `D` = deleted, `R` = renamed, `C` = copied, `U` = unmerged.

Counting rules:
- **staged**: count lines `1 XY` or `2 XY` where X != `.` and X != `?`
- **unstaged**: count lines `1 XY` or `2 XY` where Y != `.` and Y != `?`
- **untracked**: count lines starting with `?`

Ahead/behind: parse `# branch.ab +A -B` → ahead=A, behind=B. If line absent, no upstream (ahead=0, behind=-1 or sentinel).

```go
// internal/git/state.go
type WorktreeState struct {
    WorktreePath string
    Branch       string
    Ahead        int
    Behind       int    // -1 = no upstream tracking
    Staged       int
    Unstaged     int
    Untracked    int
    ChangedFiles []ChangedFile
}

type ChangedFile struct {
    Path           string
    StagedStatus   byte // 'M', 'A', 'D', 'R', 'C', '.' etc.
    UnstagedStatus byte
}

func PollState(ctx context.Context, worktreePath string) (WorktreeState, error) {
    out, err := run(ctx, worktreePath, "status", "--porcelain=v2", "--branch")
    // parse...
}
```

### Pattern 4: Ahead/Behind via rev-list (fallback)
When polling without tracking upstream, or for branch-vs-main comparison:
```bash
# Ahead of main:
git rev-list --count main..HEAD

# Behind main:
git rev-list --count HEAD..main

# Both at once (requires local tracking ref):
git rev-list --left-right --count HEAD...@{u}
# Output: "<ahead>\t<behind>"
```

Check if upstream exists first: `git rev-parse --abbrev-ref @{u}` returns non-zero exit if no upstream.

### Pattern 5: Diff Display via Raw ANSI from git
**What:** Capture `git diff --color=always` output and pass the raw ANSI string to the viewport. No custom diff renderer needed.
**When to use:** All diff modes (unstaged, staged, branch vs main, branch vs origin).

```go
// internal/git/diff.go
type DiffMode int

const (
    DiffUnstaged DiffMode = iota   // git diff HEAD
    DiffStaged                      // git diff --cached HEAD
    DiffVsMain                      // git diff main...HEAD
    DiffVsOrigin                    // git diff origin/main...HEAD
)

func Diff(ctx context.Context, worktreePath, filePath string, mode DiffMode) (string, error) {
    // Build args based on mode, add filePath if non-empty (single-file diff)
    // Use --color=always to force ANSI even when stdout is not a TTY
    out, err := run(ctx, worktreePath, args...)
    return string(out), err
}
```

Note: `git diff --color=always` outputs ANSI escape codes even when piped. The viewport's `SetContent()` strips ANSI for width measurement (`ansi.StringWidth`) but preserves the escape codes in the rendered string. This works correctly.

### Pattern 6: Syntax Highlighting with Chroma
**What:** Use chroma/v2's `quick.Highlight` to convert file content to ANSI string, then feed to viewport.
**When to use:** File preview in the inline viewer.

```go
// pkg/tui/panels/viewer.go (within the highlight helper)
import (
    "strings"
    "github.com/alecthomas/chroma/v2/quick"
)

func highlightFile(content, filename string) (string, error) {
    var sb strings.Builder
    // Detect lexer from filename; "terminal16m" = 24-bit truecolor
    // Fall back to "terminal256" for wider compat
    err := quick.Highlight(&sb, content, filename, "terminal256", "monokai")
    if err != nil {
        return content, nil // graceful degradation: return plain text
    }
    return sb.String(), nil
}
```

Formatter names (verified from chroma source):
- `"terminal"` / `"terminal8"` — 8-color
- `"terminal16"` — 16-color
- `"terminal256"` — 256-color (recommended default)
- `"terminal16m"` — 24-bit truecolor

Style names: `"monokai"`, `"dracula"`, `"github"`, `"nord"` (all built in).

The `quick.Highlight` function signature:
```go
func Highlight(w io.Writer, source, lexer, formatter, style string) error
```
Where `lexer` can be a language name or filename (chroma detects by extension when given a filename path, not just the content).

### Pattern 7: Viewport for Scrolling Content
**What:** `charm.land/bubbles/v2` viewport.Model wraps any string content with vertical and horizontal scrolling.
**When to use:** Both file preview and diff display in the inline viewer.

```go
// pkg/tui/panels/viewer.go
import "charm.land/bubbles/v2/viewport"

type ViewerModel struct {
    viewport  viewport.Model
    mode      ViewerMode // Preview | DiffUnstaged | DiffStaged | ...
    filePath  string
    width     int
    height    int
    visible   bool
}

// In Init/setup:
vp := viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))
vp.SetContent(highlightedContent) // or diff ANSI string

// In Update, pass msgs to viewport:
func (m ViewerModel) Update(msg tea.Msg) (ViewerModel, tea.Cmd) {
    m.viewport, _ = m.viewport.Update(msg)
    // handle 'e' key -> tea.ExecProcess(editor)
    // handle 'q' / Esc -> close viewer
    return m, nil
}

// In View:
func (m ViewerModel) View() string {
    return m.viewport.View()
}
```

Viewport key defaults (KeyPressMsg v2): up/down = scroll 1 line, pgup/pgdn = scroll page, `g`/`G` = top/bottom. These come from `viewport.DefaultKeyMap()` and are handled inside `viewport.Update()`.

Important: viewport.View() returns a `string`, not `tea.View`. The outer model's `View()` method must return `tea.View` (v2 API). Compose by embedding viewport.View() output into the outer View string, then wrap with `tea.NewView(...)`.

### Pattern 8: Editor Launch via tea.ExecProcess
**What:** `tea.ExecProcess` pauses the TUI, gives terminal control to the editor, then resumes.
**When to use:** User presses `e` in the inline viewer.

```go
// pkg/tui/panels/viewer.go
import (
    "os"
    "os/exec"
    tea "charm.land/bubbletea/v2"
)

type EditorFinishedMsg struct{ err error }

func openInEditor(filePath string) tea.Cmd {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = os.Getenv("VISUAL")
    }
    if editor == "" {
        editor = "vi" // last-resort default
    }
    c := exec.Command(editor, filePath)
    return tea.ExecProcess(c, func(err error) tea.Msg {
        return EditorFinishedMsg{err: err}
    })
}
```

`tea.ExecProcess` calls `p.releaseTerminal(false)` before running and `p.RestoreTerminal()` after. Terminal state is fully restored. No `tea.Suspend()` needed — ExecProcess handles everything. (Verified from bubbletea v2 source at `/Users/daniel/go/pkg/mod/charm.land/bubbletea/v2@v2.0.1/exec.go`.)

### Pattern 9: Polling Architecture in Manager
**What:** Expand `pollLoop` in `internal/dashboard/manager.go` to poll each tracked worktree.
**When to use:** The Manager already owns the goroutine lifecycle and channel.

Two viable approaches:
1. **Single loop** (simpler): One goroutine polls all worktrees sequentially every 5s. Good for <10 worktrees.
2. **Per-worktree goroutines** (scalable): One goroutine per worktree with a shared fan-in to `m.events`. Better if worktree count is large.

For Phase 2, use approach 1 (single loop). Add per-worktree goroutines in a later phase if needed.

The StateSnapshot must be expanded:
```go
// pkg/tui/tuimsg/messages.go
type StateSnapshot struct {
    UpdatedAt  time.Time
    Worktrees  []WorktreeState  // from internal/git package (via alias or copy)
}
```

The `tuimsg` leaf package must not import `internal/git` (would create a layering violation). Options:
- Define a parallel `WorktreeState` struct in `tuimsg` and copy data in Manager
- Or define it in a shared `pkg/gitstate` package that both `internal/git` and `tuimsg` can import

Recommended: define `WorktreeState` in `tuimsg` (it's already a leaf/shared package), have `internal/git` return its own struct, and have Manager map git struct -> tuimsg struct. This keeps `tuimsg` free of git subprocess dependencies.

### Anti-Patterns to Avoid
- **Spawning goroutines in Update():** All async git operations are tea.Cmd, never raw goroutines in the TUI.
- **Blocking git subprocess in View():** Never call `exec.Command` in View(). Compute content in a tea.Cmd, store result in model, render from stored result.
- **Polling too aggressively:** 5-second interval is the Phase 1 default; git status is cheap but running it on 20 worktrees every 1s will cause noticeable CPU load. Keep at 5-10s.
- **go-git for worktree management:** go-git v5's `Worktree` type is for single-repo working trees, not for managing multiple linked worktrees. Use git CLI.
- **`git diff` without `--color=always`:** When stdout is not a TTY (subprocess), git strips ANSI by default. Must pass `--color=always`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Scrollable content viewer | Custom scroll logic with offset tracking | `charm.land/bubbles/v2/viewport` | Handles ANSI-aware line width, horizontal scroll, mouse wheel, page up/down |
| Syntax highlighting | Token-by-token ANSI coloring | `github.com/alecthomas/chroma/v2` | 250+ language lexers, multiple styles, handles edge cases (unicode, multi-line tokens) |
| Editor subprocess + terminal restore | Manual tcsetattr / raw mode manipulation | `tea.ExecProcess` | Bubbletea handles terminal release/restore atomically; manual attempts corrupt terminal state |
| Git status parsing | Custom diff/status format | `git status --porcelain=v2` | Machine-stable format, explicitly versioned, handles all edge cases including renames |
| Ahead/behind count | Walking rev tree | `git rev-list --left-right --count` | git does this efficiently with bitmap indices |

**Key insight:** The terminal state restoration problem when launching editors from TUIs is extremely easy to get wrong. `tea.ExecProcess` is the only safe path in bubbletea v2.

## Common Pitfalls

### Pitfall 1: git subprocess in wrong working directory
**What goes wrong:** Git status, diff, and rev-list are all relative to CWD. Running with wrong dir gives wrong results or errors.
**Why it happens:** `exec.Command` uses process CWD by default.
**How to avoid:** Always set `cmd.Dir = worktreePath` in the shared `run()` helper.
**Warning signs:** Status returns empty even for a worktree with changes.

### Pitfall 2: --color=always not passed to git diff
**What goes wrong:** git diff output has no ANSI colors when captured via subprocess.
**Why it happens:** git detects non-TTY stdout and strips colors by default.
**How to avoid:** Always pass `--color=always` to `git diff`.
**Warning signs:** Diff displays in viewport but no red/green coloring.

### Pitfall 3: Porcelain v2 branch.ab line only present with upstream
**What goes wrong:** `# branch.ab` line is absent when no upstream tracking branch is configured.
**Why it happens:** git omits the line entirely rather than showing `+0 -0`.
**How to avoid:** Check if the line exists before parsing. If absent, set Behind = -1 (sentinel for "no upstream").
**Warning signs:** Panic on missing field, or always showing 0/0 ahead/behind.

### Pitfall 4: viewport.View() is a string, outer View() must return tea.View
**What goes wrong:** Compilation error or type mismatch if you try to return viewport.View() directly from the bubbletea model's View().
**Why it happens:** In bubbletea v2, the top-level model's View() returns `tea.View`, but embedded components (viewport, list etc.) still return `string`.
**How to avoid:** Always wrap: `return tea.NewView(lipgloss.JoinVertical(..., m.viewport.View()))` at the top-level. Sub-models return `string`.
**Warning signs:** Compile error: `cannot use string as tea.View`.

### Pitfall 5: Worktree path copying for .env files must handle non-existent source
**What goes wrong:** Copy panics or errors if the source file (e.g., `.env`) doesn't exist in the source worktree.
**Why it happens:** Not all repos have every file in the copy list.
**How to avoid:** Check if source file exists before copying. Skip silently if absent.
**Warning signs:** `devctl worktree create` fails for any repo without all configured copy-list files.

### Pitfall 6: go-git has no linked worktree support
**What goes wrong:** Using `go-git v5`'s `Worktree` type attempts to operate on the single working tree of a repository; it cannot manage multiple linked worktrees.
**Why it happens:** go-git's Worktree represents the working copy, not the git worktree concept.
**How to avoid:** Use git CLI subprocesses for all worktree operations. Verified: go-git v5 pkg docs list no `worktree add` equivalent.
**Warning signs:** No `AddLinked()` or similar method exists in go-git's API.

### Pitfall 7: chroma lexer detection by filename vs language name
**What goes wrong:** `quick.Highlight(w, content, "main.go", ...)` detects language from filename extension correctly. `quick.Highlight(w, content, "go", ...)` uses the language name directly. Mixing these can cause unexpected behavior.
**Why it happens:** Chroma's lexer resolution handles both filenames and language names but by different code paths.
**How to avoid:** Pass the filename (with extension) as the lexer argument for automatic detection.
**Warning signs:** Entire file displayed with no syntax highlighting despite being a supported language.

## Code Examples

Verified patterns from source code inspection:

### git worktree list --porcelain Parsing
```go
// Source: git-scm.com/docs/git-worktree (official docs)
func parseWorktrees(porcelain []byte) []Worktree {
    var results []Worktree
    stanzas := bytes.Split(bytes.TrimSpace(porcelain), []byte("\n\n"))
    for _, stanza := range stanzas {
        var wt Worktree
        for _, line := range bytes.Split(stanza, []byte("\n")) {
            parts := bytes.SplitN(line, []byte(" "), 2)
            switch string(parts[0]) {
            case "worktree":
                wt.Path = string(parts[1])
            case "HEAD":
                wt.Head = string(parts[1])
            case "branch":
                ref := string(parts[1])
                wt.Branch = strings.TrimPrefix(ref, "refs/heads/")
            case "bare":
                wt.Bare = true
            case "locked":
                wt.Locked = true
            case "prunable":
                wt.Prunable = true
            }
        }
        if wt.Path != "" {
            results = append(results, wt)
        }
    }
    return results
}
```

### tea.ExecProcess for Editor Launch
```go
// Source: /Users/daniel/go/pkg/mod/charm.land/bubbletea/v2@v2.0.1/exec.go (verified)
type EditorFinishedMsg struct{ err error }

func openInEditor(filePath string) tea.Cmd {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "vi"
    }
    return tea.ExecProcess(exec.Command(editor, filePath), func(err error) tea.Msg {
        return EditorFinishedMsg{err: err}
    })
}

// In Update():
case tea.KeyPressMsg:
    if msg.String() == "e" && m.viewer.visible {
        return m, openInEditor(m.viewer.filePath)
    }
case EditorFinishedMsg:
    // Bubbletea has already restored terminal; just update state if needed
```

### viewport.Model Setup and Usage
```go
// Source: /Users/daniel/go/pkg/mod/charm.land/bubbles/v2@v2.0.0/viewport/viewport.go (verified)
import "charm.land/bubbles/v2/viewport"

// Create:
vp := viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))
vp.SetContent(ansiString) // accepts ANSI-escaped content; correct width measurement

// In Update():
vp, cmd = vp.Update(msg)  // handles KeyPressMsg for scroll, MouseWheelMsg

// In View() of sub-component (returns string, not tea.View):
return vp.View()

// Viewport supports:
// - SoftWrap bool
// - MouseWheelEnabled bool
// - LeftGutterFunc for line numbers
// - SetHighlights([][]int) for search result highlighting
```

### Chroma Syntax Highlighting
```go
// Source: pkg.go.dev/github.com/alecthomas/chroma/v2/quick (verified)
import (
    "strings"
    "github.com/alecthomas/chroma/v2/quick"
)

func highlightFile(content, filename string) string {
    var sb strings.Builder
    // filename used for lexer detection by extension
    // "terminal256" = 256-color ANSI; "monokai" = dark theme
    err := quick.Highlight(&sb, content, filename, "terminal256", "monokai")
    if err != nil {
        return content // graceful degradation
    }
    return sb.String()
}
```

### git status --porcelain=v2 Counting
```go
// Source: git-scm.com/docs/git-status (official docs), verified with live test
func parseStatus(out []byte) (ahead, behind, staged, unstaged, untracked int) {
    behind = -1 // sentinel: no upstream
    for _, line := range bytes.Split(out, []byte("\n")) {
        if len(line) == 0 {
            continue
        }
        switch {
        case bytes.HasPrefix(line, []byte("# branch.ab ")):
            fmt.Sscanf(string(line), "# branch.ab +%d -%d", &ahead, &behind)
        case line[0] == '1' || line[0] == '2':
            // XY where X=staged col, Y=unstaged col
            if len(line) > 3 {
                if line[2] != '.' { staged++ }
                if line[3] != '.' { unstaged++ }
            }
        case line[0] == '?':
            untracked++
        }
    }
    return
}
```

## Schema Additions Needed

Two new migrations are required for Phase 2.

### Migration 002: worktree_state cache table
Stores the last-polled git state per worktree. Prevents stale display on startup before first poll completes.

```sql
-- 002_git_phase.up.sql
CREATE TABLE IF NOT EXISTS worktree_state (
    worktree_id   TEXT PRIMARY KEY REFERENCES worktrees(id) ON DELETE CASCADE,
    branch        TEXT NOT NULL DEFAULT '',
    ahead         INTEGER NOT NULL DEFAULT 0,
    behind        INTEGER NOT NULL DEFAULT -1, -- -1 = no upstream tracking
    staged        INTEGER NOT NULL DEFAULT 0,
    unstaged      INTEGER NOT NULL DEFAULT 0,
    untracked     INTEGER NOT NULL DEFAULT 0,
    polled_at     INTEGER NOT NULL DEFAULT 0   -- Unix timestamp
);

-- repo-level config: files to copy when creating a new worktree
CREATE TABLE IF NOT EXISTS repo_config (
    repo_id          TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    key              TEXT NOT NULL,
    value            TEXT NOT NULL,
    PRIMARY KEY (repo_id, key)
);

-- copy_files is stored as JSON array in repo_config where key='copy_files'
-- Example: [",.env", ".env.local", ".secrets"]
```

Alternative for copy_files: a separate `repo_copy_files` table with one row per file. This is cleaner for updates:

```sql
CREATE TABLE IF NOT EXISTS repo_copy_files (
    id        TEXT PRIMARY KEY,
    repo_id   TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL,  -- relative path or glob pattern
    UNIQUE(repo_id, pattern)
);
```

Recommendation: use the dedicated table (`repo_copy_files`) for easier add/remove operations.

The existing `worktrees` table from migration 001 already has the right shape. No changes needed to that table.

## Plan Decomposition

Phase 2 has clear dependency layers. Seven plans across three waves:

### Wave 1: Foundation (no TUI changes)
**Plan 2A: internal/git package**
- `internal/git/git.go` — shared `run()` helper with context, dir, error wrapping
- `internal/git/worktree.go` — `ListWorktrees`, `AddWorktree`, `RemoveWorktree`
- `internal/git/state.go` — `PollState` (status --porcelain=v2 parsing)
- `internal/git/diff.go` — `Diff(mode, file)` with `--color=always`
- Unit tests for parsers using fixture output strings

**Plan 2B: Schema migration 002**
- `pkg/storage/migrations/002_git_phase.up.sql`
- `worktree_state` cache table
- `repo_copy_files` table
- Verify migration runs cleanly on top of 001

**Plan 2C: devctl worktree subcommands**
- `cmd/devctl/worktree.go` — cobra subcommand with `list`, `create`, `delete`
- `create` calls `internal/git.AddWorktree`, inserts worktrees row, copies files
- `delete` calls `internal/git.RemoveWorktree`, removes db row
- `list` reads from db (or runs `git worktree list` directly)
- Repo auto-registration on first worktree create if not tracked

### Wave 2: State Polling
**Plan 2D: StateSnapshot expansion + Manager polling**
- Expand `tuimsg.StateSnapshot` to include `[]WorktreeState`
- Expand Manager `pollLoop` to call `internal/git.PollState` for each db-tracked worktree
- Persist polled state to `worktree_state` cache table
- Load cached state on startup for instant first render

This plan requires 2A and 2B to be complete.

### Wave 3: TUI Rendering
**Plan 2E: RepoPanel with live git state**
- Update `pkg/tui/panels/left.go` to render worktrees with ahead/behind badges, staged/unstaged counts
- List UI: repos as headers, worktrees as children, arrow-key navigation
- Focus/selection state drives right panel content

Requires 2D complete.

**Plan 2F: Changed files list + inline viewer (preview + diff)**
- `pkg/tui/panels/right.go` — changed files list for selected worktree
- `pkg/tui/panels/viewer.go` — new `ViewerModel` with viewport
- File preview: chroma highlight, viewport scroll
- Diff view: 4 modes (unstaged, staged, vs main, vs origin) toggle with keys
- `e` key: `tea.ExecProcess` to launch `$EDITOR`
- Key to open viewer, key to close (Esc/q)

Requires 2D and 2E complete (needs selection state from left panel).

**Plan 2G: Editor config + GIT-09 copy list**
- Viper config for `editor` override (falls back to `$EDITOR`)
- `devctl worktree create` reads `repo_copy_files` from db and copies files
- `devctl config set-copy-files <repo> <file1> [file2...]` command

Requires 2C and 2B complete; can run in parallel with 2E/2F.

### Dependency Graph
```
2A (git pkg) ──┬──> 2D (polling) ──> 2E (left panel) ──> 2F (viewer)
2B (schema)  ──┘
               └──> 2C (worktree CLI) ──> 2G (config+copy)
```

Wave 1 (2A, 2B, 2C) is fully parallelizable after 2A completes.
Wave 2 (2D) requires 2A + 2B.
Wave 3 (2E, 2F, 2G) requires Wave 2 for state-driven work; 2G can start after 2B+2C.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| go-git for all git ops | git CLI subprocesses | go-git never got linked worktree support | Must use git subprocess; go-git is a non-starter for worktree management |
| bubbletea v1 KeyMsg | v2 KeyPressMsg | bubbletea v2.0.0 | Project already uses v2; breaking API change |
| bubbletea v1 `View() string` | v2 `View() tea.View` | bubbletea v2.0.0 | Top-level model returns tea.View; sub-models still return string |
| bubbletea v1 `tea.EnterAltScreen` cmd | v2 declarative `v.AltScreen = true` on View | bubbletea v2.0.0 | Already handled in root.go |
| `git status --porcelain` (v1) | `git status --porcelain=v2 --branch` | git 2.11+ | v2 gives ahead/behind counts in one call |

**Deprecated/outdated:**
- `github.com/charmbracelet/bubbletea` (v1): replaced by `charm.land/bubbletea/v2` — already correctly using v2
- `tea.Suspend()` message: exists in v2 as a Msg for SIGTSTP handling, but for editor launch use `tea.ExecProcess` instead — it's the purpose-built API

## Open Questions

1. **Polling interval configuration**
   - What we know: 5s is the Phase 1 default; git status is cheap per-repo
   - What's unclear: Is 5s fast enough for the "live" feel described in requirements? Power users might want 1-2s.
   - Recommendation: Start with 5s; make interval configurable via viper config in Phase 2G.

2. **Multiple repos vs. single repo architecture**
   - What we know: The schema has a `repos` table and the Manager polls all tracked worktrees.
   - What's unclear: GIT-04 says "per-worktree git state" — is a "repo" a git bare repo with multiple worktrees, or a single checkout?
   - Recommendation: Treat a repo as a git repository root; worktrees are linked trees from it. Manager polls all worktrees across all tracked repos.

3. **File copy patterns: exact paths vs globs**
   - What we know: GIT-09 mentions `.env etc` as examples; schema recommendation uses exact paths.
   - What's unclear: Whether users want glob support (`*.local`) or just exact relative paths.
   - Recommendation: Support exact relative paths only in Phase 2; add glob support later. Simpler to implement and covers 90% of use cases.

4. **Diff vs. main when main branch is named differently**
   - What we know: `git diff main...HEAD` hardcodes the base branch name.
   - What's unclear: Some repos use `master` or custom base branch names.
   - Recommendation: Store base branch name per-repo in `repo_config`. Default to `main`. `devctl worktree create` can inspect the repo's default branch on creation.

## Sources

### Primary (HIGH confidence)
- `/Users/daniel/go/pkg/mod/charm.land/bubbletea/v2@v2.0.1/exec.go` — ExecProcess API verified from source
- `/Users/daniel/go/pkg/mod/charm.land/bubbles/v2@v2.0.0/viewport/viewport.go` — viewport API verified from source
- `https://git-scm.com/docs/git-worktree` — porcelain format, field documentation
- `https://pkg.go.dev/github.com/alecthomas/chroma/v2/quick` — Highlight() signature
- `https://github.com/alecthomas/chroma/blob/master/formatters/tty_indexed.go` — formatter names "terminal", "terminal8", "terminal16", "terminal256"
- `https://github.com/alecthomas/chroma/blob/master/formatters/tty_truecolour.go` — formatter name "terminal16m"
- Live test of git status --porcelain=v2 and git worktree list --porcelain (verified output format)

### Secondary (MEDIUM confidence)
- `https://pkg.go.dev/github.com/go-git/go-git/v5` — confirmed absence of linked worktree support
- `https://pkg.go.dev/charm.land/bubbles/v2` — component inventory (viewport, list, etc.)
- `https://pkg.go.dev/charm.land/bubbletea/v2` — ExecProcess function signature

### Tertiary (LOW confidence)
- None — all claims above are backed by primary or secondary sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all dependencies verified from go.mod, module cache, or pkg.go.dev
- Architecture: HIGH — verified from bubbletea v2 source in module cache; git commands tested live
- Pitfalls: HIGH — all except chroma lexer detection derived from direct source inspection or live testing; chroma pitfall is MEDIUM (based on docs, not live test)
- Schema: HIGH — straightforward SQL; no uncertainty

**Research date:** 2026-03-05
**Valid until:** 2026-06-05 (90 days; bubbletea v2 and chroma v2 are both stable; git porcelain format is stable by design)
