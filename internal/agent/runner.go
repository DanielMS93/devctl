package agent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// WorkflowRunner executes configured agent workflows for idle branches.
// Results are stored as AgentRun records; output that looks like a diff
// is stored as an AgentPatch.
type WorkflowRunner struct {
	runStore   *AgentRunStore
	patchStore *PatchStore
	config     AgentConfig
}

// NewWorkflowRunner creates a WorkflowRunner.
func NewWorkflowRunner(runStore *AgentRunStore, patchStore *PatchStore, cfg AgentConfig) *WorkflowRunner {
	return &WorkflowRunner{
		runStore:   runStore,
		patchStore: patchStore,
		config:     cfg,
	}
}

// RunWorkflows executes all enabled workflows for the given idle branch.
func (r *WorkflowRunner) RunWorkflows(ctx context.Context, idle IdleBranch) {
	for name, wf := range r.config.Workflows {
		if !wf.Enabled || wf.Command == "" {
			continue
		}

		// Check per-workflow cooldown via DB.
		lastTriggered, err := r.runStore.LastTriggered(ctx, idle.RepoPath, idle.Branch, name)
		if err != nil {
			slog.Error("workflow cooldown check", "workflow", name, "err", err)
			continue
		}
		if !lastTriggered.IsZero() && time.Since(lastTriggered) < r.config.Cooldown() {
			slog.Debug("workflow still in cooldown", "workflow", name, "branch", idle.Branch)
			continue
		}

		r.runWorkflow(ctx, name, wf, idle)
	}
}

// runWorkflow executes a single workflow command and records the result.
func (r *WorkflowRunner) runWorkflow(ctx context.Context, name string, wf WorkflowConfig, idle IdleBranch) {
	// Create agent run record.
	run, err := r.runStore.Create(ctx, AgentRun{
		RepoPath: idle.RepoPath,
		Branch:   idle.Branch,
		Workflow: name,
		Status:   "running",
	})
	if err != nil {
		slog.Error("create agent run", "workflow", name, "err", err)
		return
	}

	slog.Info("starting workflow", "workflow", name, "repo", idle.RepoPath, "branch", idle.Branch)

	// Execute with 5-minute timeout.
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", wf.Command)
	cmd.Dir = idle.RepoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("%v: %s", err, stderr.String())
		if updateErr := r.runStore.UpdateStatus(ctx, run.ID, "failed", &errMsg); updateErr != nil {
			slog.Error("update agent run status", "err", updateErr)
		}
		slog.Warn("workflow failed", "workflow", name, "err", errMsg)
		return
	}

	// Success — check if output looks like a diff.
	output := stdout.String()
	if output != "" && looksLikeDiff(output) {
		// Check size limit.
		sizeKB := len(output) / 1024
		if r.config.MaxPatchSizeKB > 0 && sizeKB > r.config.MaxPatchSizeKB {
			slog.Warn("patch exceeds size limit", "workflow", name, "size_kb", sizeKB, "limit_kb", r.config.MaxPatchSizeKB)
		} else {
			_, patchErr := r.patchStore.Create(ctx, AgentPatch{
				RunID:     run.ID,
				RepoPath:  idle.RepoPath,
				Branch:    idle.Branch,
				Title:     name,
				PatchData: output,
				Status:    "draft",
			})
			if patchErr != nil {
				slog.Error("create agent patch", "workflow", name, "err", patchErr)
			}
		}
	}

	if updateErr := r.runStore.UpdateStatus(ctx, run.ID, "completed", nil); updateErr != nil {
		slog.Error("update agent run status", "err", updateErr)
	}
	slog.Info("workflow completed", "workflow", name, "repo", idle.RepoPath, "branch", idle.Branch)
}

// RunAsync launches RunWorkflows in a background goroutine.
// This should only be called from Manager (which owns background goroutines).
func (r *WorkflowRunner) RunAsync(ctx context.Context, idle IdleBranch) {
	go r.RunWorkflows(ctx, idle)
}

// looksLikeDiff returns true if the output appears to be a unified diff.
func looksLikeDiff(s string) bool {
	return strings.Contains(s, "---") && strings.Contains(s, "+++")
}
