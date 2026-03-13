// Package tui re-exports the shared message types from pkg/tui/tuimsg.
// Callers outside this subsystem should import pkg/tui directly; the tuimsg
// sub-package exists solely to break the pkg/tui <-> pkg/tui/panels import cycle.
package tui

import (
	"github.com/danielmiessler/devctl/pkg/tui/panels"
	"github.com/danielmiessler/devctl/pkg/tui/tuimsg"
)

// StateSnapshot is the point-in-time snapshot of all tracked state.
// Fields expand in later phases; kept minimal for Phase 1.
type StateSnapshot = tuimsg.StateSnapshot

// StateEvent is the tea.Msg delivered to RootModel.Update() when the
// background state manager emits a new snapshot.
type StateEvent = tuimsg.StateEvent

// PatchStatusUpdater re-exports the interface from panels for external callers.
type PatchStatusUpdater = panels.PatchStatusUpdater

// IdeaCreator re-exports the interface from panels for external callers.
type IdeaCreator = panels.IdeaCreator
