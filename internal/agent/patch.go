package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// writeTempPatch writes patchData to a temporary file with .patch suffix
// and returns the file path. Caller is responsible for removing the file.
func writeTempPatch(patchData string) (string, error) {
	f, err := os.CreateTemp("", "devctl-*.patch")
	if err != nil {
		return "", fmt.Errorf("create temp patch file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(patchData); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("write temp patch file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("close temp patch file: %w", err)
	}
	return path, nil
}

// CheckPatch verifies that a patch can be cleanly applied to the repo at repoPath.
// Returns nil if the patch applies cleanly, or an error with git's stderr output.
func CheckPatch(ctx context.Context, repoPath, patchData string) error {
	tmpFile, err := writeTempPatch(patchData)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	cmd := exec.CommandContext(ctx, "git", "apply", "--check", tmpFile)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch check failed: %s", string(out))
	}
	return nil
}

// ApplyPatch applies a patch to the repo working tree at repoPath.
// It first verifies the patch can be applied cleanly via CheckPatch.
func ApplyPatch(ctx context.Context, repoPath, patchData string) error {
	if err := CheckPatch(ctx, repoPath, patchData); err != nil {
		return err
	}

	tmpFile, err := writeTempPatch(patchData)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	cmd := exec.CommandContext(ctx, "git", "apply", tmpFile)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch apply failed: %s", string(out))
	}
	return nil
}

// RevertPatch reverses a previously applied patch in the repo at repoPath.
func RevertPatch(ctx context.Context, repoPath, patchData string) error {
	tmpFile, err := writeTempPatch(patchData)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	cmd := exec.CommandContext(ctx, "git", "apply", "--reverse", tmpFile)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch revert failed: %s", string(out))
	}
	return nil
}
