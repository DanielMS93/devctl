package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupGitRepo creates a temporary git repo with an initial commit containing
// a single file, and returns the repo path and a valid patch that modifies
// that file.
func setupGitRepo(t *testing.T) (repoPath, patch string) {
	t.Helper()

	dir := t.TempDir()

	// Initialize git repo.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")

	// Create initial file and commit.
	initialContent := "hello world\n"
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "file.txt")
	run("commit", "-m", "initial")

	// Modify file and generate a diff.
	modifiedContent := "hello world\nline two\n"
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(modifiedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "diff")
	cmd.Dir = dir
	diffOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff failed: %v", err)
	}

	// Reset the working tree back to the committed state.
	run("checkout", "--", ".")

	return dir, string(diffOut)
}

func TestCheckPatch_Valid(t *testing.T) {
	repoPath, patch := setupGitRepo(t)
	ctx := context.Background()

	err := CheckPatch(ctx, repoPath, patch)
	if err != nil {
		t.Fatalf("CheckPatch should succeed for valid patch: %v", err)
	}
}

func TestCheckPatch_Invalid(t *testing.T) {
	repoPath, _ := setupGitRepo(t)
	ctx := context.Background()

	badPatch := "not a valid patch at all"
	err := CheckPatch(ctx, repoPath, badPatch)
	if err == nil {
		t.Fatal("CheckPatch should fail for invalid patch")
	}
}

func TestApplyPatch(t *testing.T) {
	repoPath, patch := setupGitRepo(t)
	ctx := context.Background()

	if err := ApplyPatch(ctx, repoPath, patch); err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	// Verify file was modified.
	content, err := os.ReadFile(filepath.Join(repoPath, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world\nline two\n" {
		t.Fatalf("unexpected file content after apply: %q", string(content))
	}
}

func TestRevertPatch(t *testing.T) {
	repoPath, patch := setupGitRepo(t)
	ctx := context.Background()

	// Apply first.
	if err := ApplyPatch(ctx, repoPath, patch); err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	// Revert.
	if err := RevertPatch(ctx, repoPath, patch); err != nil {
		t.Fatalf("RevertPatch failed: %v", err)
	}

	// Verify file is back to original.
	content, err := os.ReadFile(filepath.Join(repoPath, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world\n" {
		t.Fatalf("unexpected file content after revert: %q", string(content))
	}
}

func TestApplyPatch_AlreadyApplied(t *testing.T) {
	repoPath, patch := setupGitRepo(t)
	ctx := context.Background()

	// Apply once.
	if err := ApplyPatch(ctx, repoPath, patch); err != nil {
		t.Fatalf("first ApplyPatch failed: %v", err)
	}

	// Apply again should fail (check will fail).
	err := ApplyPatch(ctx, repoPath, patch)
	if err == nil {
		t.Fatal("second ApplyPatch should fail")
	}
}
