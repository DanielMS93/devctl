package main

import (
	"fmt"
	"time"

	"github.com/DanielMS93/devctl/internal/idea"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

var ideaCmd = &cobra.Command{
	Use:   "idea",
	Short: "Manage quest-based idea pipeline",
}

var ideaAddCmd = &cobra.Command{
	Use:   "add <prompt>",
	Short: "Queue a new idea (side-quest)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		store := idea.NewStore(db)

		repoID, _ := cmd.Flags().GetString("repo")
		kind, _ := cmd.Flags().GetString("kind")
		dependsOn, _ := cmd.Flags().GetStringSlice("depends-on")

		i, err := store.Create(cmd.Context(), args[0], repoID, kind, "", "")
		if err != nil {
			return err
		}

		// Add dependencies.
		for _, depID := range dependsOn {
			dep, err := store.Get(cmd.Context(), depID)
			if err != nil {
				return fmt.Errorf("resolve dependency %q: %w", depID, err)
			}
			if err := store.AddDep(cmd.Context(), i.ID, dep.ID); err != nil {
				return err
			}
		}

		fmt.Printf("Created idea %s: %s\n", i.ID[:8], truncate(i.Prompt, 60))
		if len(dependsOn) > 0 {
			fmt.Printf("  depends on: %v\n", dependsOn)
		}
		return nil
	},
}

var ideaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ideas with state and kind",
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		store := idea.NewStore(db)

		repoID, _ := cmd.Flags().GetString("repo")
		state, _ := cmd.Flags().GetString("state")

		var ideas []idea.Idea
		var err error

		switch {
		case state != "":
			ideas, err = store.ListByState(cmd.Context(), state)
		case repoID != "":
			ideas, err = store.ListByRepo(cmd.Context(), repoID)
		default:
			ideas, err = store.List(cmd.Context())
		}
		if err != nil {
			return err
		}

		if len(ideas) == 0 {
			fmt.Println("No ideas found.")
			return nil
		}

		fmt.Printf("%-10s %-12s %-6s %-50s %s\n", "ID", "STATE", "KIND", "PROMPT", "CREATED")
		for _, i := range ideas {
			created := time.Unix(i.CreatedAt, 0).Format("2006-01-02")
			state := i.State
			if i.Incorporated == 1 {
				state += "*"
			}
			fmt.Printf("%-10s %-12s %-6s %-50s %s\n",
				i.ID[:8], state, i.Kind, truncate(i.Prompt, 48), created)
		}
		return nil
	},
}

var ideaShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show idea details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		store := idea.NewStore(db)

		i, err := store.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}

		fmt.Printf("ID:       %s\n", i.ID[:8])
		fmt.Printf("Prompt:   %s\n", i.Prompt)
		fmt.Printf("State:    %s\n", i.State)
		fmt.Printf("Kind:     %s\n", i.Kind)
		if i.Branch != "" {
			fmt.Printf("Branch:   %s\n", i.Branch)
		}
		if i.ParentBranch != "" {
			fmt.Printf("Parent:   %s\n", i.ParentBranch)
		}
		if i.WorktreePath != "" {
			fmt.Printf("Worktree: %s\n", i.WorktreePath)
		}
		if i.Incorporated == 1 {
			fmt.Println("Incorporated: yes")
		}
		if i.ErrorMsg != nil && *i.ErrorMsg != "" {
			fmt.Printf("Error:    %s\n", *i.ErrorMsg)
		}
		fmt.Printf("Created:  %s\n", time.Unix(i.CreatedAt, 0).Format("2006-01-02 15:04"))

		// Show dependencies.
		deps, err := store.ListDeps(cmd.Context(), i.ID)
		if err == nil && len(deps) > 0 {
			fmt.Print("Depends on:")
			for _, d := range deps {
				depID := d.DependsOnID
				if len(depID) > 8 {
					depID = depID[:8]
				}
				fmt.Printf(" %s", depID)
			}
			fmt.Println()
		}

		return nil
	},
}

var ideaCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a queued or ready idea",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		store := idea.NewStore(db)

		i, err := store.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if i.State != "queued" && i.State != "ready" {
			return fmt.Errorf("idea %s has state %q; only queued/ready ideas can be cancelled", i.ID[:8], i.State)
		}

		if err := store.Delete(cmd.Context(), i.ID); err != nil {
			return err
		}
		fmt.Printf("Cancelled idea %s\n", i.ID[:8])
		return nil
	},
}

var ideaRunCmd = &cobra.Command{
	Use:   "run <id>",
	Short: "Manually launch an idea",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		store := idea.NewStore(db)
		executor := idea.NewExecutor(store)

		i, err := store.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if i.State != "queued" && i.State != "ready" {
			return fmt.Errorf("idea %s has state %q; only queued/ready ideas can be launched", i.ID[:8], i.State)
		}

		repoPath := i.RepoID
		if repoPath == "" {
			return fmt.Errorf("idea %s has no repo path set", i.ID[:8])
		}

		executor.TryLaunch(cmd.Context(), i, repoPath)
		fmt.Printf("Launched idea %s\n", i.ID[:8])
		return nil
	},
}

func init() {
	ideaAddCmd.Flags().String("repo", "", "repo path to scope the idea to")
	ideaAddCmd.Flags().String("kind", "side", "idea kind: side or sequential")
	ideaAddCmd.Flags().StringSlice("depends-on", nil, "IDs of ideas this depends on")

	ideaListCmd.Flags().String("repo", "", "filter by repo path")
	ideaListCmd.Flags().String("state", "", "filter by state")

	ideaCmd.AddCommand(ideaAddCmd, ideaListCmd, ideaShowCmd, ideaCancelCmd, ideaRunCmd)
}
