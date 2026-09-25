//go:build darwin || linux

package terminalshell

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

func checkoutWorkingDirectory(layout state.Layout, m *manifest.Manifest, root string, service *manifest.Service) (string, error) {
	roots, err := checkout.Resolve(context.Background(), layout, &manifest.Loaded{Manifest: *m, WorkspaceRoot: root}, service, nil)
	if err != nil {
		return "", err
	}
	return roots.WorkingDirectory, nil
}
