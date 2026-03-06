package agent

import (
	"sync"
	"time"

	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// IdleBranch represents a branch that has been idle longer than the configured threshold.
type IdleBranch struct {
	RepoPath  string
	Branch    string
	IdleSince time.Time
}

// IdleTracker monitors worktree activity and detects idle branches.
// It is NOT safe for concurrent use — Manager calls it from a single goroutine.
type IdleTracker struct {
	mu           sync.Mutex
	lastActivity map[string]time.Time // branchKey -> last known activity
	triggered    map[string]time.Time // branchKey -> when we last triggered
	config       AgentConfig
}

// NewIdleTracker creates an IdleTracker with the given configuration.
func NewIdleTracker(cfg AgentConfig) *IdleTracker {
	return &IdleTracker{
		lastActivity: make(map[string]time.Time),
		triggered:    make(map[string]time.Time),
		config:       cfg,
	}
}

// branchKey returns a unique key for a repo+branch combination.
func branchKey(repoPath, branch string) string {
	return repoPath + ":" + branch
}

// Check examines worktree states and returns branches that have been idle
// longer than the configured threshold. commitTimes maps branchKey to the
// time of the last commit on that branch.
func (t *IdleTracker) Check(states []tuimsg.WorktreeState, commitTimes map[string]time.Time) []IdleBranch {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	var idle []IdleBranch

	for _, ws := range states {
		if t.config.IsRepoDisabled(ws.RepoPath) {
			continue
		}

		key := branchKey(ws.RepoPath, ws.Branch)

		// Compute latest activity as max of: session LastActivity times,
		// commit time for branch, polledAt as fallback.
		latest := ws.PolledAt
		if ct, ok := commitTimes[key]; ok && ct.After(latest) {
			latest = ct
		}
		for _, sess := range ws.Sessions {
			if sess.LastActivity.After(latest) {
				latest = sess.LastActivity
			}
		}

		t.lastActivity[key] = latest

		// If activity is recent, reset any trigger state (branch woke up).
		if now.Sub(latest) < t.config.IdleThreshold() {
			delete(t.triggered, key)
			continue
		}

		// Branch is idle. Check cooldown before triggering.
		if triggeredAt, ok := t.triggered[key]; ok {
			if now.Sub(triggeredAt) < t.config.Cooldown() {
				continue // still in cooldown
			}
		}

		// Trigger.
		t.triggered[key] = now
		idle = append(idle, IdleBranch{
			RepoPath:  ws.RepoPath,
			Branch:    ws.Branch,
			IdleSince: latest,
		})
	}

	return idle
}

// ResetBranch manually resets the trigger state for a branch, e.g. when
// activity resumes or the user intervenes.
func (t *IdleTracker) ResetBranch(repoPath, branch string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.triggered, branchKey(repoPath, branch))
}
