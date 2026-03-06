package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/spf13/viper"
)

// IsBranchMerged checks if branchName has been merged into targetBranch.
// Returns true if branchName is an ancestor of targetBranch (exit code 0).
// Returns false if not an ancestor (exit code 1 -- not an error).
// If the branch ref does not exist at all, returns true (branch was cleaned up post-merge).
func IsBranchMerged(ctx context.Context, repoPath, branchName, targetBranch string) (bool, error) {
	// Check if branchName ref exists (local or remote).
	_, errLocal := run(ctx, repoPath, "rev-parse", "--verify", "refs/heads/"+branchName)
	if errLocal != nil {
		_, errRemote := run(ctx, repoPath, "rev-parse", "--verify", "refs/remotes/origin/"+branchName)
		if errRemote != nil {
			// Branch ref not found anywhere -- assumed merged (deleted post-merge).
			return true, nil
		}
	}

	// Check ancestry.
	_, err := run(ctx, repoPath, "merge-base", "--is-ancestor", branchName, targetBranch)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// DefaultBranch returns the repo's default branch (e.g., "main").
// Tries git symbolic-ref refs/remotes/origin/HEAD, falls back to viper config
// "task.default_target_branch", falls back to "main".
func DefaultBranch(ctx context.Context, repoPath string) string {
	out, err := run(ctx, repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(string(out))
		branch := strings.TrimPrefix(ref, "refs/remotes/origin/")
		if branch != "" {
			return branch
		}
	}

	if v := viper.GetString("task.default_target_branch"); v != "" {
		return v
	}

	return "main"
}
