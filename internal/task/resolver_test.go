package task

import (
	"testing"

	"github.com/DanielMS93/devctl/internal/dependency"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		tasks        []Task
		deps         []dependency.Dep
		branchMerged map[string]bool
		wantErr      bool
		check        func(t *testing.T, result []ResolvedTask)
	}{
		{
			name:  "no tasks",
			tasks: nil,
			deps:  nil,
			check: func(t *testing.T, result []ResolvedTask) {
				if len(result) != 0 {
					t.Fatalf("expected 0 results, got %d", len(result))
				}
			},
		},
		{
			name:  "single task no deps",
			tasks: []Task{{ID: "a", State: "queued"}},
			deps:  nil,
			check: func(t *testing.T, result []ResolvedTask) {
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
			name:  "completed task",
			tasks: []Task{{ID: "a", State: "completed"}},
			deps:  nil,
			check: func(t *testing.T, result []ResolvedTask) {
				r := findResolved(result, "a")
				if r.IsReady {
					t.Error("completed task should not be ready")
				}
				if r.IsBlocked {
					t.Error("completed task should not be blocked")
				}
				if r.Layer != 0 {
					t.Errorf("expected layer 0, got %d", r.Layer)
				}
			},
		},
		{
			name: "linear chain A->B, A completed",
			tasks: []Task{
				{ID: "a", State: "completed"},
				{ID: "b", State: "queued"},
			},
			deps: []dependency.Dep{{TaskID: "b", DependsOnID: "a"}},
			check: func(t *testing.T, result []ResolvedTask) {
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
			name: "linear chain A->B, A incomplete",
			tasks: []Task{
				{ID: "a", State: "queued"},
				{ID: "b", State: "queued"},
			},
			deps: []dependency.Dep{{TaskID: "b", DependsOnID: "a"}},
			check: func(t *testing.T, result []ResolvedTask) {
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
			name: "diamond A->C, B->C, both completed",
			tasks: []Task{
				{ID: "a", State: "completed"},
				{ID: "b", State: "completed"},
				{ID: "c", State: "queued"},
			},
			deps: []dependency.Dep{
				{TaskID: "c", DependsOnID: "a"},
				{TaskID: "c", DependsOnID: "b"},
			},
			check: func(t *testing.T, result []ResolvedTask) {
				rc := findResolved(result, "c")
				if !rc.IsReady {
					t.Error("c should be ready")
				}
				if rc.Layer != 1 {
					t.Errorf("c expected layer 1, got %d", rc.Layer)
				}
			},
		},
		{
			name: "diamond partial, B incomplete",
			tasks: []Task{
				{ID: "a", State: "completed"},
				{ID: "b", State: "queued"},
				{ID: "c", State: "queued"},
			},
			deps: []dependency.Dep{
				{TaskID: "c", DependsOnID: "a"},
				{TaskID: "c", DependsOnID: "b"},
			},
			check: func(t *testing.T, result []ResolvedTask) {
				rc := findResolved(result, "c")
				if rc.IsReady {
					t.Error("c should not be ready")
				}
				if !rc.IsBlocked {
					t.Error("c should be blocked")
				}
				if len(rc.BlockedBy) != 1 || rc.BlockedBy[0] != "b" {
					t.Errorf("expected BlockedBy=[b], got %v", rc.BlockedBy)
				}
			},
		},
		{
			name: "branch not merged blocks",
			tasks: []Task{
				{ID: "a", State: "completed", Branch: "feat-a"},
				{ID: "b", State: "queued"},
			},
			deps:         []dependency.Dep{{TaskID: "b", DependsOnID: "a"}},
			branchMerged: map[string]bool{"a": false},
			check: func(t *testing.T, result []ResolvedTask) {
				rb := findResolved(result, "b")
				if rb.IsReady {
					t.Error("b should not be ready when upstream branch not merged")
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
			name: "branch merged allows",
			tasks: []Task{
				{ID: "a", State: "completed", Branch: "feat-a"},
				{ID: "b", State: "queued"},
			},
			deps:         []dependency.Dep{{TaskID: "b", DependsOnID: "a"}},
			branchMerged: map[string]bool{"a": true},
			check: func(t *testing.T, result []ResolvedTask) {
				rb := findResolved(result, "b")
				if !rb.IsReady {
					t.Error("b should be ready when upstream branch is merged")
				}
			},
		},
		{
			name: "running task can be blocked",
			tasks: []Task{
				{ID: "a", State: "queued"},
				{ID: "b", State: "running"},
			},
			deps: []dependency.Dep{{TaskID: "b", DependsOnID: "a"}},
			check: func(t *testing.T, result []ResolvedTask) {
				rb := findResolved(result, "b")
				if !rb.IsBlocked {
					t.Error("running task b should be blocked when dep a is incomplete")
				}
				if len(rb.BlockedBy) != 1 || rb.BlockedBy[0] != "a" {
					t.Errorf("expected BlockedBy=[a], got %v", rb.BlockedBy)
				}
			},
		},
		{
			name: "cycle detection",
			tasks: []Task{
				{ID: "a", State: "queued"},
				{ID: "b", State: "queued"},
			},
			deps: []dependency.Dep{
				{TaskID: "b", DependsOnID: "a"},
				{TaskID: "a", DependsOnID: "b"},
			},
			wantErr: true,
		},
		{
			name: "three layers",
			tasks: []Task{
				{ID: "a", State: "queued"},
				{ID: "b", State: "queued"},
				{ID: "c", State: "queued"},
			},
			deps: []dependency.Dep{
				{TaskID: "b", DependsOnID: "a"},
				{TaskID: "c", DependsOnID: "b"},
			},
			check: func(t *testing.T, result []ResolvedTask) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := tt.branchMerged
			if merged == nil {
				merged = map[string]bool{}
			}
			result, err := Resolve(tt.tasks, tt.deps, merged)
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

// findResolved finds a ResolvedTask by task ID or panics.
func findResolved(results []ResolvedTask, id string) ResolvedTask {
	for _, r := range results {
		if r.Task.ID == id {
			return r
		}
	}
	panic("resolved task not found: " + id)
}
