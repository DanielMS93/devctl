---
phase: 01-foundation
plan: 01
subsystem: database
tags: [go, sqlite, modernc, sqlx, golang-migrate, bubbletea, wal]

# Dependency graph
requires: []
provides:
  - Go module github.com/danielmiessler/devctl with full dependency set
  - storage.Open() with WAL-mode SQLite and single-writer connection pooling
  - RunMigrations() with embedded SQL migrations via golang-migrate + iofs
  - Initial schema: repos and worktrees tables with FK and index
affects: [02-worktree-manager, 03-tui-shell, 04-session-tracking, 05-dependency-graph, 06-agent-integration]

# Tech tracking
tech-stack:
  added:
    - "charm.land/bubbletea/v2 v2.0.1 - TUI framework"
    - "charm.land/lipgloss/v2 v2.0.0 - TUI styling"
    - "charm.land/bubbles/v2 v2.0.0 - TUI components"
    - "github.com/spf13/cobra v1.9.0 - CLI framework"
    - "github.com/spf13/viper v1.21.0 - config management"
    - "modernc.org/sqlite v1.18.1 - CGO-free SQLite driver"
    - "github.com/jmoiron/sqlx v1.4.0 - SQL extensions for Go"
    - "github.com/golang-migrate/migrate/v4 v4.19.1 - migration runner"
    - "github.com/stretchr/testify v1.10.0 - test assertions"
  patterns:
    - "Single-writer SQLite: SetMaxOpenConns(1) + SetMaxIdleConns(1) before any query"
    - "WAL pragma sequence: journal_mode=WAL, synchronous=NORMAL, foreign_keys=ON, busy_timeout=5000"
    - "Embedded migrations: //go:embed migrations/*.sql + iofs.New() source"
    - "tools.go with //go:build tools tag to retain future-use deps through go mod tidy"

key-files:
  created:
    - go.mod
    - go.sum
    - tools.go
    - cmd/devctl/main.go
    - pkg/storage/storage.go
    - pkg/storage/migrate.go
    - pkg/storage/migrations/001_initial.up.sql
    - pkg/storage/migrations/001_initial.down.sql
    - .gitignore
  modified: []

key-decisions:
  - "modernc.org/sqlite chosen over mattn/go-sqlite3: no CGO, pure Go, cross-platform binary"
  - "golang-migrate/database/sqlite (not sqlite3): the 'sqlite' driver wraps modernc, 'sqlite3' would pull CGO"
  - "tools.go with build tag: retains TUI/CLI deps in go.mod across tidy cycles before they are imported"
  - "SetMaxOpenConns(1) enforced before pragmas: prevents any concurrent writers from racing"
  - "RunMigrations() takes dbPath string (not *sqlx.DB): golang-migrate needs its own DSN connection"

patterns-established:
  - "Storage pattern: Open() for app connection, RunMigrations() called once at startup before Open()"
  - "Pragma pattern: WAL first, then synchronous, then FK, then timeout — order matters for WAL activation"
  - "Migration DSN pattern: sqlite://+dbPath (two slashes, no host, relative/absolute path)"

# Metrics
duration: 9min
completed: 2026-03-05
---

# Phase 1 Plan 01: Foundation Summary

**WAL-mode SQLite with embedded golang-migrate migrations, CGO-free modernc driver, and full Charm v2 TUI dependency set declared in go.mod**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-05T07:14:25Z
- **Completed:** 2026-03-05T07:23:43Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments

- Go module initialized with complete dependency set (Charm v2 TUI stack, cobra, viper, modernc SQLite, sqlx, golang-migrate, testify)
- storage.Open() establishes WAL-mode SQLite with single-writer connection pool and all four required pragmas
- RunMigrations() applies embedded SQL migrations via iofs source — no external SQL files needed at runtime
- Initial schema creates repos and worktrees tables with FK cascade and repo_id index

## Task Commits

Each task was committed atomically:

1. **Task 1: Initialize Go module and install all dependencies** - `56f85d5` (chore)
2. **Task 2: Implement storage.Open() with WAL pragmas and RunMigrations()** - `c6d3d25` (feat)

**Plan metadata:** (docs commit follows this summary)

## Files Created/Modified

- `go.mod` - Module declaration with all dependencies including Charm v2 stack and modernc.org/sqlite
- `go.sum` - Dependency checksums
- `tools.go` - Build-tag-gated imports to retain future-use deps through go mod tidy cycles
- `cmd/devctl/main.go` - Minimal stub; replaced in Plan 03
- `pkg/storage/storage.go` - Open() with WAL pragmas and SetMaxOpenConns(1)
- `pkg/storage/migrate.go` - RunMigrations() with embed + iofs + golang-migrate/database/sqlite
- `pkg/storage/migrations/001_initial.up.sql` - repos and worktrees DDL
- `pkg/storage/migrations/001_initial.down.sql` - DROP statements for rollback
- `.gitignore` - Excludes compiled binaries and SQLite WAL files

## Decisions Made

- **modernc.org/sqlite** over mattn/go-sqlite3: pure Go, no CGO, enables single-binary distribution without Xcode/gcc dependencies
- **golang-migrate/database/sqlite** (not sqlite3): the `sqlite` driver wraps modernc; `sqlite3` would pull in the CGO mattn driver and break the no-CGO constraint
- **tools.go pattern**: build-tag-gated file preserves TUI/CLI deps in go.mod before they are imported by actual code in later plans, avoiding repeated `go get` cycles
- **RunMigrations takes dbPath**: golang-migrate opens its own connection internally; passing *sqlx.DB would require driver-specific type assertions

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Installed Go via Homebrew — not present on system**
- **Found during:** Task 1 (go mod init)
- **Issue:** `go` command not found; Go not installed
- **Fix:** `brew install go` (installed Go 1.26.0)
- **Files modified:** none (system-level install)
- **Verification:** `go version` returns `go1.26.0 darwin/arm64`
- **Committed in:** N/A (system install)

**2. [Rule 2 - Missing Critical] Added tools.go to retain dependencies through go mod tidy**
- **Found during:** Task 1 (go mod tidy)
- **Issue:** go mod tidy removed all dependencies not imported by actual code; the plan requires bubbletea/v2, lipgloss/v2, cobra, viper, testify in go.mod before they have importers
- **Fix:** Created tools.go with `//go:build tools` constraint importing all future-use packages as blank imports
- **Files modified:** tools.go (new file)
- **Verification:** go.mod retains all required entries after tidy; `go build ./...` passes
- **Committed in:** 56f85d5 (Task 1 commit)

**3. [Rule 2 - Missing Critical] Added .gitignore to exclude compiled binary and SQLite WAL files**
- **Found during:** Task 1 (git status showed compiled `devctl` binary in root)
- **Issue:** `go build ./...` outputs binary to root; would be committed without .gitignore
- **Fix:** Created .gitignore excluding /devctl binary and *.db/*.db-shm/*.db-wal
- **Files modified:** .gitignore (new file)
- **Verification:** git status no longer shows devctl binary
- **Committed in:** 56f85d5 (Task 1 commit)

---

**Total deviations:** 3 auto-fixed (1 blocking, 2 missing critical)
**Impact on plan:** All auto-fixes necessary for correctness or completeness. No scope creep.

## Issues Encountered

- `go mod tidy` with no importers empties go.mod — resolved by tools.go pattern
- .gitignore entry `devctl` matched directory `cmd/devctl` — fixed by anchoring to root with `/devctl`

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- storage.Open() and RunMigrations() ready for Plan 02 (worktree manager) to consume
- All TUI/CLI deps available for Plan 03 (TUI shell) without additional go get
- WAL pattern established: all future DB code must use SetMaxOpenConns(1) and call RunMigrations() before any queries

## Self-Check: PASSED

- All 9 files exist on disk
- Commits 56f85d5 and c6d3d25 verified in git log
- `go build ./...` passes with zero errors

---
*Phase: 01-foundation*
*Completed: 2026-03-05*
