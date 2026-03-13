// Package idea provides the data layer and execution engine for the quest-based
// idea pipeline — parallel/sequential Claude sessions in worktrees.
package idea

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Idea represents a quest: a prompt to be executed in a parallel Claude session.
type Idea struct {
	ID              string  `db:"id"`
	Prompt          string  `db:"prompt"`
	State           string  `db:"state"`             // queued, ready, running, completed, failed
	Kind            string  `db:"kind"`              // side, sequential
	RepoID          string  `db:"repo_id"`           // scoped to repo
	Branch          string  `db:"branch"`            // idea/<id[:8]>
	ParentBranch    string  `db:"parent_branch"`     // branch to merge back into
	WorktreePath    string  `db:"worktree_path"`     // path to the idea's worktree
	ParentSessionID string  `db:"parent_session_id"` // JSONL session that spawned this
	SessionID       string  `db:"session_id"`        // JSONL session of the running idea
	Incorporated    int     `db:"incorporated"`      // 1 if /main-quest has incorporated
	ErrorMsg        *string `db:"error_msg"`
	CreatedAt       int64   `db:"created_at"`
	UpdatedAt       int64   `db:"updated_at"`
	StartedAt       *int64  `db:"started_at"`
	CompletedAt     *int64  `db:"completed_at"`
}

// IdeaDep represents a directed dependency edge: IdeaID depends on DependsOnID.
type IdeaDep struct {
	IdeaID      string `db:"idea_id"`
	DependsOnID string `db:"depends_on_id"`
	CreatedAt   int64  `db:"created_at"`
}

// Store provides CRUD operations for ideas and their dependencies.
type Store struct {
	db *sqlx.DB
}

// NewStore creates an idea Store.
func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new idea with state "queued" and returns it.
func (s *Store) Create(ctx context.Context, prompt, repoID, kind, parentSessionID, parentBranch string) (Idea, error) {
	if kind == "" {
		kind = "side"
	}
	now := time.Now().Unix()
	idea := Idea{
		ID:              uuid.New().String(),
		Prompt:          prompt,
		State:           "queued",
		Kind:            kind,
		RepoID:          repoID,
		ParentSessionID: parentSessionID,
		ParentBranch:    parentBranch,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ideas (id, prompt, state, kind, repo_id, branch, parent_branch, worktree_path,
		                   parent_session_id, session_id, incorporated, error_msg,
		                   created_at, updated_at, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		idea.ID, idea.Prompt, idea.State, idea.Kind, idea.RepoID,
		idea.Branch, idea.ParentBranch, idea.WorktreePath,
		idea.ParentSessionID, idea.SessionID, idea.Incorporated, idea.ErrorMsg,
		idea.CreatedAt, idea.UpdatedAt, idea.StartedAt, idea.CompletedAt,
	)
	if err != nil {
		return Idea{}, fmt.Errorf("idea create: %w", err)
	}
	return idea, nil
}

// Get returns a single idea by ID with prefix matching.
func (s *Store) Get(ctx context.Context, id string) (Idea, error) {
	var ideas []Idea
	err := s.db.SelectContext(ctx, &ideas,
		`SELECT * FROM ideas WHERE id LIKE ?`, id+"%")
	if err != nil {
		return Idea{}, fmt.Errorf("idea get: %w", err)
	}
	switch len(ideas) {
	case 0:
		return Idea{}, fmt.Errorf("idea not found: %s", id)
	case 1:
		return ideas[0], nil
	default:
		return Idea{}, fmt.Errorf("ambiguous idea id prefix %q matches %d ideas", id, len(ideas))
	}
}

// List returns all ideas ordered by created_at DESC.
func (s *Store) List(ctx context.Context) ([]Idea, error) {
	var ideas []Idea
	err := s.db.SelectContext(ctx, &ideas, `SELECT * FROM ideas ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("idea list: %w", err)
	}
	return ideas, nil
}

// ListByState returns ideas with the given state.
func (s *Store) ListByState(ctx context.Context, state string) ([]Idea, error) {
	var ideas []Idea
	err := s.db.SelectContext(ctx, &ideas,
		`SELECT * FROM ideas WHERE state = ? ORDER BY created_at DESC`, state)
	if err != nil {
		return nil, fmt.Errorf("idea list by state: %w", err)
	}
	return ideas, nil
}

// ListByRepo returns ideas for a specific repo.
func (s *Store) ListByRepo(ctx context.Context, repoID string) ([]Idea, error) {
	var ideas []Idea
	err := s.db.SelectContext(ctx, &ideas,
		`SELECT * FROM ideas WHERE repo_id = ? ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, fmt.Errorf("idea list by repo: %w", err)
	}
	return ideas, nil
}

// SetRunning atomically transitions an idea to running state.
// Returns error if the idea is not in a launchable state (queued or ready).
func (s *Store) SetRunning(ctx context.Context, id, sessionID, worktreePath, branch string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE ideas SET state = 'running', session_id = ?, worktree_path = ?, branch = ?,
		                 started_at = ?, updated_at = ?
		WHERE id = ? AND state IN ('queued', 'ready')`,
		sessionID, worktreePath, branch, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("idea set running: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("idea %s not in launchable state (queued/ready)", id)
	}
	return nil
}

// SetCompleted marks an idea as completed.
func (s *Store) SetCompleted(ctx context.Context, id string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE ideas SET state = 'completed', completed_at = ?, updated_at = ?
		WHERE id = ?`, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("idea set completed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("idea not found: %s", id)
	}
	return nil
}

// SetFailed marks an idea as failed with an error message.
func (s *Store) SetFailed(ctx context.Context, id string, errMsg string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE ideas SET state = 'failed', error_msg = ?, completed_at = ?, updated_at = ?
		WHERE id = ?`, errMsg, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("idea set failed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("idea not found: %s", id)
	}
	return nil
}

// SetIncorporated marks an idea as incorporated by /main-quest.
func (s *Store) SetIncorporated(ctx context.Context, id string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE ideas SET incorporated = 1, updated_at = ? WHERE id = ?`, now, id,
	)
	if err != nil {
		return fmt.Errorf("idea set incorporated: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("idea not found: %s", id)
	}
	return nil
}

// Delete removes an idea by ID. Associated deps are removed by CASCADE.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ideas WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("idea delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("idea not found: %s", id)
	}
	return nil
}

// AddDep creates a dependency link with DFS cycle detection.
func (s *Store) AddDep(ctx context.Context, ideaID, dependsOnID string) error {
	// Cycle detection: DFS from dependsOnID — if it can reach ideaID, adding this edge creates a cycle.
	deps, err := s.ListAllDeps(ctx)
	if err != nil {
		return err
	}

	// Build adjacency: downstream[A] = [B, C] means A depends on B and C.
	downstream := make(map[string][]string)
	for _, d := range deps {
		downstream[d.IdeaID] = append(downstream[d.IdeaID], d.DependsOnID)
	}
	// Simulate adding the new edge.
	downstream[ideaID] = append(downstream[ideaID], dependsOnID)

	// DFS from ideaID following dependency edges — if we reach ideaID again, it's a cycle.
	visited := make(map[string]bool)
	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		if visited[node] {
			return false
		}
		visited[node] = true
		for _, dep := range downstream[node] {
			if dep == ideaID {
				return true
			}
			if hasCycle(dep) {
				return true
			}
		}
		return false
	}

	if hasCycle(dependsOnID) {
		return fmt.Errorf("adding dependency %s -> %s would create a cycle", ideaID, dependsOnID)
	}

	now := time.Now().Unix()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO idea_deps (idea_id, depends_on_id, created_at)
		VALUES (?, ?, ?)`, ideaID, dependsOnID, now,
	)
	if err != nil {
		return fmt.Errorf("idea dep add: %w", err)
	}
	return nil
}

// RemoveDep deletes a dependency link.
func (s *Store) RemoveDep(ctx context.Context, ideaID, dependsOnID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM idea_deps WHERE idea_id = ? AND depends_on_id = ?`,
		ideaID, dependsOnID,
	)
	if err != nil {
		return fmt.Errorf("idea dep remove: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("idea dependency link not found: %s -> %s", ideaID, dependsOnID)
	}
	return nil
}

// ListDeps returns dependencies for a given idea.
func (s *Store) ListDeps(ctx context.Context, ideaID string) ([]IdeaDep, error) {
	var deps []IdeaDep
	err := s.db.SelectContext(ctx, &deps,
		`SELECT * FROM idea_deps WHERE idea_id = ? ORDER BY created_at`, ideaID)
	if err != nil {
		return nil, fmt.Errorf("idea dep list: %w", err)
	}
	return deps, nil
}

// ListAllDeps returns all dependency links.
func (s *Store) ListAllDeps(ctx context.Context) ([]IdeaDep, error) {
	var deps []IdeaDep
	err := s.db.SelectContext(ctx, &deps,
		`SELECT * FROM idea_deps ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("idea dep list all: %w", err)
	}
	return deps, nil
}
