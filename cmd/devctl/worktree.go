package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DanielMS93/devctl/internal/git"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

// dbKey is the context key used to pass the *sqlx.DB through cobra commands.
type dbKey struct{}

// worktreeCmd is the parent `devctl worktree` command.
var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees",
}

// worktreeListCmd prints all tracked worktrees.
var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runWorktreeList(cmd.Context(), db)
	},
}

// worktreeCreateCmd creates a new linked worktree.
var worktreeCreateCmd = &cobra.Command{
	Use:   "create <repo-path> <branch>",
	Short: "Create a new linked worktree for a branch",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		repoPath := args[0]
		branch := args[1]
		newBranch, _ := cmd.Flags().GetBool("new-branch")
		return runWorktreeCreate(cmd.Context(), db, repoPath, branch, newBranch)
	},
}

// worktreeDeleteCmd removes a linked worktree.
var worktreeDeleteCmd = &cobra.Command{
	Use:   "delete <worktree-path>",
	Short: "Delete a linked worktree",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runWorktreeDelete(cmd.Context(), db, args[0])
	},
}

func init() {
	worktreeCreateCmd.Flags().BoolP("new-branch", "b", false, "Create a new branch")
	worktreeCmd.AddCommand(worktreeListCmd, worktreeCreateCmd, worktreeDeleteCmd)
}

func runWorktreeList(ctx context.Context, db *sqlx.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT w.id, w.path, w.branch, r.path as repo_path
		FROM worktrees w
		JOIN repos r ON r.id = w.repo_id
		ORDER BY r.path, w.branch
	`)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		found = true
		var id, path, branch, repoPath string
		if err := rows.Scan(&id, &path, &branch, &repoPath); err != nil {
			return err
		}
		fmt.Printf("  %s  [%s]  repo: %s\n", branch, path, repoPath)
	}
	if !found {
		fmt.Println("No worktrees tracked. Use 'devctl worktree create' to add one.")
	}
	return rows.Err()
}

func runWorktreeCreate(ctx context.Context, db *sqlx.DB, repoPath, branch string, newBranch bool) error {
	// Resolve repo path: accepts repo name (looked up from DB) or filesystem path.
	absRepoPath, err := resolveRepo(ctx, db, repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}

	// Validate the resolved path is actually a git repo before touching the DB.
	if _, err := os.Stat(filepath.Join(absRepoPath, ".git")); err != nil {
		return fmt.Errorf("%s is not a git repo (no .git found)", absRepoPath)
	}

	// Auto-register repo if not already tracked.
	repoID, err := ensureRepo(ctx, db, absRepoPath)
	if err != nil {
		return fmt.Errorf("register repo: %w", err)
	}

	// Determine worktree path: sibling of repo named after branch (sanitized).
	safeBranch := sanitizeBranch(branch)
	worktreePath := filepath.Join(filepath.Dir(absRepoPath), safeBranch)

	// Create the linked worktree via git.
	if err := git.AddWorktree(ctx, absRepoPath, worktreePath, branch, newBranch); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}

	// Insert DB record.
	now := time.Now().Unix()
	_, err = db.ExecContext(ctx, `
		INSERT INTO worktrees (id, repo_id, path, branch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), repoID, worktreePath, branch, now, now)
	if err != nil {
		return fmt.Errorf("insert worktree: %w", err)
	}

	// Copy any configured local-only files into the new worktree.
	if err := copyConfiguredFiles(ctx, db, repoID, absRepoPath, worktreePath); err != nil {
		// Non-fatal: log warning but don't fail the create
		fmt.Printf("Warning: some files could not be copied: %v\n", err)
	}

	fmt.Printf("Created worktree: %s (branch: %s)\n", worktreePath, branch)
	return nil
}

// copyConfiguredFiles copies files from the main worktree (srcRoot) into
// the newly created worktreePath (dstRoot). Files listed in repo_copy_files
// for this repo are copied if they exist in the source; missing files are
// silently skipped.
func copyConfiguredFiles(ctx context.Context, db *sqlx.DB, repoID, srcRoot, dstRoot string) error {
	rows, err := db.QueryContext(ctx,
		`SELECT pattern FROM repo_copy_files WHERE repo_id = ? ORDER BY pattern`, repoID)
	if err != nil {
		return fmt.Errorf("query copy files: %w", err)
	}
	defer rows.Close()

	var errs []string
	for rows.Next() {
		var pattern string
		if err := rows.Scan(&pattern); err != nil {
			continue
		}
		src := filepath.Join(srcRoot, pattern)
		dst := filepath.Join(dstRoot, pattern)

		data, err := os.ReadFile(src)
		if os.IsNotExist(err) {
			// Source file doesn't exist — skip silently (common for optional files like .env.local)
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", pattern, err))
			continue
		}

		// Ensure parent directory exists in destination
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			errs = append(errs, fmt.Sprintf("%s: mkdir: %v", pattern, err))
			continue
		}

		if err := os.WriteFile(dst, data, 0600); err != nil {
			errs = append(errs, fmt.Sprintf("%s: write: %v", pattern, err))
			continue
		}
		fmt.Printf("  Copied: %s\n", pattern)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("copy errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func runWorktreeDelete(ctx context.Context, db *sqlx.DB, worktreePath string) error {
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Find the repo path for this worktree from DB.
	var repoPath string
	err = db.QueryRowContext(ctx, `
		SELECT r.path FROM repos r
		JOIN worktrees w ON w.repo_id = r.id
		WHERE w.path = ?
	`, absPath).Scan(&repoPath)
	if err != nil {
		return fmt.Errorf("worktree not found in database: %w", err)
	}

	// Remove from git.
	if err := git.RemoveWorktree(ctx, repoPath, absPath); err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}

	// Remove from DB (worktree_state cascades via FK).
	_, err = db.ExecContext(ctx, `DELETE FROM worktrees WHERE path = ?`, absPath)
	if err != nil {
		return fmt.Errorf("delete worktree record: %w", err)
	}

	fmt.Printf("Deleted worktree: %s\n", absPath)
	return nil
}

// ensureRepo inserts a repo record if the path is not already tracked.
// Returns the repo ID.
func ensureRepo(ctx context.Context, db *sqlx.DB, absPath string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM repos WHERE path = ?`, absPath).Scan(&id)
	if err == nil {
		// Repo exists — still ensure it has a worktree entry.
		ensureWorktree(ctx, db, id, absPath)
		return id, nil
	}
	repoID, err := insertRepo(ctx, db, absPath)
	if err != nil {
		return "", err
	}
	// Also create a worktree entry for the repo's main working directory
	// so it appears in the TUI dashboard.
	ensureWorktree(ctx, db, repoID, absPath)
	return repoID, nil
}

// ensureWorktree creates a worktree row for a path if one doesn't already exist.
func ensureWorktree(ctx context.Context, db *sqlx.DB, repoID, path string) {
	var existing string
	err := db.QueryRowContext(ctx, `SELECT id FROM worktrees WHERE path = ?`, path).Scan(&existing)
	if err == nil {
		return // already exists
	}
	now := time.Now().Unix()
	branch := "main" // default; will be corrected by the next poll cycle
	if b, err := git.CurrentBranch(ctx, path); err == nil && b != "" {
		branch = b
	}
	_, _ = db.ExecContext(ctx, `
		INSERT INTO worktrees (id, repo_id, path, branch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), repoID, path, branch, now, now)
}

// sanitizeBranch converts a branch name to a filesystem-safe directory name.
// e.g. "feature/add-login" -> "feature-add-login"
func sanitizeBranch(branch string) string {
	result := make([]byte, len(branch))
	for i, c := range []byte(branch) {
		if c == '/' || c == '\\' || c == ':' || c == '*' || c == '?' || c == '"' || c == '<' || c == '>' || c == '|' {
			result[i] = '-'
		} else {
			result[i] = c
		}
	}
	return string(result)
}
