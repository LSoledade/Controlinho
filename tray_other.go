//go:build !windows

package main

import "context"

// No system tray off Windows (dev builds). Just block until the context is
// cancelled, then run the graceful shutdown — same lifetime as before.
func runUI(ctx context.Context, _ string, shutdown func()) {
	<-ctx.Done()
	shutdown()
}
