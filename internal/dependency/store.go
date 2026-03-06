// Package dependency provides the data layer for task dependency links.
package dependency

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Dep represents a directed dependency edge: TaskID depends on DependsOnID.
type Dep struct {
	TaskID      string `db:"task_id"`
	DependsOnID string `db:"depends_on_id"`
	CreatedAt   int64  `db:"created_at"`
}

// DepStore provides operations for managing task dependency links.
type DepStore struct {
	db *sqlx.DB
}

// NewStore creates a DepStore.
func NewStore(db *sqlx.DB) *DepStore {
	return &DepStore{db: db}
}

// Add creates a dependency link. The DB CHECK constraint prevents self-deps.
func (s *DepStore) Add(ctx context.Context, taskID, dependsOnID string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_deps (task_id, depends_on_id, created_at)
		VALUES (?, ?, ?)`,
		taskID, dependsOnID, now,
	)
	if err != nil {
		return fmt.Errorf("dep add: %w", err)
	}
	return nil
}

// Remove deletes a dependency link.
func (s *DepStore) Remove(ctx context.Context, taskID, dependsOnID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM task_deps WHERE task_id = ? AND depends_on_id = ?`,
		taskID, dependsOnID,
	)
	if err != nil {
		return fmt.Errorf("dep remove: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("dependency link not found: %s -> %s", taskID, dependsOnID)
	}
	return nil
}

// List returns all dependencies for a given task (what it depends on).
func (s *DepStore) List(ctx context.Context, taskID string) ([]Dep, error) {
	var deps []Dep
	err := s.db.SelectContext(ctx, &deps,
		`SELECT * FROM task_deps WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, fmt.Errorf("dep list: %w", err)
	}
	return deps, nil
}

// ListAll returns all dependency links (used by the dependency resolver).
func (s *DepStore) ListAll(ctx context.Context) ([]Dep, error) {
	var deps []Dep
	err := s.db.SelectContext(ctx, &deps,
		`SELECT * FROM task_deps ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("dep list all: %w", err)
	}
	return deps, nil
}
