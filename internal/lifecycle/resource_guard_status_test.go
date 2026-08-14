//go:build darwin || linux

package lifecycle

import (
	"context"
	"testing"

	"github.com/jamesonstone/rungrid/internal/guardstate"
)

func TestInspectStatusInvalidatesPersistedGuardWithoutRuntime(t *testing.T) {
	layout, journal := lifecycleFixture(t, t.TempDir(), nil, nil)
	scope := guardstate.AuthorityScope{
		ProjectID: layout.ProjectID, GenerationID: journal.GenerationID,
		EffectiveManifestSHA256: journal.ManifestSHA256,
	}
	if err := guardstate.WriteStatus(layout, guardstate.Status{
		ProjectID: layout.ProjectID, GenerationID: journal.GenerationID,
		Scope: scope, AuthorityValid: true, Health: "healthy",
	}); err != nil {
		t.Fatal(err)
	}

	status, err := InspectStatus(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.Runtime != "inactive" || status.ResourceGuard == nil ||
		status.ResourceGuard.AuthorityValid || status.ResourceGuard.Health != "inactive" {
		t.Fatalf("inactive runtime retained guard authority: %#v", status.ResourceGuard)
	}
}
