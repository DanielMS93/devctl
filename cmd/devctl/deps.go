package main

import (
	"fmt"

	"github.com/danielmiessler/devctl/internal/dependency"
	"github.com/danielmiessler/devctl/internal/task"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Manage task dependencies",
}

var depsAddCmd = &cobra.Command{
	Use:   "add <task-id> <depends-on-id>",
	Short: "Add a dependency between tasks",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runDepsAdd(cmd, db, args[0], args[1])
	},
}

var depsRemoveCmd = &cobra.Command{
	Use:   "remove <task-id> <depends-on-id>",
	Short: "Remove a dependency between tasks",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runDepsRemove(cmd, db, args[0], args[1])
	},
}

var depsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all task dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runDepsList(cmd, db)
	},
}

func init() {
	depsListCmd.Flags().String("task", "", "Filter dependencies by task ID (prefix match)")
	depsCmd.AddCommand(depsAddCmd, depsRemoveCmd, depsListCmd)
}

func runDepsAdd(cmd *cobra.Command, db *sqlx.DB, taskIDPrefix, depIDPrefix string) error {
	ctx := cmd.Context()
	taskStore := task.NewStore(db)
	depStore := dependency.NewStore(db)

	// Resolve both IDs via prefix match.
	t, err := taskStore.Get(ctx, taskIDPrefix)
	if err != nil {
		return fmt.Errorf("resolve task: %w", err)
	}
	dep, err := taskStore.Get(ctx, depIDPrefix)
	if err != nil {
		return fmt.Errorf("resolve dependency target: %w", err)
	}

	// Cycle detection: fetch all existing deps and check.
	allDeps, err := depStore.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("fetch dependencies: %w", err)
	}

	if wouldCreateCycle(allDeps, t.ID, dep.ID) {
		return fmt.Errorf("cannot add dependency: would create cycle %s -> ... -> %s -> %s",
			t.ID[:8], dep.ID[:8], t.ID[:8])
	}

	if err := depStore.Add(ctx, t.ID, dep.ID); err != nil {
		return err
	}

	fmt.Printf("Added: %s depends on %s\n", t.ID[:8], dep.ID[:8])
	return nil
}

func runDepsRemove(cmd *cobra.Command, db *sqlx.DB, taskIDPrefix, depIDPrefix string) error {
	ctx := cmd.Context()
	taskStore := task.NewStore(db)
	depStore := dependency.NewStore(db)

	t, err := taskStore.Get(ctx, taskIDPrefix)
	if err != nil {
		return fmt.Errorf("resolve task: %w", err)
	}
	dep, err := taskStore.Get(ctx, depIDPrefix)
	if err != nil {
		return fmt.Errorf("resolve dependency target: %w", err)
	}

	if err := depStore.Remove(ctx, t.ID, dep.ID); err != nil {
		return err
	}

	fmt.Printf("Removed: %s no longer depends on %s\n", t.ID[:8], dep.ID[:8])
	return nil
}

func runDepsList(cmd *cobra.Command, db *sqlx.DB) error {
	ctx := cmd.Context()
	taskStore := task.NewStore(db)
	depStore := dependency.NewStore(db)

	taskFlag, _ := cmd.Flags().GetString("task")

	var deps []dependency.Dep
	var err error

	if taskFlag != "" {
		// Resolve the task via prefix match, then filter.
		t, resolveErr := taskStore.Get(ctx, taskFlag)
		if resolveErr != nil {
			return fmt.Errorf("resolve task: %w", resolveErr)
		}
		deps, err = depStore.List(ctx, t.ID)
	} else {
		deps, err = depStore.ListAll(ctx)
	}
	if err != nil {
		return err
	}

	if len(deps) == 0 {
		fmt.Println("No dependencies found.")
		return nil
	}

	// Build a map of task IDs to descriptions for display.
	allTasks, err := taskStore.List(ctx)
	if err != nil {
		return fmt.Errorf("fetch tasks: %w", err)
	}
	descMap := make(map[string]string, len(allTasks))
	for _, t := range allTasks {
		descMap[t.ID] = t.Description
	}

	for _, d := range deps {
		taskDesc := descMap[d.TaskID]
		depDesc := descMap[d.DependsOnID]
		fmt.Printf("  %s (%s) depends on %s (%s)\n",
			d.TaskID[:8], taskDesc, d.DependsOnID[:8], depDesc)
	}
	return nil
}

// wouldCreateCycle checks whether adding a dependency from taskID to dependsOnID
// would introduce a cycle in the dependency graph.
func wouldCreateCycle(deps []dependency.Dep, taskID, dependsOnID string) bool {
	// Build adjacency: task -> what it depends on (upstream edges).
	upstream := make(map[string][]string)
	for _, d := range deps {
		upstream[d.TaskID] = append(upstream[d.TaskID], d.DependsOnID)
	}
	// DFS from dependsOnID following upstream edges to see if taskID is reachable.
	// If it is, adding taskID -> dependsOnID would close a cycle.
	visited := make(map[string]bool)
	var dfs func(string) bool
	dfs = func(node string) bool {
		if node == taskID {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true
		for _, up := range upstream[node] {
			if dfs(up) {
				return true
			}
		}
		return false
	}
	return dfs(dependsOnID)
}
