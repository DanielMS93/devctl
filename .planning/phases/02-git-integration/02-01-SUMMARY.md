---
phase: 02-git-integration
plan: 01
subsystem: git
tags: [git, subprocess, worktree, porcelain, exec, parsing]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Go module with cobra, bubbletea, sqlx — compile environment ready"
provides:
  - "internal/git package: run() helper with context + dir + stderr wrapping"
  - "ListWorktrees, AddWorktree, RemoveWorktree — git worktree porcelain parsing"
  - "PollState — git status --porcelain=v2 --branch -> WorktreeState with ahead/behind/staged/unstaged/untracked"
  - "Diff — 4 modes (unstaged, staged, vs-main, vs-origin) with --color=always"
  - "6 fixture-based unit tests covering all parser branches (no live git required)"
affects: [02-02-schema, 02-03-worktree-cli, 02-04-polling, 02-05-left-panel, 02-06-viewer]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Git subprocess wrapper: run(ctx, dir, args) always sets cmd.Dir; no raw strings escape package boundary"
    - "Porcelain v2 parsing: stanza split on \\n\\n for worktrees; line prefix dispatch for status"
    - "Behind=-1 sentinel: no upstream tracking branch; never 0 which is ambiguous"
    - "DiffMode enum: typed constants prevent invalid mode values at compile time"
    - "--color=always: always passed to git diff; git strips ANSI when stdout is non-TTY"

key-files:
  created:
    - internal/git/git.go
    - internal/git/worktree.go
    - internal/git/state.go
    - internal/git/diff.go
    - internal/git/git_test.go
  modified: []

key-decisions:
  - "git CLI subprocesses (not go-git): go-git v5 lacks linked worktree support; subprocess is the correct path"
  - "Behind=-1 sentinel not 0: git omits branch.ab line entirely when no upstream; 0 would be ambiguous"
  - "run() takes dir string not os.File: simpler API; callers pass worktree path directly"
  - "Diff returns []byte not string: raw ANSI bytes passed directly to viewport SetContent; no unnecessary conversion"

patterns-established:
  - "Pattern: Every git op sets cmd.Dir = dir via shared run() — no git command runs in process CWD"
  - "Pattern: Fixture-based unit tests for all parsers — tests run without live git, no flakiness"
  - "Pattern: Package boundary enforced — typed structs (Worktree, WorktreeState, ChangedFile) only; no raw strings outside git package"

# Metrics
duration: 2min
completed: 2026-03-05
---

# Phase 2 Plan 01: internal/git Package Summary

**Thin subprocess wrapper around git CLI producing typed Go structs from porcelain output — zero raw strings escape the package boundary**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-03-05T13:30:20Z
- **Completed:** 2026-03-05T13:31:49Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- `internal/git` package complete with all exports required by plans 02-02 through 02-07
- 6 fixture-based parser unit tests pass without live git — no flakiness, fast CI
- `go build ./...` and `go vet ./internal/git/...` both clean

## Task Commits

Each task was committed atomically:

1. **Task 1: Create internal/git package with run() helper and worktree operations** - `35dd0dc` (feat)
2. **Task 2: Create state.go (PollState) and diff.go (Diff), add unit tests** - `bb7a05e` (feat)

## Files Created/Modified

- `internal/git/git.go` - Shared `run()` helper: exec.CommandContext with Dir, error wrapping with stderr
- `internal/git/worktree.go` - Worktree struct + ListWorktrees, AddWorktree, RemoveWorktree + parseWorktrees
- `internal/git/state.go` - WorktreeState, ChangedFile structs + PollState, parseStatus
- `internal/git/diff.go` - DiffMode constants + Diff() returning raw ANSI bytes for 4 modes
- `internal/git/git_test.go` - 6 unit tests: TestParseWorktrees_* (3) + TestParseStatus_* (3)

## Decisions Made

- **git CLI not go-git:** go-git v5 has no linked worktree support; subprocess is the only correct path
- **Behind=-1 sentinel:** git omits `# branch.ab` line entirely when no upstream configured; -1 distinguishes "no upstream" from "zero behind"
- **Diff returns `[]byte`:** Raw ANSI bytes passed directly to viewport `SetContent()`; string conversion deferred to caller if needed
- **parseWorktrees handles empty stanzas:** `bytes.TrimSpace` before length check prevents empty Worktree entries from trailing newlines

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `internal/git` package is complete and tested; all dependent plans can proceed
- Plans 02-02 (schema migration) and 02-03 (worktree CLI) can begin immediately
- Plan 02-04 (Manager polling) can begin after 02-02 completes
- All exports verified: Worktree, WorktreeState, ChangedFile, DiffMode, DiffUnstaged, DiffStaged, DiffVsMain, DiffVsOrigin, ListWorktrees, AddWorktree, RemoveWorktree, PollState, Diff

---
*Phase: 02-git-integration*
*Completed: 2026-03-05*
