package idea

import (
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		ideas   []Idea
		deps    []IdeaDep
		wantErr bool
		check   func(t *testing.T, result []ResolvedIdea)
	}{
		{
			name:  "no ideas",
			ideas: nil,
			deps:  nil,
			check: func(t *testing.T, result []ResolvedIdea) {
				if len(result) != 0 {
					t.Fatalf("expected 0 results, got %d", len(result))
				}
			},
		},
		{
			name:  "single side-quest no deps is ready",
			ideas: []Idea{{ID: "a", State: "queued", Kind: "side"}},
			deps:  nil,
			check: func(t *testing.T, result []ResolvedIdea) {
				if len(result) != 1 {
					t.Fatalf("expected 1 result, got %d", len(result))
				}
				r := findResolved(result, "a")
				if !r.IsReady {
					t.Error("expected ready")
				}
				if r.IsBlocked {
					t.Error("expected not blocked")
				}
				if r.Layer != 0 {
					t.Errorf("expected layer 0, got %d", r.Layer)
				}
			},
		},
		{
			name:  "completed idea is neither ready nor blocked",
			ideas: []Idea{{ID: "a", State: "completed"}},
			deps:  nil,
			check: func(t *testing.T, result []ResolvedIdea) {
				r := findResolved(result, "a")
				if r.IsReady {
					t.Error("completed should not be ready")
				}
				if r.IsBlocked {
					t.Error("completed should not be blocked")
				}
			},
		},
		{
			name: "sequential chain A->B, A completed",
			ideas: []Idea{
				{ID: "a", State: "completed", Kind: "side"},
				{ID: "b", State: "queued", Kind: "sequential"},
			},
			deps: []IdeaDep{{IdeaID: "b", DependsOnID: "a"}},
			check: func(t *testing.T, result []ResolvedIdea) {
				rb := findResolved(result, "b")
				if !rb.IsReady {
					t.Error("b should be ready")
				}
				if rb.Layer != 1 {
					t.Errorf("b expected layer 1, got %d", rb.Layer)
				}
			},
		},
		{
			name: "sequential chain A->B, A incomplete",
			ideas: []Idea{
				{ID: "a", State: "queued", Kind: "side"},
				{ID: "b", State: "queued", Kind: "sequential"},
			},
			deps: []IdeaDep{{IdeaID: "b", DependsOnID: "a"}},
			check: func(t *testing.T, result []ResolvedIdea) {
				rb := findResolved(result, "b")
				if rb.IsReady {
					t.Error("b should not be ready")
				}
				if !rb.IsBlocked {
					t.Error("b should be blocked")
				}
				if len(rb.BlockedBy) != 1 || rb.BlockedBy[0] != "a" {
					t.Errorf("expected BlockedBy=[a], got %v", rb.BlockedBy)
				}
			},
		},
		{
			name: "running idea skips ready/blocked check",
			ideas: []Idea{
				{ID: "a", State: "running", Kind: "side"},
			},
			deps: nil,
			check: func(t *testing.T, result []ResolvedIdea) {
				r := findResolved(result, "a")
				if r.IsReady {
					t.Error("running should not be ready")
				}
				if r.IsBlocked {
					t.Error("running should not be blocked")
				}
			},
		},
		{
			name: "failed idea skips ready/blocked check",
			ideas: []Idea{
				{ID: "a", State: "failed", Kind: "side"},
			},
			deps: nil,
			check: func(t *testing.T, result []ResolvedIdea) {
				r := findResolved(result, "a")
				if r.IsReady {
					t.Error("failed should not be ready")
				}
			},
		},
		{
			name: "cycle detection",
			ideas: []Idea{
				{ID: "a", State: "queued"},
				{ID: "b", State: "queued"},
			},
			deps: []IdeaDep{
				{IdeaID: "b", DependsOnID: "a"},
				{IdeaID: "a", DependsOnID: "b"},
			},
			wantErr: true,
		},
		{
			name: "three layers",
			ideas: []Idea{
				{ID: "a", State: "queued"},
				{ID: "b", State: "queued"},
				{ID: "c", State: "queued"},
			},
			deps: []IdeaDep{
				{IdeaID: "b", DependsOnID: "a"},
				{IdeaID: "c", DependsOnID: "b"},
			},
			check: func(t *testing.T, result []ResolvedIdea) {
				ra := findResolved(result, "a")
				rb := findResolved(result, "b")
				rc := findResolved(result, "c")
				if ra.Layer != 0 {
					t.Errorf("a expected layer 0, got %d", ra.Layer)
				}
				if rb.Layer != 1 {
					t.Errorf("b expected layer 1, got %d", rb.Layer)
				}
				if rc.Layer != 2 {
					t.Errorf("c expected layer 2, got %d", rc.Layer)
				}
			},
		},
		{
			name: "diamond both deps completed",
			ideas: []Idea{
				{ID: "a", State: "completed"},
				{ID: "b", State: "completed"},
				{ID: "c", State: "queued"},
			},
			deps: []IdeaDep{
				{IdeaID: "c", DependsOnID: "a"},
				{IdeaID: "c", DependsOnID: "b"},
			},
			check: func(t *testing.T, result []ResolvedIdea) {
				rc := findResolved(result, "c")
				if !rc.IsReady {
					t.Error("c should be ready")
				}
				if rc.Layer != 1 {
					t.Errorf("c expected layer 1, got %d", rc.Layer)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Resolve(tt.ideas, tt.deps)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func findResolved(results []ResolvedIdea, id string) ResolvedIdea {
	for _, r := range results {
		if r.Idea.ID == id {
			return r
		}
	}
	panic("resolved idea not found: " + id)
}
