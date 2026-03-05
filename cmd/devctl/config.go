package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// configCmd is the parent `devctl config` command.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage devctl configuration",
}

// configSetCopyFilesCmd sets the list of files to copy when creating a new worktree.
var configSetCopyFilesCmd = &cobra.Command{
	Use:   "set-copy-files <repo-path> <file1> [file2...]",
	Short: "Configure files to copy when creating a new worktree for a repo",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		repoPath := args[0]
		files := args[1:]
		return runSetCopyFiles(cmd.Context(), db, repoPath, files)
	},
}

// configListCopyFilesCmd lists configured copy files for a repo.
var configListCopyFilesCmd = &cobra.Command{
	Use:   "list-copy-files <repo-path>",
	Short: "List files configured to copy for a repo's worktrees",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runListCopyFiles(cmd.Context(), db, args[0])
	},
}

// configSetCmd sets a single key-value config entry (e.g., editor).
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value (e.g., editor)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]
		viper.Set(key, value)
		if err := viper.WriteConfig(); err != nil {
			// If config file doesn't exist yet, create it
			if err := viper.SafeWriteConfig(); err != nil {
				return fmt.Errorf("write config: %w", err)
			}
		}
		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetCopyFilesCmd, configListCopyFilesCmd, configSetCmd)
}

func runSetCopyFiles(ctx context.Context, db *sqlx.DB, repoPath string, files []string) error {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path: %w", err)
	}

	// Get or create repo
	repoID, err := ensureRepo(ctx, db, absPath)
	if err != nil {
		return fmt.Errorf("register repo: %w", err)
	}

	for _, f := range files {
		id := uuid.New().String()
		_, err := db.ExecContext(ctx, `
			INSERT INTO repo_copy_files (id, repo_id, pattern)
			VALUES (?, ?, ?)
			ON CONFLICT(repo_id, pattern) DO NOTHING
		`, id, repoID, f)
		if err != nil {
			return fmt.Errorf("insert copy file %s: %w", f, err)
		}
		fmt.Printf("  Added: %s\n", f)
	}

	return nil
}

func runListCopyFiles(ctx context.Context, db *sqlx.DB, repoPath string) error {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT rcf.pattern
		FROM repo_copy_files rcf
		JOIN repos r ON r.id = rcf.repo_id
		WHERE r.path = ?
		ORDER BY rcf.pattern
	`, absPath)
	if err != nil {
		return fmt.Errorf("query copy files: %w", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		found = true
		var pattern string
		if err := rows.Scan(&pattern); err != nil {
			return err
		}
		fmt.Println(" ", pattern)
	}
	if !found {
		fmt.Printf("No copy files configured for %s\n", absPath)
	}
	return rows.Err()
}
