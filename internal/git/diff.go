package git

import (
	"context"
	"fmt"
)

// DiffMode selects which diff to show.
type DiffMode int

const (
	DiffUnstaged DiffMode = iota // git diff (working tree vs index)
	DiffStaged                   // git diff --staged (index vs HEAD)
	DiffVsMain                   // git diff main...HEAD (branch vs main)
	DiffVsOrigin                 // git diff origin/main...HEAD (branch vs origin/main)
)

// Diff runs git diff and returns raw ANSI-colored bytes suitable for viewport display.
// ALWAYS uses --color=always — git strips ANSI by default when output is not a tty.
// worktreePath is the worktree directory (cmd.Dir is set to this path via run()).
// filePath is optional: if non-empty, limits diff to that file.
func Diff(ctx context.Context, worktreePath string, mode DiffMode, filePath string) ([]byte, error) {
	var args []string
	switch mode {
	case DiffUnstaged:
		args = []string{"diff", "--color=always"}
	case DiffStaged:
		args = []string{"diff", "--staged", "--color=always"}
	case DiffVsMain:
		args = []string{"diff", "--color=always", "main...HEAD"}
	case DiffVsOrigin:
		args = []string{"diff", "--color=always", "origin/main...HEAD"}
	default:
		return nil, fmt.Errorf("unknown DiffMode %d", mode)
	}
	if filePath != "" {
		args = append(args, "--", filePath)
	}
	out, err := run(ctx, worktreePath, args...)
	if err != nil {
		return nil, fmt.Errorf("Diff mode=%d: %w", mode, err)
	}
	return out, nil
}
