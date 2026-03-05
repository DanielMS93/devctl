package dashboard

import (
	"context"
	"time"

	"github.com/danielmiessler/devctl/pkg/tui"
	"github.com/jmoiron/sqlx"
)

// Manager owns the background goroutines and the event channel.
// All goroutines must check ctx.Done() and exit cleanly on cancellation.
type Manager struct {
	db     *sqlx.DB
	events chan tui.StateEvent
	cancel context.CancelFunc
}

// NewManager creates a Manager. Call Start() to begin background polling.
func NewManager(db *sqlx.DB) *Manager {
	return &Manager{
		db:     db,
		events: make(chan tui.StateEvent, 32), // buffered: TUI lags don't block pollers
	}
}

// Start launches background goroutines. ctx is the root application context.
// In Phase 1 this is a stub ticker; real git polling is added in Phase 2.
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	go m.pollLoop(ctx)
}

// Stop cancels all background goroutines. Safe to call multiple times.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Events returns a receive-only view of the state event channel.
// The TUI subscribes to this channel via the recursive subscription pattern.
func (m *Manager) Events() <-chan tui.StateEvent {
	return m.events
}

// pollLoop is the Phase 1 stub: emits an empty snapshot every 5 seconds to
// validate channel wiring. Phase 2 replaces this with real git polling.
func (m *Manager) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Emit an initial snapshot immediately so the TUI renders on startup.
	select {
	case m.events <- tui.StateEvent{Snapshot: tui.StateSnapshot{UpdatedAt: time.Now()}}:
	case <-ctx.Done():
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case m.events <- tui.StateEvent{Snapshot: tui.StateSnapshot{UpdatedAt: time.Now()}}:
			case <-ctx.Done():
				return
			}
		}
	}
}
