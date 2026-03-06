---
phase: 06-ai-observability
plan: 04
subsystem: agent
tags: [idle-detection, workflow-runner, background-agent, shell-exec]

# Dependency graph
requires:
  - phase: 06-01
    provides: AgentRunStore, PatchStore, AgentConfig with thresholds and workflows
provides:
  - IdleTracker detecting inactive branches beyond configurable threshold
  - WorkflowRunner executing shell commands with timeout and patch capture
  - Manager integration polling idle state on each 5s cycle
affects: [06-05, 06-06]

# Tech tracking
tech-stack:
  added: []
  patterns: [idle-detection-with-cooldown, async-workflow-execution, diff-detection-heuristic]

key-files:
  created:
    - internal/agent/idle.go
    - internal/agent/idle_test.go
    - internal/agent/runner.go
  modified:
    - internal/git/git.go
    - internal/dashboard/manager.go

key-decisions:
  - "IdleTracker uses mutex for thread safety even though Manager calls from single goroutine (defensive)"
  - "Workflow commands run via sh -c with 5-minute timeout context"
  - "Diff detection uses simple heuristic (contains --- and +++) rather than full unified diff parser"
  - "Cooldown reset on activity resumption allows re-triggering after branch goes idle again"

patterns-established:
  - "branchKey format: repoPath + colon + branch for map lookups"
  - "RunAsync pattern: Manager owns goroutine lifecycle, runner just wraps in go func"

# Metrics
duration: 3min
completed: 2026-03-06
---

# Phase 6 Plan 4: Idle Detection and Workflow Triggering Summary

**IdleTracker with configurable threshold/cooldown detecting inactive branches, WorkflowRunner executing shell commands with diff capture, integrated into Manager poll loop**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-06T14:30:48Z
- **Completed:** 2026-03-06T14:34:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- IdleTracker detects branches with no commits or session activity beyond configurable threshold
- Cooldown prevents re-triggering same branch; activity resumption resets cooldown state
- WorkflowRunner executes enabled workflows as shell commands with 5-minute timeout
- Diff-like output automatically stored as draft AgentPatch records
- Manager polls LastCommitTime per branch and feeds IdleTracker on each cycle
- 7 unit tests covering threshold, cooldown, reset, disabled repos, commit/session activity

## Task Commits

Each task was committed atomically:

1. **Task 1: IdleTracker + git last commit time** - `1835d1f` (feat)
2. **Task 2: WorkflowRunner + Manager integration** - `502c156` (feat)

## Files Created/Modified
- `internal/agent/idle.go` - IdleTracker with Check(), ResetBranch(), branchKey helper
- `internal/agent/idle_test.go` - 7 tests for idle detection logic
- `internal/agent/runner.go` - WorkflowRunner with RunWorkflows(), RunAsync(), looksLikeDiff()
- `internal/git/git.go` - Added LastCommitTime() querying git log for branch tip timestamp
- `internal/dashboard/manager.go` - Agent stores, IdleTracker, WorkflowRunner initialized; idle detection in poll loop

## Decisions Made
- IdleTracker uses mutex for thread safety even though Manager calls from single goroutine (defensive design)
- Workflow commands run via `sh -c` with 5-minute timeout context
- Diff detection uses simple heuristic (contains `---` and `+++`) rather than full unified diff parser
- Cooldown reset on activity resumption allows re-triggering after branch goes idle again
- LastCommitTime returns zero time (not error) when branch/ref not found, keeping idle detection lenient

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Idle detection and workflow triggering are functional
- Agent runs and patches stored in DB, ready for TUI display (Plan 05) and CLI management (Plan 06)

---
*Phase: 06-ai-observability*
*Completed: 2026-03-06*
