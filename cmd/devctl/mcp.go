package main

import (
	"github.com/danielmiessler/devctl/internal/mcp"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server for Claude Code integration",
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP stdio JSON-RPC server",
	Long: `Starts a Model Context Protocol server on stdin/stdout.

Configure in Claude Code's settings:
  "mcpServers": {
    "devctl": {
      "command": "devctl",
      "args": ["mcp", "serve"]
    }
  }

Provides two tools:
  side_quest - Spawn a parallel Claude session in a worktree
  main_quest - Incorporate findings from a completed side-quest`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db := cmd.Context().Value(dbKey{}).(*sqlx.DB)
		server := mcp.NewServer(db)
		return server.Serve(cmd.Context())
	},
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
}
