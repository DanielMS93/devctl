package task

import (
	"fmt"

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
	if len(tasks) == 0 {
		return nil, nil
	}

	// Index tasks by ID.
	taskByID := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		taskByID[t.ID] = t
	}

	// Build adjacency: upstream[taskID] = list of dependency IDs (what it depends on).
	// downstream[depID] = list of task IDs that depend on depID.
	upstream := make(map[string][]string, len(tasks))
	downstream := make(map[string][]string, len(tasks))
	inDegree := make(map[string]int, len(tasks))

	for _, t := range tasks {
		inDegree[t.ID] = 0
	}

	for _, d := range deps {
		upstream[d.TaskID] = append(upstream[d.TaskID], d.DependsOnID)
		downstream[d.DependsOnID] = append(downstream[d.DependsOnID], d.TaskID)
		inDegree[d.TaskID]++
	}

	// Kahn's algorithm: layered topological sort.
	layer := make(map[string]int, len(tasks))
	var queue []string
	for _, t := range tasks {
		if inDegree[t.ID] == 0 {
			queue = append(queue, t.ID)
			layer[t.ID] = 0
		}
	}

	processed := 0
	for len(queue) > 0 {
		// Process all tasks at the current layer.
		var next []string
		for _, id := range queue {
			processed++
			for _, downID := range downstream[id] {
				inDegree[downID]--
				// Layer of downstream is max(current layers of its upstreams) + 1.
				if candidate := layer[id] + 1; candidate > layer[downID] {
					layer[downID] = candidate
				}
				if inDegree[downID] == 0 {
					next = append(next, downID)
				}
			}
		}
		queue = next
	}

	if processed != len(tasks) {
		return nil, fmt.Errorf("dependency cycle detected: %d tasks in cycle", len(tasks)-processed)
	}

	// Compute ready/blocked status.
	result := make([]ResolvedTask, 0, len(tasks))
	for _, t := range tasks {
		rt := ResolvedTask{
			Task:  t,
			Layer: layer[t.ID],
		}

		if t.State == "completed" {
			// Completed tasks are neither ready nor blocked.
			result = append(result, rt)
			continue
		}

		// Check upstream deps for blocking conditions.
		for _, upID := range upstream[t.ID] {
			upTask := taskByID[upID]
			blocked := false

			if upTask.State != "completed" {
				blocked = true
			} else if upTask.Branch != "" {
				// Completed but has a branch: check if merged.
				merged, ok := branchMerged[upID]
				if ok && !merged {
					blocked = true
				}
			}

			if blocked {
				rt.IsBlocked = true
				rt.BlockedBy = append(rt.BlockedBy, upID)
			}
		}

		if !rt.IsBlocked {
			rt.IsReady = true
		}

		result = append(result, rt)
	}

	return result, nil
}
