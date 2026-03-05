package tui

import "time"

// StateSnapshot is the point-in-time snapshot of all tracked state.
// Fields expand in later phases; kept minimal for Phase 1.
type StateSnapshot struct {
	UpdatedAt time.Time
}

// StateEvent is the tea.Msg delivered to RootModel.Update() when the
// background state manager emits a new snapshot.
type StateEvent struct {
	Snapshot StateSnapshot
}
