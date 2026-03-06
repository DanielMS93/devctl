// Package agent provides the data layer for agent workflow runs and patches.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// AgentRun represents a single agent workflow execution for a branch.
type AgentRun struct {
	ID          string  `db:"id"`
	RepoPath    string  `db:"repo_path"`
	Branch      string  `db:"branch"`
	Workflow    string  `db:"workflow"`
	Status      string  `db:"status"`       // pending, running, completed, failed
	TriggeredAt int64   `db:"triggered_at"`
	CompletedAt *int64  `db:"completed_at"`
	ErrorMsg    *string `db:"error_msg"`
}

// AgentPatch represents a generated patch from an agent run.
type AgentPatch struct {
	ID          string  `db:"id"`
	RunID       string  `db:"run_id"`
	RepoPath    string  `db:"repo_path"`
	Branch      string  `db:"branch"`
	Title       string  `db:"title"`
	Description *string `db:"description"`
	PatchData   string  `db:"patch_data"`
	Status      string  `db:"status"` // draft, approved, applied, rejected, reverted
	CreatedAt   int64   `db:"created_at"`
	ReviewedAt  *int64  `db:"reviewed_at"`
	AppliedAt   *int64  `db:"applied_at"`
}

// AgentRunStore provides CRUD operations for agent runs backed by SQLite.
type AgentRunStore struct {
	db *sqlx.DB
}

// NewAgentRunStore creates an AgentRunStore.
func NewAgentRunStore(db *sqlx.DB) *AgentRunStore {
	return &AgentRunStore{db: db}
}

// Create inserts a new agent run and returns it.
func (s *AgentRunStore) Create(ctx context.Context, run AgentRun) (AgentRun, error) {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	if run.Status == "" {
		run.Status = "pending"
	}
	if run.TriggeredAt == 0 {
		run.TriggeredAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_runs (id, repo_path, branch, workflow, status, triggered_at, completed_at, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.RepoPath, run.Branch, run.Workflow, run.Status, run.TriggeredAt, run.CompletedAt, run.ErrorMsg,
	)
	if err != nil {
		return AgentRun{}, fmt.Errorf("agent run create: %w", err)
	}
	return run, nil
}

// Get returns a single agent run by ID.
func (s *AgentRunStore) Get(ctx context.Context, id string) (AgentRun, error) {
	var run AgentRun
	err := s.db.GetContext(ctx, &run, `SELECT * FROM agent_runs WHERE id = ?`, id)
	if err != nil {
		return AgentRun{}, fmt.Errorf("agent run get: %w", err)
	}
	return run, nil
}

// UpdateStatus sets the status (and optional error message) for a run.
// If status is "completed" or "failed", completed_at is set to now.
func (s *AgentRunStore) UpdateStatus(ctx context.Context, id, status string, errorMsg *string) error {
	var completedAt *int64
	if status == "completed" || status == "failed" {
		now := time.Now().Unix()
		completedAt = &now
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE agent_runs SET status = ?, completed_at = ?, error_msg = ? WHERE id = ?`,
		status, completedAt, errorMsg, id,
	)
	if err != nil {
		return fmt.Errorf("agent run update status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent run not found: %s", id)
	}
	return nil
}

// ListByBranch returns all runs for a given repo+branch, ordered by triggered_at DESC.
func (s *AgentRunStore) ListByBranch(ctx context.Context, repoPath, branch string) ([]AgentRun, error) {
	var runs []AgentRun
	err := s.db.SelectContext(ctx, &runs,
		`SELECT * FROM agent_runs WHERE repo_path = ? AND branch = ? ORDER BY triggered_at DESC`,
		repoPath, branch)
	if err != nil {
		return nil, fmt.Errorf("agent run list by branch: %w", err)
	}
	return runs, nil
}

// LastTriggered returns the most recent triggered_at time for the given
// repo+branch+workflow combination. Returns zero time if no runs exist.
func (s *AgentRunStore) LastTriggered(ctx context.Context, repoPath, branch, workflow string) (time.Time, error) {
	var ts *int64
	err := s.db.GetContext(ctx, &ts, `
		SELECT MAX(triggered_at) FROM agent_runs
		WHERE repo_path = ? AND branch = ? AND workflow = ?`,
		repoPath, branch, workflow)
	if err != nil {
		return time.Time{}, fmt.Errorf("agent run last triggered: %w", err)
	}
	if ts == nil {
		return time.Time{}, nil
	}
	return time.Unix(*ts, 0), nil
}

// PatchStore provides CRUD operations for agent patches backed by SQLite.
type PatchStore struct {
	db *sqlx.DB
}

// NewPatchStore creates a PatchStore.
func NewPatchStore(db *sqlx.DB) *PatchStore {
	return &PatchStore{db: db}
}

// Create inserts a new agent patch and returns it.
func (s *PatchStore) Create(ctx context.Context, patch AgentPatch) (AgentPatch, error) {
	if patch.ID == "" {
		patch.ID = uuid.New().String()
	}
	if patch.Status == "" {
		patch.Status = "draft"
	}
	if patch.CreatedAt == 0 {
		patch.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_patches (id, run_id, repo_path, branch, title, description, patch_data, status, created_at, reviewed_at, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		patch.ID, patch.RunID, patch.RepoPath, patch.Branch, patch.Title, patch.Description,
		patch.PatchData, patch.Status, patch.CreatedAt, patch.ReviewedAt, patch.AppliedAt,
	)
	if err != nil {
		return AgentPatch{}, fmt.Errorf("agent patch create: %w", err)
	}
	return patch, nil
}

// Get returns a single patch by ID. Supports UUID prefix matching: if the
// given id is a prefix of exactly one patch ID, that patch is returned.
func (s *PatchStore) Get(ctx context.Context, id string) (AgentPatch, error) {
	var patches []AgentPatch
	err := s.db.SelectContext(ctx, &patches,
		`SELECT * FROM agent_patches WHERE id LIKE ?`, id+"%")
	if err != nil {
		return AgentPatch{}, fmt.Errorf("agent patch get: %w", err)
	}
	switch len(patches) {
	case 0:
		return AgentPatch{}, fmt.Errorf("agent patch not found: %s", id)
	case 1:
		return patches[0], nil
	default:
		return AgentPatch{}, fmt.Errorf("ambiguous patch id prefix %q matches %d patches", id, len(patches))
	}
}

// ListByStatus returns all patches with the given status, ordered by created_at DESC.
func (s *PatchStore) ListByStatus(ctx context.Context, status string) ([]AgentPatch, error) {
	var patches []AgentPatch
	err := s.db.SelectContext(ctx, &patches,
		`SELECT * FROM agent_patches WHERE status = ? ORDER BY created_at DESC`, status)
	if err != nil {
		return nil, fmt.Errorf("agent patch list by status: %w", err)
	}
	return patches, nil
}

// ListByRun returns all patches for a given run, ordered by created_at DESC.
func (s *PatchStore) ListByRun(ctx context.Context, runID string) ([]AgentPatch, error) {
	var patches []AgentPatch
	err := s.db.SelectContext(ctx, &patches,
		`SELECT * FROM agent_patches WHERE run_id = ? ORDER BY created_at DESC`, runID)
	if err != nil {
		return nil, fmt.Errorf("agent patch list by run: %w", err)
	}
	return patches, nil
}

// UpdateStatus sets the status for a patch.
func (s *PatchStore) UpdateStatus(ctx context.Context, id, status string) error {
	now := time.Now().Unix()
	var reviewedAt *int64
	if status == "approved" || status == "rejected" {
		reviewedAt = &now
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE agent_patches SET status = ?, reviewed_at = COALESCE(?, reviewed_at) WHERE id = ?`,
		status, reviewedAt, id,
	)
	if err != nil {
		return fmt.Errorf("agent patch update status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent patch not found: %s", id)
	}
	return nil
}

// SetApplied marks a patch as applied and records the applied_at timestamp.
func (s *PatchStore) SetApplied(ctx context.Context, id string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE agent_patches SET status = 'applied', applied_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("agent patch set applied: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent patch not found: %s", id)
	}
	return nil
}

// SetReverted marks a patch as reverted.
func (s *PatchStore) SetReverted(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE agent_patches SET status = 'reverted' WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("agent patch set reverted: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent patch not found: %s", id)
	}
	return nil
}
