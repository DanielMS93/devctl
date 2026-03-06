package task

import (
	"errors"

	"github.com/danielmiessler/devctl/internal/dependency"
)

// ResolvedTask holds a task along with its computed DAG status.
type ResolvedTask struct {
	Task      Task
	IsReady   bool     // all deps completed AND branches merged
	IsBlocked bool     // at least one dep incomplete or branch unmerged
	BlockedBy []string // IDs of blocking upstream tasks
	Layer     int      // topological layer (0 = no deps)
}

// Resolve performs topological sort (Kahn's algorithm) on the task graph
// and computes ready/blocked status for each task.
func Resolve(tasks []Task, deps []dependency.Dep, branchMerged map[string]bool) ([]ResolvedTask, error) {
	return nil, errors.New("not implemented")
}
