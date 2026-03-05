package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
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
