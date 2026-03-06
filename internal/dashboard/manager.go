package dashboard

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/danielmiessler/devctl/internal/agent"
	"github.com/danielmiessler/devctl/internal/claude"
	"github.com/danielmiessler/devctl/internal/dependency"
	"github.com/danielmiessler/devctl/internal/git"
	"github.com/danielmiessler/devctl/internal/task"
	"github.com/danielmiessler/devctl/pkg/tui"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
)

// Manager owns the background goroutines and the event channel.
// All goroutines must check ctx.Done() and exit cleanly on cancellation.
type Manager struct {
	db           *sqlx.DB
	events       chan tui.StateEvent
	cancel       context.CancelFunc
	taskStore    *task.TaskStore
	depStore     *dependency.DepStore
	idleTracker  *agent.IdleTracker
	runStore     *agent.AgentRunStore
	patchStore   *agent.PatchStore
	runner       *agent.WorkflowRunner
}

// NewManager creates a Manager. Call Start() to begin background polling.
func NewManager(db *sqlx.DB) *Manager {
	m := &Manager{
		db:     db,
		events: make(chan tui.StateEvent, 32),
	}
	if db != nil {
		m.taskStore = task.NewStore(db)
		m.depStore = dependency.NewStore(db)

		cfg := agent.LoadConfig()
		if cfg.Enabled {
			m.runStore = agent.NewAgentRunStore(db)
			m.patchStore = agent.NewPatchStore(db)
			m.idleTracker = agent.NewIdleTracker(cfg)
			m.runner = agent.NewWorkflowRunner(m.runStore, m.patchStore, cfg)
		}
	}
	return m
}

// Start launches background goroutines. ctx is the root application context.
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	go m.pollLoop(ctx)
}

// Stop cancels all background goroutines. Safe to call multiple times.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Events returns a receive-only view of the state event channel.
func (m *Manager) Events() <-chan tui.StateEvent {
	return m.events
}

// PatchStore returns the patch store if agent features are enabled, or nil.
func (m *Manager) PatchStore() *agent.PatchStore {
	return m.patchStore
}

// pollLoop emits StateEvents every 5 seconds.
// On startup it emits the DB cache immediately (instant render), then fires a real poll
// right away so Claude-discovered sessions appear without waiting the full 5s.
func (m *Manager) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Phase 1: emit cached DB state instantly so the TUI isn't blank.
	m.emit(ctx, m.loadCachedSnapshot(ctx))

	// Phase 2: real poll immediately — discovers Claude projects + live git state.
	m.emit(ctx, m.pollAllWorktrees(ctx))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.emit(ctx, m.pollAllWorktrees(ctx))
		}
	}
}

// emit sends a StateEvent, dropping it if channel is full (TUI is lagging).
func (m *Manager) emit(ctx context.Context, snapshot tuimsg.StateSnapshot) {
	select {
	case m.events <- tui.StateEvent{Snapshot: snapshot}:
	case <-ctx.Done():
	default:
		// Channel full: TUI is lagging. Drop this tick to avoid blocking the poller.
		slog.Warn("state event channel full, dropping snapshot")
	}
}

// worktreeRow is a DB scan target for the worktrees + repos join.
type worktreeRow struct {
	ID       string `db:"id"`
	Path     string `db:"path"`
	Branch   string `db:"branch"`
	RepoPath string `db:"repo_path"`
}

// sessionThreshold reads the configurable active-session threshold.
func sessionThreshold() time.Duration {
	min := viper.GetInt("session.active_threshold_minutes")
	if min <= 0 {
		min = 20
	}
	return time.Duration(min) * time.Minute
}

// pollAllWorktrees combines DB-tracked worktrees and Claude-auto-discovered projects,
// polls git state for each, and returns a populated StateSnapshot.
func (m *Manager) pollAllWorktrees(ctx context.Context) tuimsg.StateSnapshot {
	threshold := sessionThreshold()
	seenPaths := make(map[string]bool)
	var states []tuimsg.WorktreeState

	// 1. DB-tracked worktrees — existing registered entries with full git state.
	if m.db != nil {
		rows, err := m.db.QueryxContext(ctx, `
            SELECT w.id, w.path, w.branch, r.path as repo_path
            FROM worktrees w JOIN repos r ON r.id = w.repo_id
        `)
		if err != nil {
			slog.Error("poll: query worktrees", "err", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var row worktreeRow
				if err := rows.StructScan(&row); err != nil {
					slog.Error("poll: scan worktree row", "err", err)
					continue
				}
				gitState, err := git.PollState(ctx, row.Path)
				if err != nil {
					slog.Warn("poll: git.PollState failed", "path", row.Path, "err", err)
					continue
				}
				ts := mapGitState(row.ID, gitState)
				ts.RepoPath = row.RepoPath
				ts.RepoName = filepath.Base(row.RepoPath)
				if sessions, err := claude.ScanSessionsWithThreshold(row.Path, threshold); err == nil {
					ts.Sessions = mapClaudeSessions(sessions)
				}
				seenPaths[row.Path] = true
				states = append(states, ts)
				m.persistState(ctx, row.ID, ts)
			}
		}
	}

	// 2. Claude-auto-discovered projects not already covered by the DB.
	claudeProjects, err := claude.ScanAllProjects(threshold)
	if err != nil {
		slog.Warn("poll: claude.ScanAllProjects failed", "err", err)
	}
	for _, proj := range claudeProjects {
		if seenPaths[proj.RepoPath] {
			continue
		}
		seenPaths[proj.RepoPath] = true

		ts := tuimsg.WorktreeState{
			WorktreeID:   "claude:" + proj.RepoPath,
			WorktreePath: proj.RepoPath,
			RepoPath:     proj.RepoPath,
			RepoName:     filepath.Base(proj.RepoPath),
			Branch:       filepath.Base(proj.RepoPath), // fallback; overwritten by git below
			Behind:       -1,
			Sessions:     mapClaudeSessions(proj.Sessions),
			PolledAt:     time.Now(),
		}
		// Enrich with live git state if the path exists on disk.
		if gitState, err := git.PollState(ctx, proj.RepoPath); err == nil {
			enriched := mapGitState("claude:"+proj.RepoPath, gitState)
			enriched.RepoPath = proj.RepoPath
			enriched.RepoName = filepath.Base(proj.RepoPath)
			enriched.Sessions = ts.Sessions
			ts = enriched
		}
		states = append(states, ts)
	}

	// Idle branch detection: check all worktrees for inactivity.
	if m.idleTracker != nil {
		commitTimes := make(map[string]time.Time)
		for _, ws := range states {
			if ws.RepoPath == "" || ws.Branch == "" {
				continue
			}
			key := ws.RepoPath + ":" + ws.Branch
			ct, err := git.LastCommitTime(ctx, ws.RepoPath, ws.Branch)
			if err == nil && !ct.IsZero() {
				commitTimes[key] = ct
			}
		}
		idleBranches := m.idleTracker.Check(states, commitTimes)
		for _, ib := range idleBranches {
			slog.Info("idle branch detected", "repo", ib.RepoPath, "branch", ib.Branch, "idle_since", ib.IdleSince)
			if m.runner != nil {
				m.runner.RunAsync(ctx, ib)
			}
		}
	}

	snapshot := tuimsg.StateSnapshot{UpdatedAt: time.Now(), Worktrees: states}

	// Resolve task graph if DB is available.
	if m.taskStore != nil {
		snapshot.TaskGraph = m.resolveTaskGraph(ctx, states)
	}

	// Collect agent patches if agent features are enabled.
	if m.patchStore != nil {
		snapshot.Patches = m.collectPatches(ctx)
	}

	return snapshot
}

// resolveTaskGraph fetches tasks and deps, checks branch merge status, and resolves the DAG.
func (m *Manager) resolveTaskGraph(ctx context.Context, worktreeStates []tuimsg.WorktreeState) tuimsg.TaskGraphSnapshot {
	tasks, err := m.taskStore.List(ctx)
	if err != nil {
		slog.Warn("poll: task list", "err", err)
		return tuimsg.TaskGraphSnapshot{}
	}
	deps, err := m.depStore.ListAll(ctx)
	if err != nil {
		slog.Warn("poll: dep list", "err", err)
		return tuimsg.TaskGraphSnapshot{}
	}
	if len(tasks) == 0 {
		return tuimsg.TaskGraphSnapshot{}
	}

	// Build repo path lookup from worktree states for branch merge checks.
	// Use the first worktree's repo path for each repo name.
	repoPathByID := make(map[string]string)
	for _, ws := range worktreeStates {
		if ws.RepoPath != "" {
			repoPathByID[ws.RepoPath] = ws.RepoPath
		}
	}

	// Build branchMerged map: only check tasks that are queued/running with a branch.
	branchMerged := make(map[string]bool)
	checked := make(map[string]bool) // avoid redundant subprocess calls
	for _, t := range tasks {
		if t.Branch == "" || t.State == "completed" {
			continue
		}
		if checked[t.Branch] {
			continue
		}
		checked[t.Branch] = true

		// Find a repo path for this task. Try worktree states for a matching repo.
		var repoPath string
		for _, ws := range worktreeStates {
			if ws.RepoPath != "" {
				repoPath = ws.RepoPath
				break
			}
		}
		if repoPath == "" {
			continue
		}

		defaultBranch := git.DefaultBranch(ctx, repoPath)
		merged, err := git.IsBranchMerged(ctx, repoPath, t.Branch, defaultBranch)
		if err != nil {
			slog.Warn("poll: branch merge check", "branch", t.Branch, "err", err)
			continue
		}
		branchMerged[t.ID] = merged
	}

	resolved, err := task.Resolve(tasks, deps, branchMerged)
	hasCycle := err != nil
	return tuimsg.TaskGraphSnapshot{
		Tasks:    mapResolvedTasks(resolved),
		HasCycle: hasCycle,
	}
}

// mapResolvedTasks converts internal task.ResolvedTask to tuimsg.ResolvedTask.
// BlockedBy IDs are truncated to 8 characters for display.
func mapResolvedTasks(resolved []task.ResolvedTask) []tuimsg.ResolvedTask {
	result := make([]tuimsg.ResolvedTask, len(resolved))
	for i, rt := range resolved {
		blockedBy := make([]string, len(rt.BlockedBy))
		for j, id := range rt.BlockedBy {
			if len(id) > 8 {
				id = id[:8]
			}
			blockedBy[j] = id
		}
		result[i] = tuimsg.ResolvedTask{
			ID:          rt.Task.ID,
			Description: rt.Task.Description,
			State:       rt.Task.State,
			Branch:      rt.Task.Branch,
			IsReady:     rt.IsReady,
			IsBlocked:   rt.IsBlocked,
			BlockedBy:   blockedBy,
			Layer:       rt.Layer,
		}
	}
	return result
}

// mapGitState converts git.WorktreeState to tuimsg.WorktreeState.
// This mapping lives in Manager (not in git or tuimsg) to keep both packages clean.
func mapGitState(worktreeID string, gs git.WorktreeState) tuimsg.WorktreeState {
	changed := make([]tuimsg.ChangedFile, len(gs.ChangedFiles))
	for i, cf := range gs.ChangedFiles {
		changed[i] = tuimsg.ChangedFile{
			Path:           cf.Path,
			StagedStatus:   cf.StagedStatus,
			UnstagedStatus: cf.UnstagedStatus,
		}
	}
	return tuimsg.WorktreeState{
		WorktreeID:   worktreeID,
		WorktreePath: gs.WorktreePath,
		Branch:       gs.Branch,
		Ahead:        gs.Ahead,
		Behind:       gs.Behind,
		Staged:       gs.Staged,
		Unstaged:     gs.Unstaged,
		Untracked:    gs.Untracked,
		ChangedFiles: changed,
		PolledAt:     time.Now(),
	}
}

// mapClaudeSessions converts claude.Session slice to tuimsg.ClaudeSession slice.
func mapClaudeSessions(sessions []claude.Session) []tuimsg.ClaudeSession {
	result := make([]tuimsg.ClaudeSession, len(sessions))
	for i, s := range sessions {
		result[i] = tuimsg.ClaudeSession{
			ID:             s.ID,
			ProjectPath:    s.ProjectPath,
			Slug:           s.Slug,
			LastActivity:   s.LastActivity,
			IsActive:       s.IsActive,
			LastMessage:    s.LastMessage,
			RecentFiles:    s.RecentFiles,
			CurrentTool:    s.CurrentTool,
			CurrentCommand: s.CurrentCommand,
		}
	}
	return result
}

// collectPatches queries the patch store for all reviewable patches (draft, approved, applied).
func (m *Manager) collectPatches(ctx context.Context) tuimsg.PatchSnapshot {
	var allPatches []agent.AgentPatch
	for _, status := range []string{"draft", "approved", "applied"} {
		patches, err := m.patchStore.ListByStatus(ctx, status)
		if err != nil {
			slog.Warn("poll: patch list", "status", status, "err", err)
			continue
		}
		allPatches = append(allPatches, patches...)
	}
	return tuimsg.PatchSnapshot{Patches: mapAgentPatches(allPatches)}
}

// mapAgentPatches converts internal agent.AgentPatch to tuimsg.AgentPatch.
func mapAgentPatches(patches []agent.AgentPatch) []tuimsg.AgentPatch {
	result := make([]tuimsg.AgentPatch, len(patches))
	for i, p := range patches {
		desc := ""
		if p.Description != nil {
			desc = *p.Description
		}
		result[i] = tuimsg.AgentPatch{
			ID:          p.ID,
			RunID:       p.RunID,
			RepoPath:    p.RepoPath,
			Branch:      p.Branch,
			Title:       p.Title,
			Description: desc,
			PatchData:   p.PatchData,
			Status:      p.Status,
			CreatedAt:   time.Unix(p.CreatedAt, 0),
		}
	}
	return result
}

// persistState writes polled state to the worktree_state cache table.
func (m *Manager) persistState(ctx context.Context, worktreeID string, ts tuimsg.WorktreeState) {
	_, err := m.db.ExecContext(ctx, `
        INSERT INTO worktree_state (worktree_id, branch, ahead, behind, staged, unstaged, untracked, polled_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(worktree_id) DO UPDATE SET
            branch=excluded.branch, ahead=excluded.ahead, behind=excluded.behind,
            staged=excluded.staged, unstaged=excluded.unstaged, untracked=excluded.untracked,
            polled_at=excluded.polled_at
    `, worktreeID, ts.Branch, ts.Ahead, ts.Behind, ts.Staged, ts.Unstaged, ts.Untracked,
		ts.PolledAt.Unix())
	if err != nil {
		slog.Error("persist state", "worktree_id", worktreeID, "err", err)
	}
}

// loadCachedSnapshot reads the worktree_state cache from DB for instant startup rendering.
func (m *Manager) loadCachedSnapshot(ctx context.Context) tuimsg.StateSnapshot {
	if m.db == nil {
		return tuimsg.StateSnapshot{UpdatedAt: time.Now()}
	}
	rows, err := m.db.QueryxContext(ctx, `
        SELECT w.id, w.path, w.branch as wt_branch, r.path as repo_path,
               COALESCE(ws.branch, w.branch) as branch,
               COALESCE(ws.ahead, 0) as ahead,
               COALESCE(ws.behind, -1) as behind,
               COALESCE(ws.staged, 0) as staged,
               COALESCE(ws.unstaged, 0) as unstaged,
               COALESCE(ws.untracked, 0) as untracked,
               COALESCE(ws.polled_at, 0) as polled_at
        FROM worktrees w
        JOIN repos r ON r.id = w.repo_id
        LEFT JOIN worktree_state ws ON ws.worktree_id = w.id
    `)
	if err != nil {
		slog.Error("load cached state", "err", err)
		return tuimsg.StateSnapshot{UpdatedAt: time.Now()}
	}
	defer rows.Close()

	var states []tuimsg.WorktreeState
	for rows.Next() {
		var (
			id, path, wtBranch, repoPath, branch        string
			ahead, behind, staged, unstaged, untracked int
			polledAt                                    int64
		)
		if err := rows.Scan(&id, &path, &wtBranch, &repoPath, &branch, &ahead, &behind, &staged, &unstaged, &untracked, &polledAt); err != nil {
			slog.Error("load cached state: scan", "err", err)
			continue
		}
		states = append(states, tuimsg.WorktreeState{
			WorktreeID:   id,
			WorktreePath: path,
			Branch:       branch,
			Ahead:        ahead,
			Behind:       behind,
			Staged:       staged,
			Unstaged:     unstaged,
			Untracked:    untracked,
			PolledAt:     time.Unix(polledAt, 0),
		})
	}
	return tuimsg.StateSnapshot{UpdatedAt: time.Now(), Worktrees: states}
}
