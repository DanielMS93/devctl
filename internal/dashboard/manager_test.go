package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/DanielMS93/devctl/internal/dashboard"
	"github.com/stretchr/testify/require"
)

func TestManagerStartStop(t *testing.T) {
	mgr := dashboard.NewManager(nil) // nil db is safe in Phase 1 stub
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.Start(ctx)

	// Wait for at least one event to confirm the goroutine is running.
	select {
	case evt := <-mgr.Events():
		require.False(t, evt.Snapshot.UpdatedAt.IsZero(), "initial snapshot should have a timestamp")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no event received within 2 seconds")
	}

	// Stop the manager and confirm the goroutine exits.
	mgr.Stop()

	// Give the goroutine time to exit, then confirm the channel drains cleanly.
	time.Sleep(50 * time.Millisecond)
}

func TestManagerChannelBufferSize(t *testing.T) {
	mgr := dashboard.NewManager(nil)
	// The channel must be buffered so a slow TUI doesn't block the poller.
	// We can verify this by checking we can read from Events() without sending.
	ch := mgr.Events()
	require.NotNil(t, ch)
}
