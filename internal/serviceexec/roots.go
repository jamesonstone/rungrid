package serviceexec

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/state"
)

func serviceRoots(ctx context.Context, layout state.Layout, generationID string, m *manifest.Manifest, root string, service *manifest.Service) (override.Execution, error) {
	loaded := &manifest.Loaded{Manifest: *m, WorkspaceRoot: root}
	return override.Resolve(ctx, layout, generationID, loaded, service, nil)
}
