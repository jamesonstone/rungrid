//go:build darwin || linux

package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/session"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/terminalshell"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

type WorkspaceStatus struct {
	ProjectID           string                    `json:"project_id"`
	Runtime             string                    `json:"runtime"`
	Generation          string                    `json:"generation,omitempty"`
	PID                 int                       `json:"pid,omitempty"`
	Socket              string                    `json:"socket,omitempty"`
	RuntimeVerification string                    `json:"runtime_verification,omitempty"`
	Lifecycle           *WorkspaceLifecycleStatus `json:"lifecycle,omitempty"`
	Services            []ServiceStatus           `json:"services"`
	ResourceGuard       *guardstate.Status        `json:"resource_guard,omitempty"`
}

type WorkspaceLifecycleStatus struct {
	State            string                    `json:"state"`
	Generation       string                    `json:"generation"`
	ManifestSHA256   string                    `json:"manifest_sha256"`
	LifecycleSHA256  string                    `json:"lifecycle_sha256"`
	HashesCompatible bool                      `json:"hashes_compatible"`
	TeardownRequired bool                      `json:"teardown_required"`
	CompletedBefore  []string                  `json:"completed_before_up"`
	CleanupFailure   string                    `json:"cleanup_failure,omitempty"`
	LastFailure      *workspace.CommandOutcome `json:"last_failed_command,omitempty"`
}

func Logs(ctx context.Context, active Active, services []string, follow bool, tail int, raw bool, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(services) == 0 {
		for _, service := range active.Manifest.Services {
			if service.Source != "external" {
				services = append(services, service.Name)
			}
		}
	}
	for _, name := range services {
		service, exists := manifest.FindService(active.Manifest, name)
		if !exists || service.Source == "external" {
			return errs.New(errs.ExitUsage, "RG1114", "logs require a managed service: "+name)
		}
	}
	command := supervisor.Client(active.Layout, active.Runtime).LogsCommand(ctx, services, follow, tail, raw, stdin, stdout, stderr)
	if err := runRegisteredStream(active, command, "logs", strings.Join(services, ",")); err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1115", "read Process Compose logs", err)
	}
	return nil
}

func Attach(ctx context.Context, active Active, readOnly bool, stdin io.Reader, stdout, stderr io.Writer) error {
	command := supervisor.Client(active.Layout, active.Runtime).AttachCommand(ctx, readOnly, stdin, stdout, stderr)
	if err := runRegisteredStream(active, command, "attach", ""); err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1116", "attach Process Compose TUI", err)
	}
	return nil
}

func runRegisteredStream(active Active, command *exec.Cmd, operation, service string) error {
	if err := command.Start(); err != nil {
		return err
	}
	registration, err := guardstate.RegisterControlClient(
		active.Layout,
		supervisor.AuthorityScope(active.Layout, active.Runtime),
		command,
		operation,
		service,
		time.Time{},
	)
	if err != nil {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		return err
	}
	defer func() { _ = registration.Release() }()
	return command.Wait()
}

func Status(ctx context.Context, active Active) ([]ServiceStatus, json.RawMessage, error) {
	states, raw, err := supervisor.Client(active.Layout, active.Runtime).List(ctx)
	if err != nil {
		return nil, nil, err
	}
	byName := make(map[string]processcompose.ProcessState, len(states))
	for _, processState := range states {
		byName[processState.Name] = processState
	}
	result := make([]ServiceStatus, 0, len(active.Manifest.Services))
	for i := range active.Manifest.Services {
		service := &active.Manifest.Services[i]
		item := ServiceStatus{Name: service.Name, Source: service.Source, Activation: service.Activation, Status: "external"}
		if processState, exists := byName[service.Name]; exists {
			item.Status = processState.Status
			item.Health = processState.Health
			item.PID = processState.PID
			item.ExitCode = processState.ExitCode
		}
		_, item.SessionOwned = session.Active(active.Layout, active.Runtime.GenerationID, service.Name)
		_, item.TabRegistered = terminalshell.ActiveTab(active.Layout, active.Runtime.GenerationID, service.Name)
		result = append(result, item)
	}
	return result, raw, nil
}

func InspectStatus(ctx context.Context, layout state.Layout) (WorkspaceStatus, error) {
	result := WorkspaceStatus{ProjectID: layout.ProjectID, Runtime: "inactive", Services: []ServiceStatus{}}
	guardStatus, hasGuardStatus, err := guardstate.ReadStatus(layout)
	if err != nil {
		return result, err
	}
	if hasGuardStatus {
		result.ResourceGuard = &guardStatus
	}
	journal, hasJournal, err := workspace.ReadJournalIfPresent(layout)
	if err != nil {
		return result, err
	}
	if hasJournal {
		if _, loadErr := journalManifest(layout, journal); loadErr != nil {
			return result, loadErr
		}
		result.Generation = journal.GenerationID
		result.Lifecycle = &WorkspaceLifecycleStatus{
			State: journal.State, Generation: journal.GenerationID,
			ManifestSHA256: journal.ManifestSHA256, LifecycleSHA256: journal.LifecycleSHA256,
			HashesCompatible: true,
			TeardownRequired: journal.TeardownRequired,
			CompletedBefore:  append([]string(nil), journal.CompletedBefore...),
			CleanupFailure:   journal.CleanupFailure,
		}
		for index := len(journal.Outcomes) - 1; index >= 0; index-- {
			if journal.Outcomes[index].Status != "succeeded" {
				failure := journal.Outcomes[index]
				result.Lifecycle.LastFailure = &failure
				break
			}
		}
	}
	runtimeState, err := supervisor.Read(layout)
	if errors.Is(err, os.ErrNotExist) {
		if result.ResourceGuard != nil {
			result.ResourceGuard.AuthorityValid = false
			result.ResourceGuard.Health = "inactive"
		}
		if hasJournal {
			configuration, loadErr := journalManifest(layout, journal)
			if loadErr != nil {
				return result, loadErr
			}
			for _, service := range configuration.Services {
				status := "inactive"
				if service.Source == "external" {
					status = "external"
				}
				result.Services = append(result.Services, ServiceStatus{
					Name: service.Name, Source: service.Source, Activation: service.Activation, Status: status,
				})
			}
			mergeGuardServiceStatus(result.Services, result.ResourceGuard)
		}
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := supervisor.Verify(ctx, layout, runtimeState); err != nil {
		result.Runtime = "degraded"
		result.Generation = runtimeState.GenerationID
		result.PID = runtimeState.PID
		result.Socket = runtimeState.Socket
		result.RuntimeVerification = err.Error()
		if result.ResourceGuard != nil {
			result.ResourceGuard.AuthorityValid = false
			result.ResourceGuard.Health = "degraded"
			result.ResourceGuard.DegradedReason = err.Error()
		}
		return result, nil
	}
	configuration, _, err := runtimeManifest(layout, runtimeState)
	if err != nil {
		return result, err
	}
	services, _, err := Status(ctx, Active{Layout: layout, Runtime: runtimeState, Manifest: configuration})
	if err != nil {
		return result, err
	}
	result.Runtime = "active"
	result.Generation = runtimeState.GenerationID
	result.PID = runtimeState.PID
	result.Socket = runtimeState.Socket
	result.Services = services
	mergeGuardServiceStatus(result.Services, result.ResourceGuard)
	return result, nil
}

func mergeGuardServiceStatus(services []ServiceStatus, status *guardstate.Status) {
	if status == nil {
		return
	}
	byName := make(map[string]*guardstate.ServiceStatus, len(status.Services))
	for index := range status.Services {
		byName[status.Services[index].Name] = &status.Services[index]
	}
	for index := range services {
		services[index].ResourceGuard = byName[services[index].Name]
	}
}

func SessionActive(layout state.Layout, generationID, service string) bool {
	_, live := session.Active(layout, generationID, service)
	return live
}
