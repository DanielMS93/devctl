package git

import (
	"bytes"
	"context"
	"fmt"
)

// WorktreeState is the polled git state for one worktree.
// Behind=-1 means no upstream tracking branch is configured.
type WorktreeState struct {
	WorktreePath string
	Branch       string
	Ahead        int
	Behind       int // -1 = no upstream tracking branch
	Staged       int
	Unstaged     int
	Untracked    int
	ChangedFiles []ChangedFile
}

// ChangedFile represents one entry from git status --porcelain=v2.
type ChangedFile struct {
	Path           string
	StagedStatus   byte // 'M', 'A', 'D', 'R', 'C', '.' etc.
	UnstagedStatus byte
}

// PollState runs `git status --porcelain=v2 --branch` in worktreePath and
// returns a WorktreeState. This is the single call per worktree per poll cycle.
func PollState(ctx context.Context, worktreePath string) (WorktreeState, error) {
	out, err := run(ctx, worktreePath, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return WorktreeState{}, fmt.Errorf("PollState %s: %w", worktreePath, err)
	}
	state := parseStatus(out)
	state.WorktreePath = worktreePath

	// Get current branch name from `git rev-parse --abbrev-ref HEAD`
	branchOut, err := run(ctx, worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		state.Branch = string(bytes.TrimSpace(branchOut))
	}
	return state, nil
}

// parseStatus parses `git status --porcelain=v2 --branch` output into counts.
// Returns WorktreeState with Behind=-1 sentinel when no upstream tracking branch.
func parseStatus(out []byte) WorktreeState {
	state := WorktreeState{Behind: -1} // -1 = no upstream until # branch.ab line found
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		switch {
		case bytes.HasPrefix(line, []byte("# branch.ab ")):
			fmt.Sscanf(string(line), "# branch.ab +%d -%d", &state.Ahead, &state.Behind)
		case bytes.HasPrefix(line, []byte("# branch.head ")):
			// branch.head gives branch name or "(detached)" — PollState overwrites with rev-parse
		case line[0] == '1' || line[0] == '2':
			// Format: "1 XY ..."  or "2 XY ..."  X=staged col, Y=unstaged col
			if len(line) > 4 {
				x := line[2]
				y := line[3]
				if x != '.' {
					state.Staged++
				}
				if y != '.' {
					state.Unstaged++
				}
				// Extract path: field 9 for type 1, field 10 for type 2 (space separated)
				fields := bytes.Fields(line)
				minFields := 9
				if line[0] == '2' {
					minFields = 10
				}
				if len(fields) >= minFields {
					cf := ChangedFile{
						Path:           string(fields[minFields-1]),
						StagedStatus:   x,
						UnstagedStatus: y,
					}
					state.ChangedFiles = append(state.ChangedFiles, cf)
				}
			}
		case line[0] == '?':
			state.Untracked++
		}
	}
	return state
}
