package idea

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// IncorporateResult holds the outcome of incorporating a side-quest.
type IncorporateResult struct {
	CodeMerged bool   // true if git merge was performed
	Transcript string // transcript of the side-quest session
	MergeMsg   string // git merge output (if code was merged)
}

// Incorporate merges or extracts findings from a completed side-quest.
// It detects whether code changed, merges if so, and returns the session transcript.
func Incorporate(ctx context.Context, store *Store, i Idea, targetWorktreePath string) (IncorporateResult, error) {
	if i.State != "completed" {
		return IncorporateResult{}, fmt.Errorf("idea %s is not completed (state: %s)", i.ID[:8], i.State)
	}
	if i.Incorporated == 1 {
		return IncorporateResult{}, fmt.Errorf("idea %s is already incorporated", i.ID[:8])
	}

	var result IncorporateResult

	// Detect code changes: git diff --stat between parent branch and idea branch.
	hasChanges, err := detectCodeChanges(ctx, targetWorktreePath, i.ParentBranch, i.Branch)
	if err != nil {
		return IncorporateResult{}, fmt.Errorf("detect changes: %w", err)
	}

	// If code changes exist, merge the idea branch.
	if hasChanges {
		mergeMsg, err := mergeBranch(ctx, targetWorktreePath, i.Branch)
		if err != nil {
			return IncorporateResult{}, fmt.Errorf("merge failed (resolve manually): %w", err)
		}
		result.CodeMerged = true
		result.MergeMsg = mergeMsg
	}

	// Read the side-quest transcript for context injection.
	if i.SessionID != "" {
		transcript, err := ReadTranscript(i.SessionID, i.WorktreePath)
		if err == nil && transcript != "" {
			result.Transcript = transcript
		}
	}
	// Fallback: try reading from the worktree path as a repo path.
	if result.Transcript == "" && i.WorktreePath != "" {
		transcript, err := ReadTranscript("", i.WorktreePath)
		if err == nil && transcript != "" {
			result.Transcript = transcript
		}
	}

	// Mark as incorporated.
	if err := store.SetIncorporated(ctx, i.ID); err != nil {
		return result, fmt.Errorf("mark incorporated: %w", err)
	}

	return result, nil
}

// detectCodeChanges checks if there are file changes between two branches.
func detectCodeChanges(ctx context.Context, repoPath, baseBranch, ideaBranch string) (bool, error) {
	if baseBranch == "" || ideaBranch == "" {
		return false, nil
	}
	cmd := exec.CommandContext(ctx, "git", "diff", "--stat", baseBranch+"..."+ideaBranch)
	cmd.Dir = repoPath
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git diff --stat: %w", err)
	}
	return strings.TrimSpace(stdout.String()) != "", nil
}

// mergeBranch merges the idea branch into the current branch at targetPath.
func mergeBranch(ctx context.Context, targetPath, ideaBranch string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "merge", ideaBranch, "--no-edit",
		"-m", fmt.Sprintf("Incorporate side-quest: %s", ideaBranch))
	cmd.Dir = targetPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Abort the merge to leave things clean.
		abortCmd := exec.CommandContext(ctx, "git", "merge", "--abort")
		abortCmd.Dir = targetPath
		_ = abortCmd.Run()
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}
