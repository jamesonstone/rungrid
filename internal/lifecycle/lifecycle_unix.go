//go:build darwin || linux

package lifecycle

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type Active struct {
	Layout   state.Layout
	Runtime  supervisor.Runtime
	Manifest *manifest.Manifest
}

type UpOptions struct {
	StateOverride    string
	GeneratorVersion string
	Headless         bool
	Open             bool
	Requested        []string
}

type UpResult struct {
	Generation string `json:"generation"`
	RuntimePID int    `json:"runtime_pid"`
	Socket     string `json:"socket"`
	Reused     bool   `json:"reused"`
	OpenedWarp bool   `json:"opened_warp"`
}

type ServiceStatus struct {
	Name          string                    `json:"name"`
	Source        string                    `json:"source"`
	Activation    string                    `json:"activation"`
	Status        string                    `json:"status"`
	Health        string                    `json:"health,omitempty"`
	PID           int                       `json:"pid,omitempty"`
	ExitCode      int                       `json:"exit_code,omitempty"`
	SessionOwned  bool                      `json:"session_owned"`
	TabRegistered bool                      `json:"tab_registered"`
	ResourceGuard *guardstate.ServiceStatus `json:"resource_guard,omitempty"`
}

func LoadActive(ctx context.Context, projectID, stateOverride string) (Active, error) {
	if projectID == "" {
		return Active{}, errs.New(errs.ExitUsage, "RG1101", "project id is required to select active state")
	}
	layout, err := state.NewLayout(projectID, stateOverride)
	if err != nil {
		return Active{}, err
	}
	runtimeState, err := supervisor.Read(layout)
	if err != nil {
		if os.IsNotExist(err) {
			return Active{}, errs.New(errs.ExitConflict, "RG1102", "no active Rungrid runtime")
		}
		return Active{}, err
	}
	if err := supervisor.Verify(ctx, layout, runtimeState); err != nil {
		return Active{}, err
	}
	manifestPath := filepath.Join(layout.ProjectDir, "generations", runtimeState.GenerationID, "manifest.yaml")
	generatedManifest, err := manifest.LoadGenerated(manifestPath, runtimeState.WorkspaceRoot)
	if err != nil {
		return Active{}, err
	}
	return Active{Layout: layout, Runtime: runtimeState, Manifest: generatedManifest}, nil
}

func Up(ctx context.Context, loaded *manifest.Loaded, options UpOptions) (UpResult, error) {
	return upWorkspace(ctx, loaded, options)
}
