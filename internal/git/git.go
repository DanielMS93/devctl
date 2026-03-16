package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// run executes a git command in the given directory and returns stdout.
// ALWAYS set cmd.Dir (git operations are directory-relative).
// Error includes stderr for actionable diagnostics.
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git %s: %w\nstderr: %s", args[0], err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// CurrentBranch returns the current branch name for the given repo path.
// Returns empty string if detached HEAD or on error.
func CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	out, err := run(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// LastCommitTime returns the time of the most recent commit on the given branch.
// Returns zero time if the branch has no commits or the ref is not found.
func LastCommitTime(ctx context.Context, repoPath, branch string) (time.Time, error) {
	out, err := run(ctx, repoPath, "log", "-1", "--format=%ct", branch)
	if err != nil {
		// Branch not found or no commits — return zero time, not an error.
		return time.Time{}, nil
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return time.Time{}, nil
	}
	unix, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse commit time %q: %w", s, err)
	}
	return time.Unix(unix, 0), nil
}
