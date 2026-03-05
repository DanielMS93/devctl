// Package tuimsg defines the shared message types used by the TUI subsystem.
// It is a leaf package (no imports from pkg/tui or pkg/tui/panels) so that
// both pkg/tui and pkg/tui/panels can import it without creating an import cycle.
// It also must NOT import internal/git to keep the TUI layer independent of the
// subprocess layer; Manager maps git structs to tuimsg structs.
package tuimsg

import "time"

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
	Branch       string
	Ahead        int
	Behind       int // -1 = no upstream tracking branch
	Staged       int
	Unstaged     int
	Untracked    int
	ChangedFiles []ChangedFile
	PolledAt     time.Time
}

// StateSnapshot is the point-in-time snapshot of all tracked state delivered to the TUI.
// Worktrees contains one entry per DB-tracked worktree, populated by Manager.pollLoop.
type StateSnapshot struct {
	UpdatedAt time.Time
	Worktrees []WorktreeState
}

// StateEvent is the tea.Msg delivered to RootModel.Update() when the
// background state manager emits a new snapshot.
type StateEvent struct {
	Snapshot StateSnapshot
}
