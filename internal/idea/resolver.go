package idea

import "fmt"

// ResolvedIdea holds an idea along with its computed DAG status.
type ResolvedIdea struct {
	Idea      Idea
	IsReady   bool     // all deps completed
	IsBlocked bool     // at least one dep incomplete
	BlockedBy []string // IDs of blocking upstream ideas
	Layer     int      // topological layer (0 = no deps)
}

// Resolve performs topological sort (Kahn's algorithm) on the idea graph
// and computes ready/blocked status for each idea.
func Resolve(ideas []Idea, deps []IdeaDep) ([]ResolvedIdea, error) {
	if len(ideas) == 0 {
		return nil, nil
	}

	ideaByID := make(map[string]Idea, len(ideas))
	for _, i := range ideas {
		ideaByID[i.ID] = i
	}

	// Build adjacency.
	upstream := make(map[string][]string, len(ideas))
	downstream := make(map[string][]string, len(ideas))
	inDegree := make(map[string]int, len(ideas))

	for _, i := range ideas {
		inDegree[i.ID] = 0
	}

	for _, d := range deps {
		// Only count deps where both sides exist in our idea set.
		if _, ok := ideaByID[d.IdeaID]; !ok {
			continue
		}
		if _, ok := ideaByID[d.DependsOnID]; !ok {
			continue
		}
		upstream[d.IdeaID] = append(upstream[d.IdeaID], d.DependsOnID)
		downstream[d.DependsOnID] = append(downstream[d.DependsOnID], d.IdeaID)
		inDegree[d.IdeaID]++
	}

	// Kahn's algorithm: layered topological sort.
	layer := make(map[string]int, len(ideas))
	var queue []string
	for _, i := range ideas {
		if inDegree[i.ID] == 0 {
			queue = append(queue, i.ID)
			layer[i.ID] = 0
		}
	}

	processed := 0
	for len(queue) > 0 {
		var next []string
		for _, id := range queue {
			processed++
			for _, downID := range downstream[id] {
				inDegree[downID]--
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

	if processed != len(ideas) {
		return nil, fmt.Errorf("dependency cycle detected: %d ideas in cycle", len(ideas)-processed)
	}

	// Compute ready/blocked status.
	result := make([]ResolvedIdea, 0, len(ideas))
	for _, i := range ideas {
		ri := ResolvedIdea{
			Idea:  i,
			Layer: layer[i.ID],
		}

		if i.State == "completed" || i.State == "failed" || i.State == "running" {
			result = append(result, ri)
			continue
		}

		// Check upstream deps for blocking conditions.
		for _, upID := range upstream[i.ID] {
			upIdea := ideaByID[upID]
			if upIdea.State != "completed" {
				ri.IsBlocked = true
				ri.BlockedBy = append(ri.BlockedBy, upID)
			}
		}

		if !ri.IsBlocked {
			ri.IsReady = true
		}

		result = append(result, ri)
	}

	return result, nil
}
