# Requirements: DevCTL

**Defined:** 2026-03-05
**Core Value:** A developer can open `devctl dashboard` and immediately see everything happening across all their repos and worktrees — no lost sessions, no forgotten branches, no missed follow-ups.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Foundation

- [ ] **FOUND-01**: User can install devctl as a single binary with no runtime dependencies
- [ ] **FOUND-02**: System initializes SQLite database at `~/.devctl/state.db` with WAL mode on first run
- [ ] **FOUND-03**: System applies schema migrations automatically on startup using embedded SQL files
- [ ] **FOUND-04**: TUI renders a three-panel layout (left: repo tree, right: detail pane, bottom: log/status) using Bubbletea
- [ ] **FOUND-05**: Background state manager polls git repos and pushes state updates to TUI via buffered channel without blocking the UI

### Git Awareness

- [ ] **GIT-01**: User can list all tracked worktrees with `devctl worktree list`
- [ ] **GIT-02**: User can create a worktree for a given repo and branch with `devctl worktree create <repo> <branch>`
- [ ] **GIT-03**: User can delete a worktree with `devctl worktree delete <branch>`
- [ ] **GIT-04**: Dashboard displays per-worktree git state: ahead/behind count vs main, staged changes, unstaged changes, untracked files
- [ ] **GIT-05**: Dashboard displays the list of changed files for each worktree
- [ ] **GIT-06**: User can select a changed file to open an inline file preview with syntax highlighting and scrolling
- [ ] **GIT-07**: User can view an inline diff for a worktree (unstaged changes, staged changes, branch vs main, branch vs origin)
- [ ] **GIT-08**: User can open any viewed file in their configured editor from the inline viewer
- [ ] **GIT-09**: When creating a worktree, system copies specified local-only files (e.g. `.env`, `.env.local`, local config files) from the source worktree so the new worktree can run the app immediately; user configures the copy list per-repo in devctl config

### Dashboard

- [ ] **DASH-01**: User can launch `devctl dashboard` to see all tracked repos, worktrees, sessions, tasks, dependencies, and file changes in one view
- [ ] **DASH-02**: Dashboard shows a clear visual status for each session and process: running, idle, finished, interrupted, blocked
- [ ] **DASH-03**: User can navigate the dashboard with keyboard only: ↑↓ to move between repos/worktrees, enter to expand, `d` to view diff, `f` to view file, `r` to restore session, `t` to view task

### Session Management

- [ ] **SESS-01**: User can start a session with `devctl session start` associating it with a repo, worktree, branch, and optional task
- [ ] **SESS-02**: User can stop a session with `devctl session stop`
- [ ] **SESS-03**: User can list all sessions (active and historical) with `devctl session list`
- [ ] **SESS-04**: System records last_activity timestamp and last_command for each session
- [ ] **SESS-05**: User can switch to any active worktree context with `devctl jump` via a fuzzy selector showing all active sessions

### Tasks & Dependencies

- [ ] **TASK-01**: User can create a task with a description via `devctl tasks` (CRUD operations)
- [ ] **TASK-02**: Tasks have states: queued, running, blocked, completed
- [ ] **TASK-03**: User can link tasks as dependencies with `devctl deps add <task> <depends-on>`
- [ ] **TASK-04**: User can remove dependency links with `devctl deps remove <task> <depends-on>`
- [ ] **TASK-05**: User can list all dependency relationships with `devctl deps list`
- [ ] **TASK-06**: System detects task dependencies from explicit links and git branch ancestry relationships
- [ ] **TASK-07**: System automatically marks a task as blocked when its upstream dependency is incomplete or upstream branch is not merged
- [ ] **TASK-08**: Dashboard surfaces task execution graph showing ready, blocked, and completed tasks

### AI Observability

- [ ] **AI-01**: Dashboard displays all active Claude Code sessions with current file modifications and commands being executed
- [ ] **AI-02**: User can select a Claude session in the dashboard to open a docked split-pane view (left: repos/sessions, right: live Claude output with scroll history)
- [ ] **AI-03**: System detects when a branch becomes idle (no commits, no session activity for configurable threshold, default 20 minutes)
- [ ] **AI-04**: System triggers configured agent analysis workflows when a branch goes idle (code review, test generation, refactor suggestions, documentation, dependency review)
- [ ] **AI-05**: Agent-generated improvements appear as draft patches in the dashboard
- [ ] **AI-06**: User can approve, reject, edit, or apply each draft patch via `devctl agent review`
- [ ] **AI-07**: User can apply a reviewed patch with `devctl agent apply`
- [ ] **AI-08**: User can revert an applied agent patch with `devctl agent revert` / `devctl patch revert`
- [ ] **AI-09**: Agent patches are stored as temporary git patches, making them fully reversible before apply
- [ ] **AI-10**: User can enable or disable individual agent action types (code_review, test_generation, refactor_suggestions, documentation) via configuration

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Session Recovery

- **SESS-V2-01**: System checkpoints active sessions periodically storing repo, worktree, branch, task, last command, modified files, and timestamp
- **SESS-V2-02**: User can restore an interrupted session with `devctl session restore <session_id>`, navigating to the worktree and displaying previous context

### GitHub Integration

- **GH-V2-01**: User can create a GitHub PR from a worktree directly via devctl
- **GH-V2-02**: Dashboard surfaces CI status for open PRs

### Notifications

- **NOTF-V2-01**: User can configure desktop notifications for idle detection triggers
- **NOTF-V2-02**: User can configure notifications when a blocked task becomes unblocked

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Cloud collaboration | Single-developer local tool by design; team features are a different product |
| Team coordination | Multi-user state synchronization is out of scope for v1 |
| GitHub PR automation | Future enhancement; adds significant OAuth/API complexity |
| CI/CD integration | Remote pipeline awareness is out of scope for local-first tool |
| Remote execution | Local-only by design; remote would require auth, networking |
| Terminal emulator features | devctl is not tmux/zellij; avoid replacing terminal multiplexers |
| Code editor features | devctl views files/diffs but does not edit; `open in editor` is the boundary |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 1 | Pending |
| FOUND-02 | Phase 1 | Pending |
| FOUND-03 | Phase 1 | Pending |
| FOUND-04 | Phase 1 | Pending |
| FOUND-05 | Phase 1 | Pending |
| GIT-01 | Phase 2 | Pending |
| GIT-02 | Phase 2 | Pending |
| GIT-03 | Phase 2 | Pending |
| GIT-04 | Phase 2 | Pending |
| GIT-05 | Phase 2 | Pending |
| GIT-06 | Phase 2 | Pending |
| GIT-07 | Phase 2 | Pending |
| GIT-08 | Phase 2 | Pending |
| GIT-09 | Phase 2 | Pending |
| DASH-01 | Phase 3 | Pending |
| DASH-02 | Phase 3 | Pending |
| DASH-03 | Phase 3 | Pending |
| SESS-01 | Phase 4 | Pending |
| SESS-02 | Phase 4 | Pending |
| SESS-03 | Phase 4 | Pending |
| SESS-04 | Phase 4 | Pending |
| SESS-05 | Phase 4 | Pending |
| TASK-01 | Phase 5 | Pending |
| TASK-02 | Phase 5 | Pending |
| TASK-03 | Phase 5 | Pending |
| TASK-04 | Phase 5 | Pending |
| TASK-05 | Phase 5 | Pending |
| TASK-06 | Phase 5 | Pending |
| TASK-07 | Phase 5 | Pending |
| TASK-08 | Phase 5 | Pending |
| AI-01 | Phase 6 | Pending |
| AI-02 | Phase 6 | Pending |
| AI-03 | Phase 6 | Pending |
| AI-04 | Phase 6 | Pending |
| AI-05 | Phase 6 | Pending |
| AI-06 | Phase 6 | Pending |
| AI-07 | Phase 6 | Pending |
| AI-08 | Phase 6 | Pending |
| AI-09 | Phase 6 | Pending |
| AI-10 | Phase 6 | Pending |

**Coverage:**
- v1 requirements: 39 total
- Mapped to phases: 38
- Unmapped: 0 ✓

---
*Requirements defined: 2026-03-05*
*Last updated: 2026-03-05 after initial definition*
