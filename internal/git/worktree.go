package git

import (
	"bytes"
	"context"
	"strings"
)

// Worktree represents one entry from `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // short name (refs/heads/ stripped); empty if detached
	Bare     bool
	Locked   bool
	Prunable bool
}

// ListWorktrees runs `git worktree list --porcelain` in repoPath and parses the stanza output.
// repoPath must be the main worktree or any linked worktree path — git resolves the root.
func ListWorktrees(ctx context.Context, repoPath string) ([]Worktree, error) {
	out, err := run(ctx, repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out), nil
}

// AddWorktree runs `git worktree add <path> <branch>` in repoPath.
// If newBranch is true, creates a new branch with the given name.
// If newBranch is false, checks out an existing branch at the given path.
func AddWorktree(ctx context.Context, repoPath, worktreePath, branch string, newBranch bool) error {
	var args []string
	if newBranch {
		args = []string{"worktree", "add", worktreePath, "-b", branch}
	} else {
		args = []string{"worktree", "add", worktreePath, branch}
	}
	_, err := run(ctx, repoPath, args...)
	return err
}

// RemoveWorktree runs `git worktree remove <path> --force` in repoPath.
func RemoveWorktree(ctx context.Context, repoPath, worktreePath string) error {
	_, err := run(ctx, repoPath, "worktree", "remove", worktreePath, "--force")
	return err
}

// parseWorktrees parses the stanza-based output of `git worktree list --porcelain`.
// Stanzas are separated by blank lines; each stanza is key-value pairs.
func parseWorktrees(out []byte) []Worktree {
	var results []Worktree
	stanzas := bytes.Split(out, []byte("\n\n"))
	for _, stanza := range stanzas {
		stanza = bytes.TrimSpace(stanza)
		if len(stanza) == 0 {
			continue
		}
		var wt Worktree
		for _, line := range bytes.Split(stanza, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			parts := bytes.SplitN(line, []byte(" "), 2)
			key := string(parts[0])
			var val string
			if len(parts) > 1 {
				val = string(parts[1])
			}
			switch key {
			case "worktree":
				wt.Path = val
			case "HEAD":
				wt.Head = val
			case "branch":
				wt.Branch = strings.TrimPrefix(val, "refs/heads/")
			case "bare":
				wt.Bare = true
			case "locked":
				wt.Locked = true
			case "prunable":
				wt.Prunable = true
			}
		}
		if wt.Path != "" {
			results = append(results, wt)
		}
	}
	return results
}
