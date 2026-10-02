//go:build darwin || linux

package lifecycle

import (
	"context"
	"time"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
)

// execAckTimeout bounds how long a restarted service may take to reach
// `rungrid internal exec` and acknowledge its working directory.
var execAckTimeout = 15 * time.Second

// verifyExec proves that a restarted service really started in the checkout
// the override change selected. Process Compose service wrappers run the
// Rungrid executable that started the runtime, so a runtime started by a
// version without overrides restarts the service in its old directory and
// writes no acknowledgement; that must be reported, never claimed.
func verifyExec(ctx context.Context, active Active, service *manifest.Service) string {
	loaded := &manifest.Loaded{Manifest: *active.Manifest, WorkspaceRoot: active.Runtime.WorkspaceRoot}
	execution, err := override.Resolve(ctx, active.Layout, active.Runtime.GenerationID, loaded, service, nil)
	if err != nil {
		return "resolve expected checkout: " + err.Error()
	}
	deadline := time.Now().Add(execAckTimeout)
	for {
		if directory, ok := override.ReadAck(active.Layout, active.Runtime.GenerationID, service.Name); ok {
			if directory == execution.WorkingDirectory {
				return ""
			}
			return "service started in " + directory + ", not " + execution.WorkingDirectory
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return "service started but did not confirm its checkout; the runtime's Rungrid executable likely predates overrides. Restart the workspace with this version (rungrid down, then rungrid up)"
		}
		time.Sleep(100 * time.Millisecond)
	}
}
