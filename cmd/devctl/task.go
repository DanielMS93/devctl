package main

import (
	"context"
	"fmt"

	"github.com/DanielMS93/devctl/internal/task"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Manage tasks",
}

var taskCreateCmd = &cobra.Command{
	Use:   "create <description>",
	Short: "Create a new task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runTaskCreate(cmd, db, args[0])
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runTaskList(cmd, db)
	},
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a task's state or branch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runTaskUpdate(cmd, db, args[0])
	},
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runTaskDelete(cmd, db, args[0])
	},
}

func init() {
	taskCreateCmd.Flags().String("repo", "", "Repo name or path to scope the task to")
	taskCreateCmd.Flags().String("branch", "", "Git branch linked to this task")
	taskListCmd.Flags().String("repo", "", "Filter tasks by repo name or path")
	taskUpdateCmd.Flags().String("state", "", "New state (queued, running, completed)")
	taskUpdateCmd.Flags().String("branch", "", "Git branch linked to this task")
	taskCmd.AddCommand(taskCreateCmd, taskListCmd, taskUpdateCmd, taskDeleteCmd)
}

func runTaskCreate(cmd *cobra.Command, db *sqlx.DB, description string) error {
	ctx := cmd.Context()
	store := task.NewStore(db)

	var repoID string
	repoFlag, _ := cmd.Flags().GetString("repo")
	if repoFlag != "" {
		repoPath, err := resolveRepo(ctx, db, repoFlag)
		if err != nil {
			return fmt.Errorf("resolve repo: %w", err)
		}
		id, err := lookupRepoID(ctx, db, repoPath)
		if err != nil {
			return fmt.Errorf("repo not found: %w", err)
		}
		repoID = id
	}

	t, err := store.Create(ctx, description, repoID)
	if err != nil {
		return err
	}

	// Set branch if provided.
	branchFlag, _ := cmd.Flags().GetString("branch")
	if branchFlag != "" {
		if err := store.Update(ctx, t.ID, t.State, branchFlag); err != nil {
			return fmt.Errorf("set branch: %w", err)
		}
	}

	fmt.Printf("Created task %s: %s\n", t.ID[:8], description)
	return nil
}

func runTaskList(cmd *cobra.Command, db *sqlx.DB) error {
	ctx := cmd.Context()
	store := task.NewStore(db)

	repoFlag, _ := cmd.Flags().GetString("repo")

	var tasks []task.Task
	var err error

	if repoFlag != "" {
		repoPath, resolveErr := resolveRepo(ctx, db, repoFlag)
		if resolveErr != nil {
			return fmt.Errorf("resolve repo: %w", resolveErr)
		}
		id, lookupErr := lookupRepoID(ctx, db, repoPath)
		if lookupErr != nil {
			return fmt.Errorf("repo not found: %w", lookupErr)
		}
		tasks, err = store.ListByRepo(ctx, id)
	} else {
		tasks, err = store.List(ctx)
	}
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found. Use 'devctl tasks create <description>' to add one.")
		return nil
	}

	for _, t := range tasks {
		branch := ""
		if t.Branch != "" {
			branch = "  " + t.Branch
		}
		fmt.Printf("  [%s] %-10s %s%s\n", t.ID[:8], t.State, t.Description, branch)
	}
	return nil
}

func runTaskUpdate(cmd *cobra.Command, db *sqlx.DB, idPrefix string) error {
	ctx := cmd.Context()
	store := task.NewStore(db)

	stateFlag, _ := cmd.Flags().GetString("state")
	branchFlag, _ := cmd.Flags().GetString("branch")

	if stateFlag == "" && branchFlag == "" {
		return fmt.Errorf("at least one of --state or --branch is required")
	}

	if stateFlag == "blocked" {
		return fmt.Errorf("blocked is computed automatically, not set manually")
	}

	// Fetch current task via prefix match.
	t, err := store.Get(ctx, idPrefix)
	if err != nil {
		return err
	}

	// Apply updates: keep current values for unset flags.
	newState := t.State
	if stateFlag != "" {
		newState = stateFlag
	}
	newBranch := t.Branch
	if branchFlag != "" {
		newBranch = branchFlag
	}

	if err := store.Update(ctx, t.ID, newState, newBranch); err != nil {
		return err
	}

	fmt.Printf("Updated task %s\n", t.ID[:8])
	return nil
}

func runTaskDelete(cmd *cobra.Command, db *sqlx.DB, idPrefix string) error {
	ctx := cmd.Context()
	store := task.NewStore(db)

	// Resolve short ID via prefix match.
	t, err := store.Get(ctx, idPrefix)
	if err != nil {
		return err
	}

	if err := store.Delete(ctx, t.ID); err != nil {
		return err
	}

	fmt.Printf("Deleted task %s\n", t.ID[:8])
	return nil
}

// lookupRepoID returns the repo UUID for a given absolute path.
func lookupRepoID(ctx context.Context, db *sqlx.DB, absPath string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM repos WHERE path = ?`, absPath).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("no repo at path %s", absPath)
	}
	return id, nil
}
