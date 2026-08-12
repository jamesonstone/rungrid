//go:build darwin || linux

package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/session"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func stopRuntime(ctx context.Context, active Active) error {
	if err := supervisor.Verify(ctx, active.Layout, active.Runtime); err != nil {
		return err
	}
	markerRelative := filepath.Join("locks", "down-"+active.Runtime.GenerationID+".json")
	marker := []byte(fmt.Sprintf(
		"{\"api_version\":\"rungrid/output/v1\",\"project_id\":%q,\"generation_id\":%q}\n",
		active.Layout.ProjectID,
		active.Runtime.GenerationID,
	))
	if err := state.WriteFileAtomic(active.Layout.ProjectDir, markerRelative, marker, 0o600); err != nil {
		return err
	}
	// The immutable generation shutdown marker is authoritative. Resource guard
	// status is observational and must not prevent an otherwise safe teardown.
	_ = guardstate.MarkShutdown(active.Layout, active.Runtime.GenerationID)
	markerPath := filepath.Join(active.Layout.ProjectDir, markerRelative)
	defer func() { _ = os.Remove(markerPath) }()
	serviceNames := make([]string, 0, len(active.Manifest.Services))
	for _, service := range active.Manifest.Services {
		if service.Source != "external" {
			serviceNames = append(serviceNames, service.Name)
		}
	}
	quiesceContext, cancelQuiesce := context.WithTimeout(ctx, 3*time.Second)
	_ = session.WaitGenerationReleased(quiesceContext, active.Layout, active.Runtime.GenerationID, serviceNames)
	cancelQuiesce()

	client := supervisor.Client(active.Layout, active.Runtime)
	var failures []string
	for index := len(active.Manifest.Services) - 1; index >= 0; index-- {
		service := &active.Manifest.Services[index]
		if service.Source == "external" {
			continue
		}
		if current, err := client.Get(ctx, service.Name); err == nil && !shouldStop(current.Status) {
			continue
		}
		if err := client.Stop(ctx, service.Name); err != nil && !isAlreadyStopped(err) {
			failures = append(failures, service.Name+": "+err.Error())
		}
	}
	for index := len(active.Manifest.Services) - 1; index >= 0; index-- {
		service := &active.Manifest.Services[index]
		if service.Source == "compose" {
			if err := serviceexec.ComposeShutdown(active.Manifest, service, active.Runtime.WorkspaceRoot, ctx); err != nil {
				failures = append(failures, service.Name+": "+err.Error())
			}
		}
	}
	if err := supervisor.Stop(ctx, active.Layout, active.Runtime); err != nil {
		failures = append(failures, "runtime: "+err.Error())
	}
	if len(failures) > 0 {
		return errs.New(errs.ExitPartial, "RG1113", "partial workspace shutdown:\n  - "+strings.Join(failures, "\n  - "))
	}
	return nil
}
