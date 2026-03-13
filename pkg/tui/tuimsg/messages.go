// Package tuimsg defines the shared message types used by the TUI subsystem.
// It is a leaf package (no imports from pkg/tui or pkg/tui/panels) so that
// both pkg/tui and pkg/tui/panels can import it without creating an import cycle.
// It also must NOT import internal/git to keep the TUI layer independent of the
// subprocess layer; Manager maps git structs to tuimsg structs.
package tuimsg

import "time"

// ClaudeSession represents one Claude Code session for a worktree's repo.
type ClaudeSession struct {
	ID             string
	ProjectPath    string
	Slug           string
	LastActivity   time.Time
	IsActive       bool
	LastMessage    string
	RecentFiles    []string
	CurrentTool          string // e.g. "Bash", "Read", "Write", "Edit"
	CurrentCommand       string // target being operated on (file path or truncated command)
	WaitingForPermission bool   // session is blocked waiting for user to approve a tool call
}

// ChangedFile represents one file with staged/unstaged status characters.
// Status chars: 'M'=modified, 'A'=added, 'D'=deleted, 'R'=renamed, '.'=unmodified.
type ChangedFile struct {
	Path           string
	StagedStatus   byte
	UnstagedStatus byte
}

// WorktreeState is the polled git state for one worktree, as delivered to the TUI.
// Behind=-1 means no upstream tracking branch is configured for this worktree.
type WorktreeState struct {
	WorktreeID   string
	WorktreePath string
	RepoPath     string // root repo path (may differ from WorktreePath for git worktrees)
	RepoName     string // basename of RepoPath, used for left panel grouping
	Branch       string
	Ahead        int
	Behind       int // -1 = no upstream tracking branch
	Staged       int
	Unstaged     int
	Untracked    int
	ChangedFiles []ChangedFile
	PolledAt     time.Time
	Sessions     []ClaudeSession // Claude Code sessions for this worktree's repo
	AgentStatus  string          // "", "running", "completed", "failed" — latest agent run status
}

// ResolvedTask is the TUI-side representation of a resolved task.
type ResolvedTask struct {
	ID          string
	Description string
	State       string   // queued, running, completed (DB state)
	Branch      string
	IsReady     bool
	IsBlocked   bool
	BlockedBy   []string // short IDs of blocking tasks
	Layer       int
}

// TaskGraphSnapshot holds the resolved task graph for TUI rendering.
type TaskGraphSnapshot struct {
	Tasks    []ResolvedTask
	HasCycle bool
}

// AgentPatch is the TUI-side representation of a generated patch from an agent run.
type AgentPatch struct {
	ID          string
	RunID       string
	RepoPath    string
	Branch      string
	Title       string
	Description string
	PatchData   string // the actual diff content
	Status      string // draft, approved, applied, rejected, reverted
	CreatedAt   time.Time
}

// PatchSnapshot holds all active agent patches for TUI rendering.
type PatchSnapshot struct {
	Patches []AgentPatch
}

// ResolvedIdea is the TUI-side representation of a resolved idea (side/main quest).
type ResolvedIdea struct {
	ID           string
	Prompt       string
	State        string   // queued, ready, running, completed, failed
	Kind         string   // side, sequential
	IsReady      bool
	IsBlocked    bool
	BlockedBy    []string // short IDs of blocking ideas
	Layer        int
	Incorporated bool
	Branch       string
	ErrorMsg     string
	CreatedAt    time.Time
}

// IdeaGraphSnapshot holds the resolved idea graph for TUI rendering.
type IdeaGraphSnapshot struct {
	Ideas    []ResolvedIdea
	HasCycle bool
}

// StateSnapshot is the point-in-time snapshot of all tracked state delivered to the TUI.
// Worktrees contains one entry per DB-tracked worktree, populated by Manager.pollLoop.
type StateSnapshot struct {
	UpdatedAt time.Time
	Worktrees []WorktreeState
	TaskGraph TaskGraphSnapshot
	Patches   PatchSnapshot
	IdeaGraph IdeaGraphSnapshot
}

// StateEvent is the tea.Msg delivered to RootModel.Update() when the
// background state manager emits a new snapshot.
type StateEvent struct {
	Snapshot StateSnapshot
}
