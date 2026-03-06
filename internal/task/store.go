// Package task provides the data layer for task management.
package task

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Task represents a user-defined work item scoped to a repo.
type Task struct {
	ID          string `db:"id"`
	Description string `db:"description"`
	State       string `db:"state"`       // queued, running, completed
	Branch      string `db:"branch"`      // optional linked git branch
	WorktreeID  string `db:"worktree_id"` // optional linked worktree
	RepoID      string `db:"repo_id"`     // scoped to repo
	CreatedAt   int64  `db:"created_at"`
	UpdatedAt   int64  `db:"updated_at"`
}

// validStates are the allowed stored states. "blocked" is computed at runtime.
var validStates = map[string]bool{
	"queued":    true,
	"running":   true,
	"completed": true,
}

// TaskStore provides CRUD operations for tasks backed by SQLite.
type TaskStore struct {
	db *sqlx.DB
}

// NewStore creates a TaskStore.
func NewStore(db *sqlx.DB) *TaskStore {
	return &TaskStore{db: db}
}

// Create inserts a new task with state "queued" and returns it.
func (s *TaskStore) Create(ctx context.Context, description, repoID string) (Task, error) {
	now := time.Now().Unix()
	t := Task{
		ID:          uuid.New().String(),
		Description: description,
		State:       "queued",
		RepoID:      repoID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, description, state, branch, worktree_id, repo_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Description, t.State, t.Branch, t.WorktreeID, t.RepoID, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return Task{}, fmt.Errorf("task create: %w", err)
	}
	return t, nil
}

// List returns all tasks ordered by created_at DESC.
func (s *TaskStore) List(ctx context.Context) ([]Task, error) {
	var tasks []Task
	err := s.db.SelectContext(ctx, &tasks, `SELECT * FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("task list: %w", err)
	}
	return tasks, nil
}

// ListByRepo returns tasks for a specific repo ordered by created_at DESC.
func (s *TaskStore) ListByRepo(ctx context.Context, repoID string) ([]Task, error) {
	var tasks []Task
	err := s.db.SelectContext(ctx, &tasks,
		`SELECT * FROM tasks WHERE repo_id = ? ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, fmt.Errorf("task list by repo: %w", err)
	}
	return tasks, nil
}

// Get returns a single task by ID. Supports prefix matching: if the given id
// is a prefix of exactly one task ID, that task is returned. Returns an error
// if the prefix is ambiguous (matches multiple tasks).
func (s *TaskStore) Get(ctx context.Context, id string) (Task, error) {
	var tasks []Task
	err := s.db.SelectContext(ctx, &tasks,
		`SELECT * FROM tasks WHERE id LIKE ?`, id+"%")
	if err != nil {
		return Task{}, fmt.Errorf("task get: %w", err)
	}
	switch len(tasks) {
	case 0:
		return Task{}, fmt.Errorf("task not found: %s", id)
	case 1:
		return tasks[0], nil
	default:
		return Task{}, fmt.Errorf("ambiguous task id prefix %q matches %d tasks", id, len(tasks))
	}
}

// Update modifies a task's state and branch. State must be one of
// queued/running/completed -- "blocked" is rejected because it is computed.
func (s *TaskStore) Update(ctx context.Context, id string, state string, branch string) error {
	if !validStates[state] {
		return fmt.Errorf("invalid task state %q (allowed: queued, running, completed)", state)
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET state = ?, branch = ?, updated_at = ? WHERE id = ?`,
		state, branch, now, id,
	)
	if err != nil {
		return fmt.Errorf("task update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// Delete removes a task by ID. Associated dependency links are removed by
// CASCADE.
func (s *TaskStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("task delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}
