# devctl

Developer control plane for repos, worktrees, Claude sessions, and tasks.

One TUI to see everything happening across all your repos — no lost sessions, no forgotten branches, no missed follow-ups.

## Features

- **Repo & worktree tracking** — register repos, manage git worktrees
- **Claude session visibility** — see all active/recent Claude Code sessions per worktree
- **Side-quest / main-quest system** — spawn parallel Claude sessions in worktrees with context from the parent session, then incorporate findings back
- **Task & dependency graph** — DAG-based task tracking with topological ordering
- **Patch review** — approve/reject patches from the TUI
- **Idea pipeline** — queue ideas with dependencies, auto-launch when ready

## Install

### Homebrew (macOS/Linux)

```bash
brew install DanielMS93/tap/devctl
```

### Go install

Requires Go 1.25+:

```bash
go install github.com/DanielMS93/devctl/cmd/devctl@latest
```

### From source

```bash
git clone https://github.com/DanielMS93/devctl.git
cd devctl
make install
```

### GitHub Releases

Download pre-built binaries from [Releases](https://github.com/DanielMS93/devctl/releases).

## Quick start

```bash
# Open the dashboard (creates ~/.devctl/ on first run)
devctl

# Register a repo
devctl repo add /path/to/your/repo

# Manage worktrees
devctl worktree create myrepo feature-branch
devctl worktree list

# Manage ideas
devctl idea add "investigate Redis caching" --repo myrepo
devctl idea list
```

## TUI keybindings

| Key | Action |
|-----|--------|
| `tab` / mouse click | Switch panels |
| `j` / `k` | Navigate up/down |
| `enter` | Dive in / resume session |
| `n` | New Claude session in selected worktree |
| `s` | Spawn side-quest from selected session |
| `l` | Live session transcript viewer |
| `i` | Toggle idea pipeline panel |
| `p` | Toggle patch review panel |
| `t` | Toggle task graph |
| `d` | Diff view for selected file |
| `f` | Full file view |
| `a` | Toggle all sessions / approve patch |
| `x` | Reject patch |
| `esc` | Close overlay / go back |
| `q` | Quit |

## Side-quest system

The quest system lets you spawn parallel Claude sessions that run in isolated git worktrees with the parent session's transcript as context.

### Setup

Add the MCP server to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "devctl": {
      "command": "devctl",
      "args": ["mcp", "serve"]
    }
  }
}
```

Copy the slash commands from `.claude/commands/` into your project.

### Usage

From within a Claude Code session:

- **`/side-quest "investigate X"`** — spawns a parallel Claude session in a new worktree with your current session's transcript as context
- **`/main-quest`** — incorporates a completed side-quest's findings back (merges code changes or returns transcript)

From the TUI:

- Select a session and press `s` to spawn a side-quest

From the CLI:

```bash
devctl idea add "investigate Redis caching" --repo myrepo
devctl idea add "refactor auth" --depends-on <idea-id>
devctl idea list
devctl idea show <id>
```

## Configuration

Optional config file at `~/.devctl/config.yaml`:

```yaml
session:
  active_threshold_minutes: 20

idea:
  max_transcript_chars: 50000
```

## Architecture

- **TUI**: Bubbletea v2 + Lipgloss v2
- **Storage**: SQLite (WAL mode) at `~/.devctl/state.db`
- **Background polling**: 5-second cadence for git state, sessions, and idea pipeline
- **MCP server**: stdio JSON-RPC for Claude Code integration

## License

MIT
