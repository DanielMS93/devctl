# Feature Landscape

**Domain:** Terminal-based developer session orchestrator / multi-repo worktree manager
**Researched:** 2026-03-05
**Confidence note:** Web tools unavailable. Analysis draws from training knowledge of tmux, zellij, lazygit, gitui, mprocs, and similar tools (training cutoff August 2025), plus the detailed PROJECT.md requirements. Confidence is MEDIUM overall — the tool landscape for these well-established tools is stable; the AI-agent observability angle is newer.

---

## Reference Tools Surveyed

| Tool | Domain | Key insight |
|------|--------|-------------|
| tmux | Terminal multiplexer / session mgr | Gold standard for sessions; painful UX, no git awareness |
| zellij | Terminal workspace / multiplexer | Better UX than tmux; layouts, plugins, still no git layer |
| mprocs | Multi-process runner TUI | Process list + log pane; minimal, focused, no git |
| lazygit | Git TUI | Git operations with full keyboard UI; no session/worktree orchestration |
| gitui | Git TUI (Rust) | Faster than lazygit; similar git-only scope |
| Warp | Terminal emulator | AI shell, blocks, modern UX; not a session orchestrator |
| gh cli | GitHub CLI | Repo/PR operations; no local session tracking |
| direnv / mise | Environment managers | Per-directory env; not a dashboard |

---

## Table Stakes

Features users expect from a tool in this category. Missing = product feels incomplete or broken.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Session list with status | Every session tool (tmux, zellij) shows running sessions | Low | Name, start time, current dir, alive/dead |
| Session attach / detach | Core session management primitive | Low | Map to worktrees in devctl's model |
| Session create / kill | Basic lifecycle | Low | — |
| Git status per repo/worktree | Any git-aware tool shows ahead/behind, staged, unstaged counts | Medium | lazygit, gitui set the bar; devctl must match |
| Branch name visible at all times | Developers orient by branch; always-visible is expected | Low | Must be prominent in dashboard |
| Keyboard-only navigation | Mouse is unacceptable in serious terminal tools | Medium | Arrow keys, vim-style hjkl, enter; esc to go back |
| Fuzzy search / jump | Popularized by fzf; developers expect it everywhere | Medium | Jump across repos, sessions, worktrees |
| Worktree list + switch | git worktree is increasingly common; any worktree tool must list and switch | Medium | Core to devctl's identity |
| Changed files list per worktree | Developers need to know what's dirty before switching context | Low | Basic git diff --stat output |
| Inline file diff | lazygit and gitui both show inline diffs; expected | Medium | Staged, unstaged, vs main |
| Process / command output visible | mprocs shows this; any tool that runs things must show output | Medium | At minimum, last command and exit code |
| Clear error states | Tools that fail silently lose trust immediately | Low | Error messages in UI, not just stderr |
| Fast startup | Terminal tools that take >1s to start get abandoned | Medium | Go binary is fast; SQLite cold open must be optimized |
| Persistent state across restarts | Users expect their sessions/repos not to disappear on restart | Medium | SQLite-backed; checkpoint files |
| Quit / exit that's obvious | tmux's C-b d vs C-b : kill-session confusion is a known pain point | Low | q or :q must work; confirm on destructive actions |

---

## Differentiators

Features that set devctl apart. Not baseline-expected in this category, but high value.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Session resurrection / crash recovery | tmux loses sessions on server restart; zellij does too; checkpoint-based recovery is rare | High | Checkpoint files + SQLite; serialize full session state |
| Cross-repo dependency graph | No existing tool tracks task/branch dependencies across repos | High | Core differentiation; nothing else does this |
| Blocked task surfacing | Automatically knowing "I can't start B until A is merged" is novel | High | Requires dependency model + git ancestry detection |
| Live AI session observability | No tool gives a live view into what Claude Code is doing | High | File watch + log streaming; novel feature category |
| Idle branch detection + auto-actions | Proactive "you've been idle 20 min, want a code review?" is new | High | Idle timer + agent integration |
| Draft patch approval workflow | Agent generates patches, human approves in-TUI | High | Novel; competes with nothing today |
| Work planning engine (ready/blocked/done) | Task execution graph in a terminal TUI | High | Project management meets git tooling |
| Single pane of glass for all dev activity | tmux shows terminals; devctl shows what's happening in those terminals | Medium | The "control plane" concept is new |
| Worktree-first design | git worktree is underutilized; devctl normalizes it as the unit of work | Medium | lazygit has minimal worktree support; devctl makes it first-class |
| Context switching with full state restoration | Not just switching terminal windows — restoring env, branch, task context | High | Differentiates from tmux/zellij which only restore windows |
| Inline diff with staged/unstaged toggle | lazygit does this; devctl doing it without leaving the orchestration view is valuable | Medium | Contextual diff without switching to a git TUI |

---

## Anti-Features

Features to explicitly NOT build in v1.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Built-in terminal emulator / shell | Competing with tmux/zellij is out of scope and adds massive complexity | Integrate alongside tmux/zellij; devctl is the control plane, not the shell |
| Cloud sync / team sharing | Single-developer local tool is the thesis; cloud sync adds auth, infra, privacy concerns | Defer entirely; local-only is a feature, not a limitation |
| Pull request creation / GitHub UI | gh cli does this better; duplication dilutes focus | Emit gh commands or link to gh; don't replicate |
| CI/CD status boards | Too broad; requires per-provider integrations (GHA, CircleCI, etc.) | Out of scope; link to external dashboards |
| Plugin / extension system | Premature; adds API surface to maintain before core is stable | Build opinionated features first; plugins after v1 ships |
| Built-in code editor | Helix, Neovim, VS Code are the editors; devctl opens files in $EDITOR | Always delegate to $EDITOR; never embed an editor |
| AI code generation (write code) | devctl observes AI sessions, does not replace the AI | Claude Code is the agent; devctl is the dashboard |
| Notification system (desktop popups) | Breaks the terminal-native contract; devctl is TUI-only | Use in-TUI status indicators; no OS notifications |
| Mouse-first UI | Terminal power users reject mouse-centric tools | Keyboard-first always; mouse as optional enhancement only |
| Per-file git blame / history | gitui and lazygit do this better; too deep into git-TUI territory | Defer to lazygit/gitui for deep git operations |
| Remote repository management (clone wizard) | Not a session problem | Use git clone directly; devctl tracks what's already cloned |
| Interactive rebase UI | Out of scope; lazygit owns this | Document "use lazygit for this" |

---

## Feature Dependencies

```
SQLite state DB
  → Session tracking (stores sessions)
  → Worktree manager (stores worktrees + repo mappings)
  → Task model (stores tasks)
  → Dependency model (stores task_dependencies)

Worktree manager
  → Dashboard worktree panel
  → Fast context switch (devctl jump)
  → Git status per worktree (must know worktrees to query git)
  → Session tracking (sessions linked to worktrees)

Session tracking
  → Session resurrection (sessions must be tracked before they can be checkpointed)
  → Dashboard session panel
  → Live AI observability (AI sessions are a session subtype)

Task model
  → Dependency detection (tasks must exist to have dependencies)
  → Work planning engine (planning queries task graph)
  → Blocked task detection (blocked state is derived from task dependencies)
  → Draft agent actions (agent actions are spawned from task context)

Git CLI integration
  → Git status per worktree
  → Inline diff viewer
  → Changed files list
  → Idle branch detection (checks last git commit time)
  → Branch ancestry detection (used for dependency inference)

Inline diff viewer
  → Staged / unstaged toggle (needs git integration)
  → File change visibility (drives what files are shown)

Idle detection
  → Draft agent actions (agent actions are triggered by idle state)
  → Draft patch generation (patches are the output of agent actions)

Draft patch generation
  → Patch approval workflow (approval requires a patch to exist)
  → Patch reversibility (reversibility requires patch is stored as git patch)

Live AI observability
  → Claude session docked window (the docked view needs the observability data stream)
```

---

## MVP Recommendation

Prioritize for a working v1 that earns trust:

1. **Dashboard with repo/worktree/session panels** — the north star; everything else is contextual
2. **Worktree manager** (create, list, switch, delete) — devctl's core unit of work
3. **Git status per worktree** (ahead/behind, staged, unstaged, changed files) — table stakes; without this the dashboard is not useful
4. **Session tracking** (start, stop, list, link to worktree) — establishes the session model all other features build on
5. **Fast context switch** (`devctl jump`) — high daily-use value, relatively contained implementation
6. **Inline file diff viewer** — completes the "single pane of glass" for file changes

Defer from MVP (build in later phases):

- **Session resurrection** — high value but high complexity; requires checkpoint reliability; defer until session tracking is stable
- **Dependency detection / work planning engine** — requires task model to be proven; high complexity; build after basic session/worktree loop is validated
- **Live AI observability + Claude docked window** — requires stable session model and file-watch infrastructure; build in a dedicated AI integration phase
- **Idle detection + draft agent actions + patch workflow** — highest complexity, most novel; build last; validate core tool value first

---

## Competitive Gaps (What the Market Is Missing)

These are the features that no existing tool provides, making them the highest-leverage investments for differentiation:

1. **Cross-repo, cross-worktree situational awareness** — tmux/zellij show windows; nothing shows what's in those windows in a structured way
2. **Task dependency graph with git-awareness** — project management tools (Linear, Jira) don't know about git; git tools don't know about tasks
3. **AI agent session observability** — Claude Code, Cursor, Aider all lack a "control plane" view; devctl fills this gap
4. **Proactive idle analysis with reversible patches** — no tool closes the loop from "I forgot about that branch" to "here's a draft code review you can approve"

---

## Sources

- Training knowledge of tmux (2.x), zellij (0.38-0.40), lazygit (0.40-0.41), gitui (0.24-0.26), mprocs (0.6.x) — confidence HIGH for well-established feature sets of these tools
- Training knowledge of git worktree CLI behavior — confidence HIGH
- AI agent observability (Claude Code integration patterns) — confidence MEDIUM; newer domain, patterns still emerging as of training cutoff
- DevCTL PROJECT.md — /Users/daniel/Projects/devctl/.planning/PROJECT.md
- Note: WebSearch and WebFetch were unavailable during this research session. Recommend verifying mprocs 0.7+ features and any zellij plugin ecosystem changes post-August 2025.
