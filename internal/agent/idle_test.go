package agent

import (
	"testing"
	"time"

	"github.com/DanielMS93/devctl/pkg/tui/tuimsg"
)

func defaultTestConfig() AgentConfig {
	return AgentConfig{
		Enabled:              true,
		IdleThresholdMinutes: 1, // 1 minute for fast tests
		CooldownMinutes:      5,
		Workflows:            map[string]WorkflowConfig{},
		DisabledRepos:        nil,
	}
}

func TestIdleTracker_IdleBranch(t *testing.T) {
	cfg := defaultTestConfig()
	tracker := NewIdleTracker(cfg)

	oldTime := time.Now().Add(-10 * time.Minute) // well past 1-minute threshold
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/a",
			Branch:   "main",
			PolledAt: oldTime,
		},
	}

	idle := tracker.Check(states, nil)
	if len(idle) != 1 {
		t.Fatalf("expected 1 idle branch, got %d", len(idle))
	}
	if idle[0].RepoPath != "/repo/a" || idle[0].Branch != "main" {
		t.Errorf("unexpected idle branch: %+v", idle[0])
	}
}

func TestIdleTracker_ActiveBranch(t *testing.T) {
	cfg := defaultTestConfig()
	tracker := NewIdleTracker(cfg)

	recentTime := time.Now().Add(-10 * time.Second) // within 1-minute threshold
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/a",
			Branch:   "main",
			PolledAt: recentTime,
		},
	}

	idle := tracker.Check(states, nil)
	if len(idle) != 0 {
		t.Fatalf("expected 0 idle branches, got %d", len(idle))
	}
}

func TestIdleTracker_Cooldown(t *testing.T) {
	cfg := defaultTestConfig()
	tracker := NewIdleTracker(cfg)

	oldTime := time.Now().Add(-10 * time.Minute)
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/a",
			Branch:   "main",
			PolledAt: oldTime,
		},
	}

	// First call should trigger.
	idle := tracker.Check(states, nil)
	if len(idle) != 1 {
		t.Fatalf("first check: expected 1 idle, got %d", len(idle))
	}

	// Second call within cooldown should NOT trigger.
	idle = tracker.Check(states, nil)
	if len(idle) != 0 {
		t.Fatalf("second check (cooldown): expected 0 idle, got %d", len(idle))
	}
}

func TestIdleTracker_CooldownReset(t *testing.T) {
	cfg := defaultTestConfig()
	tracker := NewIdleTracker(cfg)

	oldTime := time.Now().Add(-10 * time.Minute)
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/a",
			Branch:   "main",
			PolledAt: oldTime,
		},
	}

	// First trigger.
	idle := tracker.Check(states, nil)
	if len(idle) != 1 {
		t.Fatalf("first check: expected 1 idle, got %d", len(idle))
	}

	// Activity resumes (recent polledAt).
	states[0].PolledAt = time.Now().Add(-5 * time.Second)
	idle = tracker.Check(states, nil)
	if len(idle) != 0 {
		t.Fatalf("activity resumed: expected 0 idle, got %d", len(idle))
	}

	// Goes idle again — should trigger even though cooldown hasn't elapsed,
	// because the trigger state was reset when activity resumed.
	states[0].PolledAt = time.Now().Add(-10 * time.Minute)
	idle = tracker.Check(states, nil)
	if len(idle) != 1 {
		t.Fatalf("idle again after reset: expected 1 idle, got %d", len(idle))
	}
}

func TestIdleTracker_DisabledRepos(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.DisabledRepos = []string{"/repo/disabled"}
	tracker := NewIdleTracker(cfg)

	oldTime := time.Now().Add(-10 * time.Minute)
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/disabled",
			Branch:   "main",
			PolledAt: oldTime,
		},
		{
			RepoPath: "/repo/enabled",
			Branch:   "dev",
			PolledAt: oldTime,
		},
	}

	idle := tracker.Check(states, nil)
	if len(idle) != 1 {
		t.Fatalf("expected 1 idle (disabled skipped), got %d", len(idle))
	}
	if idle[0].RepoPath != "/repo/enabled" {
		t.Errorf("expected /repo/enabled, got %s", idle[0].RepoPath)
	}
}

func TestIdleTracker_CommitTimeUsed(t *testing.T) {
	cfg := defaultTestConfig()
	tracker := NewIdleTracker(cfg)

	// PolledAt is old, but commit time is recent.
	oldTime := time.Now().Add(-10 * time.Minute)
	recentCommit := time.Now().Add(-10 * time.Second)
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/a",
			Branch:   "main",
			PolledAt: oldTime,
		},
	}
	commitTimes := map[string]time.Time{
		"/repo/a:main": recentCommit,
	}

	idle := tracker.Check(states, commitTimes)
	if len(idle) != 0 {
		t.Fatalf("expected 0 idle (recent commit), got %d", len(idle))
	}
}

func TestIdleTracker_SessionActivityUsed(t *testing.T) {
	cfg := defaultTestConfig()
	tracker := NewIdleTracker(cfg)

	oldTime := time.Now().Add(-10 * time.Minute)
	recentSession := time.Now().Add(-10 * time.Second)
	states := []tuimsg.WorktreeState{
		{
			RepoPath: "/repo/a",
			Branch:   "main",
			PolledAt: oldTime,
			Sessions: []tuimsg.ClaudeSession{
				{LastActivity: recentSession, IsActive: true},
			},
		},
	}

	idle := tracker.Check(states, nil)
	if len(idle) != 0 {
		t.Fatalf("expected 0 idle (active session), got %d", len(idle))
	}
}
