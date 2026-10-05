//go:build darwin || linux

package terminalshell

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/state"
)

// checkoutWorkingDirectory is where a managed tab shell opens: the active
// repository override, a worktrees-use binding, or the declared directory.
func checkoutWorkingDirectory(layout state.Layout, generationID string, m *manifest.Manifest, root string, service *manifest.Service) (string, error) {
	execution, err := override.Resolve(context.Background(), layout, generationID, &manifest.Loaded{Manifest: *m, WorkspaceRoot: root}, service, nil)
	if err != nil {
		return "", err
	}
	return execution.WorkingDirectory, nil
}
