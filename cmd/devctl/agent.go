package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/danielmiessler/devctl/internal/agent"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agent workflows and patches",
}

var agentReviewCmd = &cobra.Command{
	Use:   "review [patch-id]",
	Short: "List draft patches or inspect a specific patch",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		if len(args) == 1 {
			return runAgentReviewOne(cmd, db, args[0])
		}
		return runAgentReviewList(cmd, db)
	},
}

var agentApplyCmd = &cobra.Command{
	Use:   "apply <patch-id>",
	Short: "Apply a draft or approved patch via git",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runAgentApply(cmd, db, args[0])
	},
}

var agentRevertCmd = &cobra.Command{
	Use:   "revert <patch-id>",
	Short: "Revert a previously applied patch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runAgentRevert(cmd, db, args[0])
	},
}

var agentEditCmd = &cobra.Command{
	Use:   "edit <patch-id>",
	Short: "Edit a patch's diff content in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runAgentEdit(cmd, db, args[0])
	},
}

var agentConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current agent configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentConfig()
	},
}

var agentToggleCmd = &cobra.Command{
	Use:   "toggle <workflow>",
	Short: "Toggle a workflow's enabled state",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentToggle(args[0])
	},
}

func init() {
	agentCmd.AddCommand(agentReviewCmd, agentApplyCmd, agentRevertCmd, agentEditCmd, agentConfigCmd, agentToggleCmd)
}

func runAgentReviewList(cmd *cobra.Command, db *sqlx.DB) error {
	ctx := cmd.Context()
	store := agent.NewPatchStore(db)

	patches, err := store.ListByStatus(ctx, "draft")
	if err != nil {
		return err
	}

	if len(patches) == 0 {
		fmt.Println("No draft patches found.")
		return nil
	}

	fmt.Printf("%-10s %-30s %-20s %-10s %s\n", "ID", "TITLE", "BRANCH", "STATUS", "CREATED")
	for _, p := range patches {
		created := time.Unix(p.CreatedAt, 0).Format("2006-01-02")
		fmt.Printf("%-10s %-30s %-20s %-10s %s\n",
			p.ID[:8], truncate(p.Title, 28), truncate(p.Branch, 18), p.Status, created)
	}
	return nil
}

func runAgentReviewOne(cmd *cobra.Command, db *sqlx.DB, idPrefix string) error {
	ctx := cmd.Context()
	store := agent.NewPatchStore(db)

	p, err := store.Get(ctx, idPrefix)
	if err != nil {
		return err
	}

	fmt.Printf("Patch:   %s\n", p.ID[:8])
	fmt.Printf("Title:   %s\n", p.Title)
	fmt.Printf("Status:  %s\n", p.Status)
	fmt.Printf("Branch:  %s\n", p.Branch)
	fmt.Printf("Created: %s\n", time.Unix(p.CreatedAt, 0).Format("2006-01-02 15:04"))
	fmt.Println()
	fmt.Println(p.PatchData)
	return nil
}

func runAgentApply(cmd *cobra.Command, db *sqlx.DB, idPrefix string) error {
	ctx := cmd.Context()
	store := agent.NewPatchStore(db)

	p, err := store.Get(ctx, idPrefix)
	if err != nil {
		return err
	}

	if p.Status != "draft" && p.Status != "approved" {
		return fmt.Errorf("patch %s has status %q; only draft or approved patches can be applied", p.ID[:8], p.Status)
	}

	if err := agent.CheckPatch(ctx, p.RepoPath, p.PatchData); err != nil {
		return fmt.Errorf("patch has conflicts: %w", err)
	}

	if err := agent.ApplyPatch(ctx, p.RepoPath, p.PatchData); err != nil {
		return err
	}

	if err := store.SetApplied(ctx, p.ID); err != nil {
		return err
	}

	fmt.Printf("Applied patch: %s\n", p.Title)
	return nil
}

func runAgentRevert(cmd *cobra.Command, db *sqlx.DB, idPrefix string) error {
	ctx := cmd.Context()
	store := agent.NewPatchStore(db)

	p, err := store.Get(ctx, idPrefix)
	if err != nil {
		return err
	}

	if p.Status != "applied" {
		return fmt.Errorf("patch %s has status %q; only applied patches can be reverted", p.ID[:8], p.Status)
	}

	if err := agent.RevertPatch(ctx, p.RepoPath, p.PatchData); err != nil {
		return err
	}

	if err := store.SetReverted(ctx, p.ID); err != nil {
		return err
	}

	fmt.Printf("Reverted patch: %s\n", p.Title)
	return nil
}

func runAgentEdit(cmd *cobra.Command, db *sqlx.DB, idPrefix string) error {
	ctx := cmd.Context()
	store := agent.NewPatchStore(db)

	p, err := store.Get(ctx, idPrefix)
	if err != nil {
		return err
	}

	if p.Status != "draft" && p.Status != "approved" {
		return fmt.Errorf("patch %s has status %q; only draft or approved patches can be edited", p.ID[:8], p.Status)
	}

	// Write patch data to temp file for editing.
	tmpFile, err := os.CreateTemp("", "devctl-edit-*.patch")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(p.PatchData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Determine editor.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Open editor interactively.
	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Read back modified content.
	newData, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("read edited file: %w", err)
	}

	if string(newData) == p.PatchData {
		fmt.Println("No changes made.")
		return nil
	}

	if err := store.UpdatePatchData(ctx, p.ID, string(newData)); err != nil {
		return err
	}

	fmt.Printf("Patch %s updated.\n", p.Title)
	return nil
}

func runAgentConfig() error {
	cfg := agent.LoadConfig()

	fmt.Printf("Agent Enabled: %v\n", cfg.Enabled)
	fmt.Printf("Idle Threshold: %d minutes\n", cfg.IdleThresholdMinutes)
	fmt.Printf("Cooldown: %d minutes\n", cfg.CooldownMinutes)
	fmt.Printf("Max Patch Size: %d KB\n", cfg.MaxPatchSizeKB)
	fmt.Println()

	fmt.Println("Workflows:")
	for name, wf := range cfg.Workflows {
		status := "disabled"
		if wf.Enabled {
			status = "enabled"
		}
		fmt.Printf("  %-20s %s\n", name, status)
		if wf.Command != "" {
			fmt.Printf("    command: %s\n", wf.Command)
		}
		if wf.PromptFile != "" {
			fmt.Printf("    prompt_file: %s\n", wf.PromptFile)
		}
	}

	if len(cfg.DisabledRepos) > 0 {
		fmt.Println()
		fmt.Println("Disabled Repos:")
		for _, r := range cfg.DisabledRepos {
			fmt.Printf("  %s\n", r)
		}
	}

	return nil
}

func runAgentToggle(workflow string) error {
	key := "agent.workflows." + workflow + ".enabled"
	current := viper.GetBool(key)
	viper.Set(key, !current)

	if err := viper.WriteConfig(); err != nil {
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	state := "disabled"
	if !current {
		state = "enabled"
	}
	fmt.Printf("Workflow %s is now %s\n", workflow, state)
	return nil
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
