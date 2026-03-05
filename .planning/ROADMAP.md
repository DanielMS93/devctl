# Roadmap: DevCTL

## Overview

DevCTL is built bottom-up: correct architectural foundations first, then git integration and worktree management, then the dashboard TUI that surfaces it all, then sessions, then the dependency graph, and finally AI observability with idle-triggered agent actions. Each phase delivers a coherent, verifiable capability that the next phase builds on. The six phases match the natural dependency graph of the system — nothing is built before what it depends on exists.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Foundation** - Storage, CLI scaffold, TUI skeleton, and background state manager with correct concurrency patterns
- [ ] **Phase 2: Git Integration** - Worktree CRUD, git state polling, file change visibility, and inline diff/file viewer
- [ ] **Phase 3: Dashboard TUI** - Three-panel layout, keyboard navigation, and full git state rendered in the TUI
- [ ] **Phase 4: Session Management** - Session lifecycle, context switching, and session panel in the dashboard
- [ ] **Phase 5: Tasks and Dependencies** - Task CRUD, dependency graph, blocked/ready state, dashboard visualization
- [ ] **Phase 6: AI Observability** - Claude session monitoring, idle detection, draft agent patches, and approval workflow

## Phase Details

### Phase 1: Foundation
**Goal**: The project compiles, installs as a single binary, stores data reliably, and the TUI renders with correct concurrency — the architectural floor everything else stands on
**Depends on**: Nothing (first phase)
**Requirements**: FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05
**Success Criteria** (what must be TRUE):
  1. `go install` produces a single `devctl` binary that runs with no external runtime dependencies
  2. Running any `devctl` command for the first time creates `~/.devctl/state.db` with WAL mode enabled and applies all migrations automatically
  3. `devctl dashboard` opens without crashing and renders a three-panel TUI skeleton (even with empty data)
  4. Background git polling runs without blocking the TUI — the UI stays responsive while state updates arrive
  5. Running `devctl` with `-race` produces zero data race warnings
**Plans**: 3 plans

Plans:
- [ ] 01-01-PLAN.md — Go module init, all dependencies, storage.Open() with WAL pragmas, embedded migration runner, initial schema
- [ ] 01-02-PLAN.md — Background state manager (buffered channel, context-cancelled goroutine), Bubbletea v2 RootModel and three stub panels
- [ ] 01-03-PLAN.md — Cobra CLI wiring, full integration, race detector smoke test, human TUI verification

### Phase 2: Git Integration
**Goal**: Users can manage worktrees from the CLI and see accurate, live git state (ahead/behind, staged/unstaged, changed files, diffs) per worktree
**Depends on**: Phase 1
**Requirements**: GIT-01, GIT-02, GIT-03, GIT-04, GIT-05, GIT-06, GIT-07, GIT-08, GIT-09
**Success Criteria** (what must be TRUE):
  1. User can create, list, and delete worktrees via `devctl worktree` subcommands and the new worktree has specified local-only files copied over
  2. Dashboard shows accurate ahead/behind count vs main, staged, unstaged, and untracked file counts for each tracked worktree
  3. User can select a changed file in the dashboard to open an inline preview with syntax highlighting and scroll
  4. User can view an inline diff (unstaged, staged, branch vs main, branch vs origin) from the dashboard
  5. User can press a key in the inline viewer to open the current file in their configured editor
**Plans**: TBD

Plans:
- [ ] 02-01: TBD

### Phase 3: Dashboard TUI
**Goal**: Users can launch `devctl dashboard` and see all tracked repos, worktrees, and file change state in one keyboard-navigable view
**Depends on**: Phase 2
**Requirements**: DASH-01, DASH-02, DASH-03
**Success Criteria** (what must be TRUE):
  1. `devctl dashboard` displays all tracked repos, worktrees, and their git state in a three-panel layout without requiring a mouse
  2. Each worktree entry shows a clear visual status (running, idle, finished, interrupted, blocked)
  3. User can navigate entirely with keyboard: arrow keys move between repos/worktrees, enter expands, `d` opens diff, `f` opens file, `r` triggers session restore, `t` opens task view
  4. Dashboard resizes correctly when the terminal window is resized
**Plans**: TBD

Plans:
- [ ] 03-01: TBD

### Phase 4: Session Management
**Goal**: Users can track, list, and jump between work sessions, and the dashboard reflects live session state
**Depends on**: Phase 3
**Requirements**: SESS-01, SESS-02, SESS-03, SESS-04, SESS-05
**Success Criteria** (what must be TRUE):
  1. User can start and stop sessions with `devctl session start/stop`, each linked to a repo, worktree, branch, and optional task
  2. `devctl session list` shows all active and historical sessions with status, last activity timestamp, and last command
  3. `devctl jump` opens a fuzzy selector across all active worktree sessions and switches context on selection
  4. Session state persists across `devctl` restarts — sessions started before a restart still appear in `session list`
**Plans**: TBD

Plans:
- [ ] 04-01: TBD

### Phase 5: Tasks and Dependencies
**Goal**: Users can create and link tasks, and the dashboard surfaces which tasks are ready versus blocked based on explicit links and git branch ancestry
**Depends on**: Phase 4
**Requirements**: TASK-01, TASK-02, TASK-03, TASK-04, TASK-05, TASK-06, TASK-07, TASK-08
**Success Criteria** (what must be TRUE):
  1. User can create, update, and delete tasks via `devctl tasks` and assign them states (queued, running, blocked, completed)
  2. User can declare and remove dependency links between tasks with `devctl deps add/remove` and list all links with `devctl deps list`
  3. System automatically marks a task as blocked when its upstream dependency task is not completed or the upstream branch is not merged
  4. Dashboard displays a task execution graph showing which tasks are ready, blocked, and completed
**Plans**: TBD

Plans:
- [ ] 05-01: TBD

### Phase 6: AI Observability
**Goal**: Users can see live Claude Code session activity in the dashboard, receive idle-triggered draft patches, and approve or revert agent-generated changes
**Depends on**: Phase 5
**Requirements**: AI-01, AI-02, AI-03, AI-04, AI-05, AI-06, AI-07, AI-08, AI-09, AI-10
**Success Criteria** (what must be TRUE):
  1. Dashboard shows all active Claude Code sessions with the files being modified and commands executing in real time
  2. User can select a Claude session and open a docked split-pane showing live streamed output with scroll history
  3. When a branch has no commits or session activity for the configured threshold (default 20 min), the system triggers configured agent analysis workflows automatically
  4. Agent-generated improvements appear as draft patches in the dashboard; user can approve, reject, edit, or apply each patch via `devctl agent review` and `devctl agent apply`
  5. User can revert any applied agent patch with `devctl agent revert` and user can enable or disable individual agent action types via configuration
**Plans**: TBD

Plans:
- [ ] 06-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 0/TBD | Not started | - |
| 2. Git Integration | 0/TBD | Not started | - |
| 3. Dashboard TUI | 0/TBD | Not started | - |
| 4. Session Management | 0/TBD | Not started | - |
| 5. Tasks and Dependencies | 0/TBD | Not started | - |
| 6. AI Observability | 0/TBD | Not started | - |
