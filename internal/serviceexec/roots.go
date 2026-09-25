package serviceexec

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

func serviceRoots(ctx context.Context, layout state.Layout, m *manifest.Manifest, root string, service *manifest.Service) (string, string, error) {
	loaded := &manifest.Loaded{Manifest: *m, WorkspaceRoot: root}
	roots, err := checkout.Resolve(ctx, layout, loaded, service, nil)
	if err != nil {
		return "", "", err
	}
	return roots.RepositoryRoot, roots.WorkingDirectory, nil
}
