# Phase 5: Tasks and Dependencies - Research

**Researched:** 2026-03-06
**Domain:** Task CRUD, DAG dependency resolution, git branch ancestry, TUI graph rendering
**Confidence:** HIGH

## Summary

Phase 5 adds task management and dependency tracking to devctl. The core domain is a directed acyclic graph (DAG) where tasks are nodes and dependency links are edges. The system must determine which tasks are "ready" (all upstream dependencies completed AND upstream branches merged) versus "blocked" (at least one incomplete dependency or unmerged upstream branch). This is fundamentally a topological sort problem combined with git branch ancestry checks.

The existing codebase provides all the building blocks: SQLite + sqlx for storage, Cobra for CLI subcommands, the poll-based dashboard manager for state updates, and Bubbletea v2 for TUI rendering. No new external dependencies are needed. The DAG logic (topological sort, cycle detection, ready/blocked computation) is simple enough to hand-roll in ~100 lines of Go using Kahn's algorithm. For TUI graph visualization, the practical approach is a layered text rendering using box-drawing characters -- not a full graph layout engine.

The git branch ancestry check uses `git merge-base --is-ancestor <branch> <target>` which returns exit code 0 if merged, 1 if not. This fits perfectly into the existing `internal/git` subprocess pattern. The poll loop already runs git commands per worktree; adding one `merge-base` call per task-with-branch-link is low overhead.

**Primary recommendation:** Add `tasks` and `task_deps` tables via migration 003, implement `devctl tasks` and `devctl deps` as Cobra subcommands, build the DAG resolver as a pure function in `internal/task/`, add branch ancestry checking in `internal/git/`, and render the task graph in the dashboard as a layered text view in a new panel mode.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/spf13/cobra` | v1.10.2 | CLI: `devctl tasks` and `devctl deps` subcommands | Already in use; matches repo/worktree patterns |
| `github.com/jmoiron/sqlx` | v1.4.0 | Task and dependency CRUD | Already in use; StructScan pattern |
| `modernc.org/sqlite` | v1.18.1 | Storage backend | Already in use; no CGO |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | Migration 003 for tasks/deps tables | Already in use; embedded FS |
| `charm.land/bubbletea/v2` | v2.0.1 | Dashboard task graph panel | Already in use; v2 API |
| `charm.land/lipgloss/v2` | v2.0.0 | Task graph styling | Already in use |
| `github.com/google/uuid` | v1.6.0 | Task ID generation | Already in use |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/spf13/viper` | v1.21.0 | Config for default branch name | Already in use |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled DAG | `github.com/souleb/dag` | External dep for ~80 lines of code; not worth it |
| Hand-rolled DAG | `gonum.org/v1/gonum/graph` | Heavy dependency (math library); massive overkill |
| Text-based graph | `github.com/guptarohit/asciigraph` | Only does line charts, not DAG layouts |
| Layered text rendering | Full Sugiyama graph layout | Extremely complex; terminal constraints make simple layering sufficient |

**Installation:**
No new dependencies needed. All libraries already in go.mod.

## Architecture Patterns

### Recommended Project Structure
```
internal/task/
  store.go           # TaskStore: CRUD operations against SQLite
  resolver.go        # DAG resolution: topo sort, ready/blocked computation
  resolver_test.go   # Table-driven tests for cycle detection, blocking logic
internal/dependency/
  store.go           # DepStore: dependency link CRUD
internal/git/
  ancestry.go        # IsBranchMerged(ctx, repoPath, branch, target) bool
cmd/devctl/
  task.go            # devctl tasks create/list/update/delete
  deps.go            # devctl deps add/remove/list
pkg/tui/tuimsg/
  messages.go        # Add TaskState, TaskGraphSnapshot types
pkg/tui/panels/
  tasks.go           # TaskGraphPanel: layered text rendering of DAG
pkg/storage/migrations/
  003_tasks.up.sql    # tasks + task_deps tables
  003_tasks.down.sql
```

### Pattern 1: Task Store (CRUD with sqlx)
**What:** Follow the existing DB access pattern -- store struct with `*sqlx.DB`, methods return domain types.
**When to use:** All task and dependency CRUD operations.
**Example:**
```go
// Source: mirrors existing codebase patterns (repo.go, worktree.go)
type Task struct {
    ID          string    `db:"id"`
    Description string    `db:"description"`
    State       string    `db:"state"`       // queued, running, blocked, completed
    Branch      string    `db:"branch"`      // optional: linked git branch
    WorktreeID  string    `db:"worktree_id"` // optional: linked worktree
    CreatedAt   int64     `db:"created_at"`
    UpdatedAt   int64     `db:"updated_at"`
}

type TaskStore struct {
    db *sqlx.DB
}

func (s *TaskStore) Create(ctx context.Context, desc string) (Task, error) {
    t := Task{
        ID:          uuid.New().String(),
        Description: desc,
        State:       "queued",
        CreatedAt:   time.Now().Unix(),
        UpdatedAt:   time.Now().Unix(),
    }
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO tasks (id, description, state, branch, worktree_id, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, t.ID, t.Description, t.State, t.Branch, t.WorktreeID, t.CreatedAt, t.UpdatedAt)
    return t, err
}
```

### Pattern 2: DAG Resolver (Pure Function)
**What:** Compute task readiness from the dependency graph + branch merge status. Pure function: takes tasks + deps + merge-status map, returns annotated tasks.
**When to use:** Every poll cycle, after querying tasks and checking branch ancestry.
**Example:**
```go
// Source: Kahn's algorithm / leaf-removal topological sort
// Reference: https://kendru.github.io/go/2021/10/26/sorting-a-dependency-graph-in-go/

type ResolvedTask struct {
    Task       Task
    IsReady    bool     // all deps completed AND branches merged
    IsBlocked  bool     // at least one dep incomplete or branch unmerged
    BlockedBy  []string // IDs of blocking upstream tasks
    Layer      int      // topological layer (0 = no deps, 1 = depends on layer 0, etc.)
}

// Resolve computes readiness for all tasks.
// branchMerged maps task.ID -> true if the task's branch is merged into target.
func Resolve(tasks []Task, deps []Dep, branchMerged map[string]bool) ([]ResolvedTask, error) {
    // Build adjacency: task -> upstream dependencies
    upstream := make(map[string][]string)  // taskID -> list of dependency taskIDs
    taskMap := make(map[string]Task)
    for _, t := range tasks {
        taskMap[t.ID] = t
    }
    for _, d := range deps {
        upstream[d.TaskID] = append(upstream[d.TaskID], d.DependsOnID)
    }

    // Kahn's algorithm for layered topo sort
    inDegree := make(map[string]int)
    for _, t := range tasks {
        if _, ok := inDegree[t.ID]; !ok {
            inDegree[t.ID] = 0
        }
        for _, depID := range upstream[t.ID] {
            inDegree[t.ID]++
            _ = depID
        }
    }

    // ... layer assignment, ready/blocked computation ...
    // A task is READY when:
    //   1. State is "queued" (not already running/completed)
    //   2. All upstream tasks have state == "completed"
    //   3. All upstream tasks with branches have branchMerged[id] == true
    // A task is BLOCKED when any upstream fails conditions 2 or 3.
    return resolved, nil
}
```

### Pattern 3: Git Branch Ancestry Check
**What:** Use `git merge-base --is-ancestor` to check if a branch has been merged.
**When to use:** During poll cycle, for each task that has a linked branch.
**Example:**
```go
// Source: git-scm.com/docs/git-merge-base
// Fits existing internal/git/git.go subprocess pattern

// IsBranchMerged checks if branchName has been merged into targetBranch.
// Returns true if branchName is an ancestor of targetBranch.
func IsBranchMerged(ctx context.Context, repoPath, branchName, targetBranch string) (bool, error) {
    _, err := run(ctx, repoPath, "merge-base", "--is-ancestor", branchName, targetBranch)
    if err == nil {
        return true, nil // exit 0 = is ancestor = merged
    }
    // Exit code 1 = not ancestor = not merged (this is not an error)
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
        return false, nil
    }
    return false, err // actual error (exit code > 1 or other failure)
}
```

### Pattern 4: Layered Text Graph Rendering
**What:** Render tasks as a layered DAG using box-drawing characters. Each layer is a column (or row) of tasks at the same topological depth.
**When to use:** Dashboard task graph panel.
**Example:**
```
Layer 0          Layer 1          Layer 2
+-----------+    +-----------+    +-----------+
| DB schema |---→| API impl  |---→| Tests     |
| [done]    |    | [running] |    | [queued]  |
+-----------+    +-----------+    +-----------+
                 +-----------+
            +---→| UI impl   |---→
                 | [blocked] |
                 +-----------+
```
**Rendering approach:** Use lipgloss for styled boxes. Draw edges with `─`, `→`, `├`, `└` Unicode box characters. Layers flow left-to-right. Each task is a small box showing description (truncated) + state badge. Vertical stacking within a layer when multiple tasks share the same depth.

### Anti-Patterns to Avoid
- **Circular dependencies:** MUST detect cycles at insertion time (`devctl deps add`) and reject with a clear error. Never allow the DB to contain cycles.
- **Polling branch merge status for every task every cycle:** Only check branches for tasks in "queued" or "running" state. Completed/blocked tasks with no upstream changes can be cached.
- **Storing computed state (ready/blocked) in DB:** Compute on every poll cycle from current dep graph + git state. The DB stores only the explicit user-set state + links.
- **Auto-transitioning task states without user action:** The system MARKS tasks as blocked (computed), but only the user transitions queued->running or running->completed. Blocked is a computed overlay, not a user-set state.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| UUID generation | Custom ID scheme | `github.com/google/uuid` | Already in use; collision-free |
| SQL migrations | Manual `CREATE TABLE IF NOT EXISTS` | `golang-migrate` embedded FS | Already in use; versioned, reversible |
| CLI argument parsing | Manual flag parsing | Cobra subcommands | Already in use; completions, help text |
| Terminal styling | Manual ANSI codes | lipgloss | Already in use; composable styles |

**Key insight:** The DAG resolver IS worth hand-rolling (Kahn's algorithm is ~50 lines), but the surrounding infrastructure (storage, CLI, TUI) should use the existing stack exactly as prior phases do.

## Common Pitfalls

### Pitfall 1: Circular Dependencies
**What goes wrong:** User creates A->B->C->A cycle. Topo sort fails or infinite loops.
**Why it happens:** No validation at insertion time.
**How to avoid:** On `devctl deps add X Y`, run cycle detection before INSERT. Use DFS from Y following existing edges to see if X is reachable. If so, reject with "would create circular dependency: X -> ... -> Y -> X".
**Warning signs:** `Resolve()` returns error or empty result.

### Pitfall 2: Orphaned Dependencies
**What goes wrong:** Task deleted but dependency links remain, causing foreign key errors or phantom blockers.
**Why it happens:** No CASCADE delete on task_deps.
**How to avoid:** `ON DELETE CASCADE` on both FK columns in task_deps table. Test with: create task, add dep, delete task, verify dep link removed.

### Pitfall 3: Branch Name Ambiguity
**What goes wrong:** Task linked to branch "feature/x" but user meant the remote tracking branch, or branch was force-pushed and the old commit is gone.
**Why it happens:** `git merge-base --is-ancestor` works on local refs. If branch is deleted after merge (common), the check fails.
**How to avoid:** Check both local and remote refs: `refs/heads/<branch>` and `refs/remotes/origin/<branch>`. Also consider: if the branch ref doesn't exist at all, treat it as "merged" (branch was cleaned up post-merge). Make this configurable via viper.

### Pitfall 4: State Machine Confusion
**What goes wrong:** Task state becomes inconsistent. E.g., task marked "completed" but dependency still shows "blocked".
**Why it happens:** Mixing user-set state with computed state.
**How to avoid:** Clear separation: DB stores user-set state (queued/running/completed). "blocked" is computed at resolve time based on upstream deps. A task's DB state might be "queued" but its resolved state is "blocked" because an upstream is incomplete.

### Pitfall 5: Performance with Many Tasks
**What goes wrong:** Poll cycle becomes slow if checking branch ancestry for 50+ tasks.
**Why it happens:** Each `git merge-base --is-ancestor` spawns a subprocess.
**How to avoid:** Cache merge-base results keyed by (branch, target, target-commit-hash). Invalidate when target branch moves (different HEAD). Most branch ancestry relationships don't change between poll cycles.

### Pitfall 6: TUI Graph Overflow
**What goes wrong:** Graph with many tasks overflows terminal width or height.
**Why it happens:** Naive rendering doesn't account for terminal dimensions.
**How to avoid:** Scrollable viewport for the graph panel. Truncate task descriptions. Collapse completed tasks into a summary. Show at most N layers at once with horizontal scroll.

## Code Examples

### Migration 003: Tasks and Dependencies Schema
```sql
-- Source: follows existing migration patterns (001, 002)

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'queued',  -- queued, running, completed
    branch      TEXT DEFAULT '',                  -- optional linked git branch
    worktree_id TEXT DEFAULT '' REFERENCES worktrees(id) ON DELETE SET DEFAULT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);

CREATE TABLE IF NOT EXISTS task_deps (
    task_id       TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (task_id, depends_on_id),
    CHECK (task_id != depends_on_id)  -- no self-dependencies
);

CREATE INDEX IF NOT EXISTS idx_task_deps_depends_on ON task_deps(depends_on_id);
```

### Cycle Detection at Insert Time
```go
// Source: standard DFS cycle detection

// WouldCreateCycle checks if adding edge (taskID -> dependsOnID) creates a cycle.
// It does DFS from dependsOnID to see if taskID is reachable.
func WouldCreateCycle(deps []Dep, taskID, dependsOnID string) bool {
    // Build adjacency: X depends on Y means edge Y -> X in "blocks" direction
    // We need: from dependsOnID, can we reach taskID by following "depends_on" edges?
    upstream := make(map[string][]string) // task -> what it depends on
    for _, d := range deps {
        upstream[d.TaskID] = append(upstream[d.TaskID], d.DependsOnID)
    }

    // DFS from dependsOnID following upstream edges
    visited := make(map[string]bool)
    var dfs func(node string) bool
    dfs = func(node string) bool {
        if node == taskID {
            return true // cycle found
        }
        if visited[node] {
            return false
        }
        visited[node] = true
        for _, up := range upstream[node] {
            if dfs(up) {
                return true
            }
        }
        return false
    }
    return dfs(dependsOnID)
}
```

### Cobra Subcommand Pattern for Tasks
```go
// Source: mirrors existing cmd/devctl/repo.go pattern

var taskCmd = &cobra.Command{
    Use:   "tasks",
    Short: "Manage tasks",
}

var taskCreateCmd = &cobra.Command{
    Use:   "create <description>",
    Short: "Create a new task",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
        store := task.NewStore(db)
        t, err := store.Create(cmd.Context(), args[0])
        if err != nil {
            return err
        }
        fmt.Printf("Created task %s: %s\n", t.ID[:8], t.Description)
        return nil
    },
}

var taskListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all tasks with status",
    RunE: func(cmd *cobra.Command, args []string) error {
        db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
        store := task.NewStore(db)
        tasks, err := store.List(cmd.Context())
        if err != nil {
            return err
        }
        for _, t := range tasks {
            fmt.Printf("  [%s] %-8s %s\n", t.ID[:8], t.State, t.Description)
        }
        return nil
    },
}

func init() {
    taskCmd.AddCommand(taskCreateCmd, taskListCmd, taskUpdateCmd, taskDeleteCmd)
}
```

### Dashboard Integration: Task State in Poll Loop
```go
// Source: mirrors existing manager.go pollAllWorktrees pattern

// In dashboard/manager.go pollLoop, after worktree polling:
// 1. Query all tasks + deps from DB
// 2. For tasks with branches, check merge status via git
// 3. Run Resolve() to compute ready/blocked
// 4. Include TaskGraphSnapshot in StateSnapshot

type TaskGraphSnapshot struct {
    Tasks    []ResolvedTask
    HasCycle bool
}

// Added to tuimsg.StateSnapshot:
// TaskGraph TaskGraphSnapshot
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Separate graph libraries | Hand-roll Kahn's algorithm | Always (for small DAGs) | No dependency bloat |
| `git branch --merged` | `git merge-base --is-ancestor` | git 1.8.0+ (2012) | Direct ancestor check, cleaner semantics |
| Full Sugiyama layout | Layered text with box-drawing | Terminal constraint | Practical for <50 nodes |

**Deprecated/outdated:**
- `git branch --contains` is slower than `merge-base --is-ancestor` for single-branch checks.

## Open Questions

1. **Task-to-worktree vs task-to-branch linking**
   - What we know: Tasks need both a branch link (for merge detection) and optionally a worktree link (for dashboard grouping).
   - What's unclear: Should tasks be scoped to a specific repo, or are they global? A branch name alone is ambiguous across repos.
   - Recommendation: Add a `repo_id` column to tasks. Tasks are scoped to a repo. Branch names are resolved within that repo's path.

2. **"blocked" as state vs computed overlay**
   - What we know: Requirements say states are "queued, running, blocked, completed". But blocked is really a computed property.
   - What's unclear: Should the DB column ever contain "blocked", or is it always computed?
   - Recommendation: DB stores only user-set states: queued, running, completed. "blocked" is computed by the resolver and returned as a field on ResolvedTask. The CLI `devctl tasks list` shows the computed state. This prevents state machine bugs.

3. **Graph panel placement in dashboard**
   - What we know: The dashboard has left panel (repos) + right panel (detail). The `t` key is already wired as a placeholder.
   - What's unclear: Should the task graph replace the right panel, be a new panel mode, or a full-screen overlay?
   - Recommendation: New mode on the right panel (like sessions vs files mode). Press `t` to toggle into task graph view. This follows the existing pattern and avoids layout changes.

4. **Target branch for merge detection**
   - What we know: `git merge-base --is-ancestor feature main` checks if feature is merged into main.
   - What's unclear: What is the "target branch"? Always `main`? The repo's default branch?
   - Recommendation: Use the repo's default branch (detected via `git symbolic-ref refs/remotes/origin/HEAD`), with a fallback viper config `task.default_target_branch` (default: "main").

## Sources

### Primary (HIGH confidence)
- Existing codebase: `internal/git/git.go`, `internal/dashboard/manager.go`, `cmd/devctl/repo.go` -- established patterns
- Existing migrations: `001_initial.up.sql`, `002_git_phase.up.sql` -- schema conventions
- [git merge-base docs](https://git-scm.com/docs/git-merge-base) -- `--is-ancestor` semantics and exit codes

### Secondary (MEDIUM confidence)
- [Sorting a Dependency Graph in Go](https://kendru.github.io/go/2021/10/26/sorting-a-dependency-graph-in-go/) -- layered topo sort pattern
- [souleb/dag Go package](https://pkg.go.dev/github.com/souleb/dag) -- Kahn's algorithm reference implementation
- [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) -- terminal DAG visualization approach with Bubbletea

### Tertiary (LOW confidence)
- Terminal graph rendering specifics -- no authoritative source for best box-drawing layout; will need iterative prototyping

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already in use, no new deps needed
- Architecture: HIGH -- follows established codebase patterns exactly
- DAG resolution: HIGH -- well-understood algorithm (Kahn's), simple implementation
- Git ancestry: HIGH -- `merge-base --is-ancestor` is stable, documented git feature
- TUI graph rendering: MEDIUM -- approach is sound but layout details need prototyping; no prior art specific to this codebase's panel system
- Pitfalls: HIGH -- standard DAG/state-machine concerns, well-documented

**Research date:** 2026-03-06
**Valid until:** 2026-04-06 (stable domain; no fast-moving dependencies)
