# DevCTL

## What This Is

DevCTL is a terminal-based developer session orchestrator — a local CLI/TUI control plane that gives engineers a single interface to manage all development activity across repositories, git worktrees, coding sessions, tasks, file changes, and dependencies. It runs locally, integrates with Git and AI coding agents (Claude Code), and surfaces situational awareness for multi-repository parallel workflows.

## Core Value

A developer can open `devctl dashboard` and immediately see everything happening across all their repos and worktrees — no lost sessions, no forgotten branches, no missed follow-ups.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Terminal dashboard (`devctl dashboard`) displaying repos, worktrees, sessions, tasks, dependencies, and file changes
- [ ] Worktree manager — create, delete, list, switch worktrees; update branches
- [ ] Git state awareness per worktree — ahead/behind vs main, staged, unstaged, untracked files
- [ ] File change visibility — list changed files per worktree with inline preview on select
- [ ] Inline file viewer — syntax highlighting, scrolling, open-in-editor
- [ ] Inline diff viewer — unstaged, staged, branch vs main, branch vs origin
- [ ] Session tracking — start/stop/list sessions; store repo, worktree, branch, task, timestamps, last command
- [ ] Session resurrection — checkpoint sessions periodically; restore via `devctl session restore <id>`
- [ ] Fast context switching — `devctl jump` fuzzy selector across all active worktrees/sessions
- [ ] Dependency detection — explicit task links, git branch ancestry, file change impact, module dependencies
- [ ] Dependency visualization — blocked task indicators, dependency graph display in dashboard
- [ ] Blocked task detection — auto-detect when upstream dependency is incomplete or not merged
- [ ] Work planning engine — task execution graph surfacing ready/blocked/completed tasks
- [ ] Live AI session observability — display active Claude sessions, files being modified, commands, tests running
- [ ] Claude session docked window — split-pane view (repos/sessions left, Claude output right) with live streaming logs
- [ ] Idle branch detection — detect inactivity (default 20 min threshold) and trigger post-work analysis
- [ ] Draft agent actions — on idle: code review, test generation, refactor suggestions, documentation, dependency review
- [ ] Draft patch generation + approval workflow — approve/reject/edit/apply agent-generated patches
- [ ] Patch reversibility — temporary git patch format; `devctl patch revert`
- [ ] Agent workflow configuration — enable/disable individual agent action types per user preference
- [ ] Keyboard-driven TUI navigation — arrow keys, enter, d/f/r/t keybindings; left/right/bottom panel layout

### Out of Scope

- Cloud collaboration — single-developer local tool only; team features deferred
- Team coordination features — not in initial release
- GitHub PR automation — future enhancement
- CI/CD integration — future enhancement
- Remote execution — local-only by design

## Context

- Stack is decided: Go, Bubbletea + Lipgloss (TUI), Git CLI integration, SQLite
- Local storage: `~/.devctl/state.db` (tables: repos, worktrees, sessions, tasks, task_dependencies, patches)
- Session checkpoints: `~/.devctl/sessions/`
- Project layout: `cmd/devctl`, `internal/` (dashboard, git, session, task, dependency, agent, diff), `pkg/` (tui, storage)
- Primary integration target for AI sessions is Claude Code

## Constraints

- **Tech Stack**: Go — already decided, cross-platform binary distribution
- **Tech Stack**: Bubbletea + Lipgloss — already decided for TUI framework
- **Storage**: SQLite — local only, no network database
- **Scope**: Local only — no cloud, no remote, no team features in v1
- **Git Integration**: Via git CLI (not libgit2) — simpler dependency, good enough for MVP

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go for implementation | Cross-platform binary, fast startup, good TUI ecosystem | — Pending |
| Bubbletea + Lipgloss for TUI | Standard Go TUI stack, active community, composable | — Pending |
| SQLite for storage | Local-first, zero-dependency DB, sufficient for single-user state | — Pending |
| Git CLI (not libgit2) | Simpler build, no CGO dependency, easier to distribute | — Pending |
| Session checkpoints as files in ~/.devctl/sessions/ | Separate from DB for crash resilience | — Pending |

---
*Last updated: 2026-03-05 after initialization*
