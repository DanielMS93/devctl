package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/danielmiessler/devctl/internal/dashboard"
	"github.com/danielmiessler/devctl/pkg/storage"
	"github.com/danielmiessler/devctl/pkg/tui"
)

func main() {
	// Root context: cancelled on exit to shut down all goroutines cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Determine data directory: ~/.devctl/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "devctl: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	dataDir := filepath.Join(homeDir, ".devctl")
	dbPath := filepath.Join(dataDir, "state.db")
	logPath := filepath.Join(dataDir, "devctl.log")

	// Structured log file (non-interactive output; TUI owns the terminal).
	logFile, err := openLogFile(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "devctl: cannot open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Open database (creates ~/.devctl/ if needed, sets WAL + pragmas).
	db, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "devctl: open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Apply migrations (idempotent; safe to run on every startup).
	if err := storage.RunMigrations(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "devctl: apply migrations: %v\n", err)
		os.Exit(1)
	}

	// Background state manager (goroutines not yet started).
	mgr := dashboard.NewManager(db)
	mgr.Start(ctx)
	defer mgr.Stop()

	// Build the Cobra command tree.
	root := &cobra.Command{
		Use:   "devctl",
		Short: "Developer control plane for repos, worktrees, sessions, and tasks",
		Long: `devctl gives you one place to see everything happening across all your
repos and worktrees — no lost sessions, no forgotten branches, no missed follow-ups.`,
		SilenceUsage: true,
		// Running bare `devctl` opens the dashboard directly.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboard(mgr)
		},
		// PersistentPreRunE stores the DB in context so all subcommands can access it
		// without global state. The db variable is captured from the main() scope above.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetContext(context.WithValue(cmd.Context(), dbKey{}, db))
			return nil
		},
	}

	root.AddCommand(dashboardCmd(mgr))
	root.AddCommand(worktreeCmd)
	root.AddCommand(configCmd)
	root.AddCommand(repoCmd)
	root.AddCommand(taskCmd)
	root.AddCommand(depsCmd)
	root.AddCommand(agentCmd)

	// Shell completion: worktree create <repo> completes from registered repo names.
	worktreeCreateCmd.ValidArgsFunction = repoNameCompletion(db)

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

// runDashboard opens the TUI dashboard. Used by both the bare `devctl` command and `devctl dashboard`.
func runDashboard(mgr *dashboard.Manager) error {
	m := tui.NewRootModel(mgr.Events())
	// v2: AltScreen is set declaratively in View(); do NOT use tea.WithAltScreen().
	// v2: Use p.Run(), not p.Start() (removed in v2).
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// dashboardCmd returns the `devctl dashboard` subcommand (kept for discoverability/scripts).
func dashboardCmd(mgr *dashboard.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Open the interactive dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboard(mgr)
		},
	}
}

// openLogFile creates (or appends to) the devctl log file, creating parent dirs.
func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}
