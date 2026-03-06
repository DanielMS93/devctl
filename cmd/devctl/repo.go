package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage tracked repos",
}

var repoScanCmd = &cobra.Command{
	Use:   "scan <dir>",
	Short: "Scan a directory for git repos and register them",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		depth, _ := cmd.Flags().GetInt("depth")
		return runRepoScan(cmd.Context(), db, args[0], depth)
	},
}

var repoAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Register a single git repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runRepoAdd(cmd.Context(), db, args[0])
	},
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <name-or-path>",
	Short: "Unregister a repo (does not delete files)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runRepoRemove(cmd.Context(), db, args[0])
	},
}

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked repos",
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		return runRepoList(cmd.Context(), db)
	},
}

func init() {
	repoScanCmd.Flags().IntP("depth", "d", 3, "Max directory depth to scan")
	repoCmd.AddCommand(repoScanCmd, repoAddCmd, repoListCmd, repoRemoveCmd)
}

func runRepoScan(ctx context.Context, db *sqlx.DB, dir string, maxDepth int) error {
	absDir, err := filepath.Abs(expandHome(dir))
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	var added, skipped int
	err = walkGitRepos(absDir, maxDepth, func(repoPath string) error {
		_, insertErr := ensureRepo(ctx, db, repoPath)
		if insertErr != nil {
			fmt.Printf("  skip %s: %v\n", repoPath, insertErr)
			skipped++
			return nil
		}
		fmt.Printf("  + %s\n", repoPath)
		added++
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nFound %d repos", added+skipped)
	if added > 0 {
		fmt.Printf(", registered %d new", added)
	}
	if skipped > 0 {
		fmt.Printf(", skipped %d (already tracked or error)", skipped)
	}
	fmt.Println()
	return nil
}

func runRepoAdd(ctx context.Context, db *sqlx.DB, path string) error {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return fmt.Errorf("%s is not a git repo (no .git found)", abs)
	}
	if _, err := ensureRepo(ctx, db, abs); err != nil {
		return err
	}
	fmt.Printf("Registered: %s\n", abs)
	return nil
}

func runRepoRemove(ctx context.Context, db *sqlx.DB, arg string) error {
	// Try exact path match first, then name match.
	var ids []struct {
		ID   string
		Path string
	}
	rows, err := db.QueryContext(ctx, `SELECT id, path FROM repos WHERE path = ? OR name = ? ORDER BY path`, arg, arg)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r struct{ ID, Path string }
		if err := rows.Scan(&r.ID, &r.Path); err != nil {
			return err
		}
		ids = append(ids, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(ids) == 0 {
		return fmt.Errorf("no repo found matching %q", arg)
	}
	if len(ids) > 1 {
		msg := fmt.Sprintf("ambiguous: %q matches %d repos:\n", arg, len(ids))
		for _, r := range ids {
			msg += fmt.Sprintf("  %s\n", r.Path)
		}
		msg += "Use the full path to be specific."
		return fmt.Errorf("%s", msg)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM repos WHERE id = ?`, ids[0].ID); err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	fmt.Printf("Removed: %s\n", ids[0].Path)
	return nil
}

func runRepoList(ctx context.Context, db *sqlx.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT name, path FROM repos ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		found = true
		var name, path string
		if err := rows.Scan(&name, &path); err != nil {
			return err
		}
		fmt.Printf("  %-30s %s\n", name, path)
	}
	if !found {
		fmt.Println("No repos tracked. Use 'devctl repo scan ~/Projects' to discover repos.")
	}
	return rows.Err()
}

// walkGitRepos calls fn for every git repo found under root up to maxDepth.
// Does not recurse into .git directories or repos themselves (no nested repos).
func walkGitRepos(root string, maxDepth int, fn func(string) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if !d.IsDir() {
			return nil
		}

		// Skip hidden directories (except the root itself).
		if path != root && len(d.Name()) > 0 && d.Name()[0] == '.' {
			return filepath.SkipDir
		}

		// Enforce max depth.
		rel, _ := filepath.Rel(root, path)
		depth := 0
		if rel != "." {
			depth = len(filepath.SplitList(filepath.ToSlash(rel)))
			// Count path separators instead — SplitList uses OS list separator.
			depth = countPathComponents(rel)
		}
		if depth > maxDepth {
			return filepath.SkipDir
		}

		// Check if this directory is a git repo.
		if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
			if fnErr := fn(path); fnErr != nil {
				return fnErr
			}
			// Don't recurse into the repo — no nested repo support.
			return filepath.SkipDir
		}

		return nil
	})
}

func countPathComponents(rel string) int {
	if rel == "" || rel == "." {
		return 0
	}
	count := 1
	for _, c := range rel {
		if c == filepath.Separator {
			count++
		}
	}
	return count
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// resolveRepo resolves a repo argument that may be either:
//   - An absolute or relative filesystem path
//   - A repo name registered in the DB
//
// Returns the absolute path of the repo.
func resolveRepo(ctx context.Context, db *sqlx.DB, arg string) (string, error) {
	// If the arg looks like a path (contains separator, starts with . or ~, or is absolute), resolve it directly.
	if filepath.IsAbs(arg) || arg[0] == '.' || arg[0] == '~' || containsPathSep(arg) {
		abs, err := filepath.Abs(expandHome(arg))
		if err != nil {
			return "", fmt.Errorf("resolve path: %w", err)
		}
		return abs, nil
	}

	// Try name lookup in DB.
	var paths []string
	rows, err := db.QueryContext(ctx, `SELECT path FROM repos WHERE name = ? ORDER BY path`, arg)
	if err != nil {
		return "", fmt.Errorf("lookup repo: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return "", err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	switch len(paths) {
	case 0:
		// Not found by name — fall back to treating it as a relative path.
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", fmt.Errorf("resolve path: %w", err)
		}
		return abs, nil
	case 1:
		return paths[0], nil
	default:
		msg := fmt.Sprintf("ambiguous repo name %q — found %d matches:\n", arg, len(paths))
		for _, p := range paths {
			msg += fmt.Sprintf("  %s\n", p)
		}
		msg += "Use the full path instead."
		return "", fmt.Errorf("%s", msg)
	}
}

func containsPathSep(s string) bool {
	for _, c := range s {
		if c == '/' || c == '\\' {
			return true
		}
	}
	return false
}

// repoNameCompletion returns a ValidArgsFunction that completes repo names from the DB.
func repoNameCompletion(db *sqlx.DB) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		rows, err := db.QueryContext(cmd.Context(), `SELECT name, path FROM repos ORDER BY name`)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var name, path string
			if err := rows.Scan(&name, &path); err != nil {
				continue
			}
			// Format: "name\tpath" — cobra uses the tab-separated part as a description.
			names = append(names, name+"\t"+path)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}

// insertRepo inserts a new repo record. Returns the new ID.
// Callers should prefer ensureRepo (which is idempotent).
func insertRepo(ctx context.Context, db *sqlx.DB, absPath string) (string, error) {
	id := uuid.New().String()
	name := filepath.Base(absPath)
	now := time.Now().Unix()
	_, err := db.ExecContext(ctx, `
		INSERT INTO repos (id, path, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`, id, absPath, name, now, now)
	return id, err
}
