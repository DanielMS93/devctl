// Package tuimsg defines the shared message types used by the TUI subsystem.
// It is a leaf package (no imports from pkg/tui or pkg/tui/panels) so that
// both pkg/tui and pkg/tui/panels can import it without creating an import cycle.
package tuimsg

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
