package idea

import (
	"context"
	"testing"

	"github.com/danielmiessler/devctl/pkg/storage"
)

func testDB(t *testing.T) *Store {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(dbPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestStoreCRUD(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()

	// Create.
	i, err := store.Create(ctx, "investigate redis caching", "/repo", "side", "session-123", "main")
	if err != nil {
		t.Fatal(err)
	}
	if i.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if i.State != "queued" {
		t.Errorf("expected state=queued, got %s", i.State)
	}
	if i.Kind != "side" {
		t.Errorf("expected kind=side, got %s", i.Kind)
	}

	// Get.
	got, err := store.Get(ctx, i.ID[:8])
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != i.ID {
		t.Errorf("expected ID=%s, got %s", i.ID, got.ID)
	}
	if got.Prompt != "investigate redis caching" {
		t.Errorf("expected prompt match")
	}

	// List.
	ideas, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ideas) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(ideas))
	}

	// ListByState.
	queued, err := store.ListByState(ctx, "queued")
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 {
		t.Fatalf("expected 1 queued, got %d", len(queued))
	}

	// ListByRepo.
	byRepo, err := store.ListByRepo(ctx, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(byRepo) != 1 {
		t.Fatalf("expected 1 by repo, got %d", len(byRepo))
	}

	// SetRunning.
	err = store.SetRunning(ctx, i.ID, "session-456", "/tmp/wt", "idea/abc")
	if err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, i.ID)
	if got.State != "running" {
		t.Errorf("expected state=running, got %s", got.State)
	}
	if got.WorktreePath != "/tmp/wt" {
		t.Errorf("expected worktree path")
	}

	// SetRunning again should fail (already running).
	err = store.SetRunning(ctx, i.ID, "x", "y", "z")
	if err == nil {
		t.Error("expected error on double SetRunning")
	}

	// SetCompleted.
	err = store.SetCompleted(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, i.ID)
	if got.State != "completed" {
		t.Errorf("expected state=completed, got %s", got.State)
	}

	// SetIncorporated.
	err = store.SetIncorporated(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, i.ID)
	if got.Incorporated != 1 {
		t.Error("expected incorporated=1")
	}

	// Delete.
	err = store.Delete(ctx, i.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(ctx, i.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSetFailed(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()

	i, _ := store.Create(ctx, "test", "/repo", "side", "", "")
	err := store.SetFailed(ctx, i.ID, "something broke")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(ctx, i.ID)
	if got.State != "failed" {
		t.Errorf("expected state=failed, got %s", got.State)
	}
	if got.ErrorMsg == nil || *got.ErrorMsg != "something broke" {
		t.Error("expected error message")
	}
}

func TestDepsCycleDetection(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()

	a, _ := store.Create(ctx, "idea a", "/repo", "side", "", "")
	b, _ := store.Create(ctx, "idea b", "/repo", "sequential", "", "")

	// A -> B (B depends on A).
	err := store.AddDep(ctx, b.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}

	// B -> A would create a cycle.
	err = store.AddDep(ctx, a.ID, b.ID)
	if err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestDepsOperations(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()

	a, _ := store.Create(ctx, "idea a", "/repo", "side", "", "")
	b, _ := store.Create(ctx, "idea b", "/repo", "sequential", "", "")
	c, _ := store.Create(ctx, "idea c", "/repo", "sequential", "", "")

	// Add deps: C depends on A and B.
	if err := store.AddDep(ctx, c.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.AddDep(ctx, c.ID, b.ID); err != nil {
		t.Fatal(err)
	}

	// List deps for C.
	deps, err := store.ListDeps(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}

	// ListAllDeps.
	all, err := store.ListAllDeps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 total deps, got %d", len(all))
	}

	// RemoveDep.
	err = store.RemoveDep(ctx, c.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	deps, _ = store.ListDeps(ctx, c.ID)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep after remove, got %d", len(deps))
	}
}

func TestDefaultKind(t *testing.T) {
	store := testDB(t)
	ctx := context.Background()

	i, err := store.Create(ctx, "test", "/repo", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if i.Kind != "side" {
		t.Errorf("expected default kind=side, got %s", i.Kind)
	}
}
