package dashboard

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielmiessler/devctl/internal/git"
	"github.com/danielmiessler/devctl/pkg/tui"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
	"github.com/jmoiron/sqlx"
)

// Manager owns the background goroutines and the event channel.
// All goroutines must check ctx.Done() and exit cleanly on cancellation.
type Manager struct {
	db     *sqlx.DB
	events chan tui.StateEvent
	cancel context.CancelFunc
}

// NewManager creates a Manager. Call Start() to begin background polling.
func NewManager(db *sqlx.DB) *Manager {
	return &Manager{
		db:     db,
		events: make(chan tui.StateEvent, 32),
	}
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

// pollLoop emits StateEvents with real git data every 5 seconds.
// On first iteration it emits cached state immediately, then polls live.
func (m *Manager) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Emit cached state immediately so TUI renders on startup without waiting 5s.
	snapshot := m.loadCachedSnapshot(ctx)
	m.emit(ctx, snapshot)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := m.pollAllWorktrees(ctx)
			m.emit(ctx, snapshot)
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

// pollAllWorktrees queries all tracked worktrees, polls git state for each,
// persists results to worktree_state, and returns a populated StateSnapshot.
func (m *Manager) pollAllWorktrees(ctx context.Context) tuimsg.StateSnapshot {
	if m.db == nil {
		return tuimsg.StateSnapshot{UpdatedAt: time.Now()}
	}
	rows, err := m.db.QueryxContext(ctx, `
        SELECT w.id, w.path, w.branch, r.path as repo_path
        FROM worktrees w JOIN repos r ON r.id = w.repo_id
    `)
	if err != nil {
		slog.Error("poll: query worktrees", "err", err)
		return tuimsg.StateSnapshot{UpdatedAt: time.Now()}
	}
	defer rows.Close()

	var states []tuimsg.WorktreeState
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
		states = append(states, ts)
		m.persistState(ctx, row.ID, ts)
	}

	return tuimsg.StateSnapshot{
		UpdatedAt: time.Now(),
		Worktrees: states,
	}
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
