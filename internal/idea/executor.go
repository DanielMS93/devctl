package idea

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Executor launches side-quest Claude sessions in worktrees.
type Executor struct {
	store *Store
}

// NewExecutor creates an Executor.
func NewExecutor(store *Store) *Executor {
	return &Executor{store: store}
}

// TryLaunch attempts to launch a Claude session for the given idea.
// It creates a worktree, transitions the idea to running, and spawns claude --print.
// This method is safe to call concurrently — the SetRunning guard prevents double-launch.
func (e *Executor) TryLaunch(ctx context.Context, idea Idea, repoPath string) {
	shortID := idea.ID[:8]
	branch := "idea/" + shortID

	// Build the worktree path: <repo>/../.devctl-ideas/<shortID>
	ideasDir := filepath.Join(filepath.Dir(repoPath), ".devctl-ideas")
	wtPath := filepath.Join(ideasDir, shortID)

	// Build prompt with parent context.
	prompt := BuildPromptWithContext(idea.Prompt, idea.ParentSessionID, repoPath)

	// Create the worktree directory.
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		e.fail(ctx, idea.ID, fmt.Sprintf("create ideas dir: %v", err))
		return
	}

	// Create a new branch and worktree.
	parentBranch := idea.ParentBranch
	if parentBranch == "" {
		parentBranch = defaultBranch(ctx, repoPath)
	}

	// Create the branch from the parent branch.
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, wtPath, parentBranch)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		e.fail(ctx, idea.ID, fmt.Sprintf("git worktree add: %v: %s", err, stderr.String()))
		return
	}

	// Atomically claim the idea — prevents double-launch race.
	if err := e.store.SetRunning(ctx, idea.ID, "", wtPath, branch); err != nil {
		// Another goroutine got there first, or idea was cancelled. Clean up worktree.
		slog.Info("idea already claimed, cleaning up worktree", "id", shortID)
		cleanupWorktree(ctx, repoPath, wtPath, branch)
		return
	}

	slog.Info("launching side-quest", "id", shortID, "worktree", wtPath, "prompt_len", len(prompt))

	// Launch claude in a background goroutine with timeout.
	go e.runClaude(ctx, idea.ID, wtPath, prompt)
}

// runClaude executes claude --print in the worktree and records the result.
func (e *Executor) runClaude(ctx context.Context, ideaID, wtPath, prompt string) {
	// 30 minute timeout.
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	claudeBin := findClaudeBin()

	cmd := exec.CommandContext(runCtx, claudeBin, "-p", prompt)
	cmd.Dir = wtPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("%v: %s", err, stderr.String())
		if setErr := e.store.SetFailed(ctx, ideaID, errMsg); setErr != nil {
			slog.Error("idea set failed", "id", ideaID[:8], "err", setErr)
		}
		slog.Warn("side-quest failed", "id", ideaID[:8], "err", errMsg)
		return
	}

	if setErr := e.store.SetCompleted(ctx, ideaID); setErr != nil {
		slog.Error("idea set completed", "id", ideaID[:8], "err", setErr)
		return
	}
	slog.Info("side-quest completed", "id", ideaID[:8], "output_len", stdout.Len())
}

// fail marks an idea as failed and logs the error.
func (e *Executor) fail(ctx context.Context, id, msg string) {
	shortID := id
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if err := e.store.SetFailed(ctx, id, msg); err != nil {
		slog.Error("idea set failed", "id", shortID, "err", err)
	}
	slog.Warn("side-quest launch failed", "id", shortID, "reason", msg)
}

// cleanupWorktree removes a worktree and its branch on failure.
func cleanupWorktree(ctx context.Context, repoPath, wtPath, branch string) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = repoPath
	_ = cmd.Run()

	cmd = exec.CommandContext(ctx, "git", "branch", "-D", branch)
	cmd.Dir = repoPath
	_ = cmd.Run()
}

// defaultBranch returns the default branch name for the repo (main or master).
func defaultBranch(ctx context.Context, repoPath string) string {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		if parts := strings.Split(ref, "/"); len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return "main"
}

// findClaudeBin returns the path to the claude binary.
func findClaudeBin() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		home + "/.local/bin/claude",
		home + "/go/bin/claude",
		"/usr/local/bin/claude",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "claude"
}
