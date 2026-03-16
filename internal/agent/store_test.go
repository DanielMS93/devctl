package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DanielMS93/devctl/pkg/storage"
	"github.com/jmoiron/sqlx"
)

// testDB creates a temporary SQLite database with all migrations applied.
func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.RunMigrations(dbPath); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func TestAgentRunStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	store := NewAgentRunStore(db)
	ctx := context.Background()

	run, err := store.Create(ctx, AgentRun{
		RepoPath: "/tmp/repo",
		Branch:   "main",
		Workflow: "code_review",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if run.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if run.Status != "pending" {
		t.Errorf("expected status pending, got %s", run.Status)
	}

	got, err := store.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RepoPath != "/tmp/repo" {
		t.Errorf("expected repo_path /tmp/repo, got %s", got.RepoPath)
	}
	if got.Workflow != "code_review" {
		t.Errorf("expected workflow code_review, got %s", got.Workflow)
	}
}

func TestAgentRunStore_UpdateStatus(t *testing.T) {
	db := testDB(t)
	store := NewAgentRunStore(db)
	ctx := context.Background()

	run, _ := store.Create(ctx, AgentRun{
		RepoPath: "/tmp/repo",
		Branch:   "main",
		Workflow: "code_review",
	})

	errMsg := "something broke"
	if err := store.UpdateStatus(ctx, run.ID, "failed", &errMsg); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := store.Get(ctx, run.ID)
	if got.Status != "failed" {
		t.Errorf("expected status failed, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if got.ErrorMsg == nil || *got.ErrorMsg != errMsg {
		t.Errorf("expected error_msg %q, got %v", errMsg, got.ErrorMsg)
	}
}

func TestAgentRunStore_ListByBranch(t *testing.T) {
	db := testDB(t)
	store := NewAgentRunStore(db)
	ctx := context.Background()

	store.Create(ctx, AgentRun{RepoPath: "/tmp/repo", Branch: "feat-a", Workflow: "code_review"})
	store.Create(ctx, AgentRun{RepoPath: "/tmp/repo", Branch: "feat-b", Workflow: "code_review"})
	store.Create(ctx, AgentRun{RepoPath: "/tmp/repo", Branch: "feat-a", Workflow: "test_gen"})

	runs, err := store.ListByBranch(ctx, "/tmp/repo", "feat-a")
	if err != nil {
		t.Fatalf("list by branch: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs for feat-a, got %d", len(runs))
	}
}

func TestAgentRunStore_LastTriggered(t *testing.T) {
	db := testDB(t)
	store := NewAgentRunStore(db)
	ctx := context.Background()

	// No runs yet — should return zero time.
	ts, err := store.LastTriggered(ctx, "/tmp/repo", "main", "code_review")
	if err != nil {
		t.Fatalf("last triggered: %v", err)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time, got %v", ts)
	}

	store.Create(ctx, AgentRun{RepoPath: "/tmp/repo", Branch: "main", Workflow: "code_review", TriggeredAt: 1000})
	store.Create(ctx, AgentRun{RepoPath: "/tmp/repo", Branch: "main", Workflow: "code_review", TriggeredAt: 2000})

	ts, err = store.LastTriggered(ctx, "/tmp/repo", "main", "code_review")
	if err != nil {
		t.Fatalf("last triggered: %v", err)
	}
	if ts.Unix() != 2000 {
		t.Errorf("expected 2000, got %d", ts.Unix())
	}
}

func TestPatchStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	runStore := NewAgentRunStore(db)
	patchStore := NewPatchStore(db)
	ctx := context.Background()

	run, _ := runStore.Create(ctx, AgentRun{
		RepoPath: "/tmp/repo",
		Branch:   "main",
		Workflow: "code_review",
	})

	patch, err := patchStore.Create(ctx, AgentPatch{
		RunID:     run.ID,
		RepoPath:  "/tmp/repo",
		Branch:    "main",
		Title:     "Fix null check",
		PatchData: "diff --git a/foo.go b/foo.go\n...",
	})
	if err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if patch.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if patch.Status != "draft" {
		t.Errorf("expected status draft, got %s", patch.Status)
	}

	got, err := patchStore.Get(ctx, patch.ID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if got.Title != "Fix null check" {
		t.Errorf("expected title 'Fix null check', got %s", got.Title)
	}
}

func TestPatchStore_GetPrefixMatch(t *testing.T) {
	db := testDB(t)
	runStore := NewAgentRunStore(db)
	patchStore := NewPatchStore(db)
	ctx := context.Background()

	run, _ := runStore.Create(ctx, AgentRun{
		RepoPath: "/tmp/repo",
		Branch:   "main",
		Workflow: "code_review",
	})

	patch, _ := patchStore.Create(ctx, AgentPatch{
		RunID:     run.ID,
		RepoPath:  "/tmp/repo",
		Branch:    "main",
		Title:     "Fix something",
		PatchData: "diff data",
	})

	// First 8 chars should match.
	prefix := patch.ID[:8]
	got, err := patchStore.Get(ctx, prefix)
	if err != nil {
		t.Fatalf("get by prefix: %v", err)
	}
	if got.ID != patch.ID {
		t.Errorf("expected ID %s, got %s", patch.ID, got.ID)
	}
}

func TestPatchStore_ListByStatus(t *testing.T) {
	db := testDB(t)
	runStore := NewAgentRunStore(db)
	patchStore := NewPatchStore(db)
	ctx := context.Background()

	run, _ := runStore.Create(ctx, AgentRun{
		RepoPath: "/tmp/repo",
		Branch:   "main",
		Workflow: "code_review",
	})

	patchStore.Create(ctx, AgentPatch{
		RunID: run.ID, RepoPath: "/tmp/repo", Branch: "main",
		Title: "Patch A", PatchData: "diff a", Status: "draft",
	})
	patchStore.Create(ctx, AgentPatch{
		RunID: run.ID, RepoPath: "/tmp/repo", Branch: "main",
		Title: "Patch B", PatchData: "diff b", Status: "approved",
	})
	patchStore.Create(ctx, AgentPatch{
		RunID: run.ID, RepoPath: "/tmp/repo", Branch: "main",
		Title: "Patch C", PatchData: "diff c", Status: "draft",
	})

	drafts, err := patchStore.ListByStatus(ctx, "draft")
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if len(drafts) != 2 {
		t.Fatalf("expected 2 draft patches, got %d", len(drafts))
	}

	approved, err := patchStore.ListByStatus(ctx, "approved")
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if len(approved) != 1 {
		t.Fatalf("expected 1 approved patch, got %d", len(approved))
	}
}

func TestPatchStore_SetAppliedAndReverted(t *testing.T) {
	db := testDB(t)
	runStore := NewAgentRunStore(db)
	patchStore := NewPatchStore(db)
	ctx := context.Background()

	run, _ := runStore.Create(ctx, AgentRun{
		RepoPath: "/tmp/repo",
		Branch:   "main",
		Workflow: "code_review",
	})

	patch, _ := patchStore.Create(ctx, AgentPatch{
		RunID: run.ID, RepoPath: "/tmp/repo", Branch: "main",
		Title: "Patch X", PatchData: "diff x",
	})

	if err := patchStore.SetApplied(ctx, patch.ID); err != nil {
		t.Fatalf("set applied: %v", err)
	}
	got, _ := patchStore.Get(ctx, patch.ID)
	if got.Status != "applied" {
		t.Errorf("expected status applied, got %s", got.Status)
	}
	if got.AppliedAt == nil {
		t.Error("expected applied_at to be set")
	}

	if err := patchStore.SetReverted(ctx, patch.ID); err != nil {
		t.Fatalf("set reverted: %v", err)
	}
	got, _ = patchStore.Get(ctx, patch.ID)
	if got.Status != "reverted" {
		t.Errorf("expected status reverted, got %s", got.Status)
	}
}

