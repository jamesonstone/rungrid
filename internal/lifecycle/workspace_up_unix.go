//go:build darwin || linux

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/generate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/planner"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/warp"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"gopkg.in/yaml.v3"
)

func upWorkspace(ctx context.Context, loaded *manifest.Loaded, options UpOptions) (UpResult, error) {
	effective, err := effectiveUpManifest(loaded, options.Headless)
	if err != nil {
		return UpResult{}, err
	}
	if err := validateRequested(&effective.Manifest, options.Requested, options); err != nil {
		return UpResult{}, err
	}
	planned := planner.Build(effective, options.GeneratorVersion)
	layout, err := state.NewLayout(effective.Manifest.Project.ID, options.StateOverride)
	if err != nil {
		return UpResult{}, err
	}
	lock, err := workspace.Acquire(ctx, layout)
	if err != nil {
		return UpResult{}, err
	}
	defer func() { _ = lock.Release() }()

	if runtimeState, reused, err := reconcileBeforeUp(ctx, layout, effective, planned); err != nil {
		return UpResult{}, err
	} else if reused {
		return finishUp(ctx, layout, effective, runtimeState, options, true)
	}

	generated, err := generate.Run(effective, options.StateOverride, options.GeneratorVersion, false)
	if err != nil {
		return UpResult{}, err
	}
	pcExecutable, pcVersion, rungridExecutable, err := upExecutables(ctx, &effective.Manifest)
	if err != nil {
		return UpResult{}, err
	}
	teardown := len(effective.Manifest.Lifecycle.BeforeUp) > 0 || len(effective.Manifest.Lifecycle.AfterDown) > 0
	journal := workspace.NewJournal(
		layout.ProjectID,
		generated.Plan.GenerationID,
		generated.Plan.ManifestSHA256,
		generated.Plan.LifecycleSHA256,
		effective.WorkspaceRoot,
		teardown,
	)
	if err := workspace.WriteJournal(layout, journal); err != nil {
		return UpResult{}, err
	}
	if err := runBeforeUp(ctx, layout, &journal, &effective.Manifest); err != nil {
		return UpResult{}, rollbackUp(layout, &journal, &effective.Manifest, err)
	}
	if err := waitExternalServices(ctx, effective); err != nil {
		return UpResult{}, rollbackUp(layout, &journal, &effective.Manifest, err)
	}

	runtimeState, reused, err := supervisor.Start(ctx, supervisor.StartOptions{
		Layout: layout, GenerationID: generated.Plan.GenerationID, WorkspaceRoot: effective.WorkspaceRoot,
		ProcessCompose: pcExecutable, ProcessComposeVersion: pcVersion, RungridExecutable: rungridExecutable,
		StartupTimeout: effective.Manifest.Runtime.StartupTimeout.Duration,
	})
	if err != nil {
		return UpResult{}, rollbackUp(layout, &journal, &effective.Manifest, err)
	}
	attachRuntime(&journal, runtimeState)
	if err := workspace.WriteJournal(layout, journal); err != nil {
		return UpResult{}, rollbackUp(layout, &journal, &effective.Manifest, err)
	}
	result, err := finishUp(ctx, layout, effective, runtimeState, options, reused)
	if err != nil {
		return UpResult{}, rollbackUp(layout, &journal, &effective.Manifest, err)
	}
	journal.State = workspace.StateActive
	journal.CleanupFailure = ""
	if err := workspace.WriteJournal(layout, journal); err != nil {
		return UpResult{}, rollbackUp(layout, &journal, &effective.Manifest, err)
	}
	return result, nil
}

func reconcileBeforeUp(
	ctx context.Context,
	layout state.Layout,
	loaded *manifest.Loaded,
	planned planner.Plan,
) (supervisor.Runtime, bool, error) {
	journal, exists, err := workspace.ReadJournalIfPresent(layout)
	if err != nil {
		return supervisor.Runtime{}, false, err
	}
	if exists && journal.State == workspace.StateActive && journal.GenerationID == planned.GenerationID {
		if journal.ManifestSHA256 != planned.ManifestSHA256 || journal.LifecycleSHA256 != planned.LifecycleSHA256 {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1140", "active lifecycle journal hashes do not match the requested generation")
		}
		if filepath.Clean(journal.WorkspaceRoot) != filepath.Clean(loaded.WorkspaceRoot) {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1145", "active lifecycle generation belongs to a different resolved workspace root")
		}
		runtimeState, active, verifyErr := verifiedJournalRuntime(ctx, layout, &journal)
		if verifyErr == nil && active {
			return runtimeState, true, nil
		}
		if verifyErr != nil {
			return supervisor.Runtime{}, false, verifyErr
		}
	}
	if exists && (journal.TeardownRequired || journal.State != workspace.StateInactive) {
		cleanupContext, cancel := durableCleanupContext(ctx)
		err := cleanupJournalLocked(cleanupContext, layout, &journal)
		cancel()
		if err != nil {
			return supervisor.Runtime{}, false, err
		}
	}
	if runtimeState, readErr := supervisor.Read(layout); readErr == nil {
		if err := supervisor.Verify(ctx, layout, runtimeState); err != nil {
			return supervisor.Runtime{}, false, err
		}
		if exists {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1141", "verified runtime exists while lifecycle journal is inactive")
		}
		if runtimeState.GenerationID != planned.GenerationID {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1142", "a different verified runtime generation is active")
		}
		if len(loaded.Manifest.Lifecycle.BeforeUp) > 0 || len(loaded.Manifest.Lifecycle.AfterDown) > 0 {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1143", "verified runtime has no lifecycle journal; run rungrid down before enabling lifecycle hooks")
		}
		journal, _, err := journalFromRuntime(layout)
		if err != nil {
			return supervisor.Runtime{}, false, err
		}
		if err := workspace.WriteJournal(layout, journal); err != nil {
			return supervisor.Runtime{}, false, err
		}
		return runtimeState, true, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return supervisor.Runtime{}, false, readErr
	}
	return supervisor.Runtime{}, false, nil
}

func finishUp(
	ctx context.Context,
	layout state.Layout,
	loaded *manifest.Loaded,
	runtimeState supervisor.Runtime,
	options UpOptions,
	reused bool,
) (UpResult, error) {
	client := supervisor.Client(layout, runtimeState)
	readyContext, cancel := context.WithTimeout(ctx, loaded.Manifest.Runtime.StartupTimeout.Duration)
	_, readyErr := client.Run(readyContext, "project", "is-ready", "--wait")
	cancel()
	if readyErr != nil {
		return UpResult{}, errs.Wrap(errs.ExitNotReady, "RG1105", "workspace-owned services did not become ready", readyErr)
	}
	rungridExecutable, err := processcompose.ExecutablePath()
	if err != nil {
		return UpResult{}, err
	}
	opened := false
	shouldOpen := options.Open && !options.Headless && loaded.Manifest.Terminal.Mode == "warp"
	if shouldOpen {
		record, installErr := warp.Install(layout, &loaded.Manifest, runtimeState.GenerationID, rungridExecutable)
		if installErr != nil {
			return UpResult{}, installErr
		}
		if openErr := warp.Open(ctx, record, ""); openErr != nil {
			return UpResult{}, openErr
		}
		opened = true
	}
	for _, name := range options.Requested {
		service, _ := manifest.FindService(&loaded.Manifest, name)
		waitContext, waitCancel := context.WithTimeout(ctx, loaded.Manifest.Runtime.StartupTimeout.Duration)
		err := waitForService(waitContext, client, layout, runtimeState.GenerationID, service)
		waitCancel()
		if err != nil {
			return UpResult{}, err
		}
	}
	return UpResult{
		Generation: runtimeState.GenerationID, RuntimePID: runtimeState.PID,
		Socket: runtimeState.Socket, Reused: reused, OpenedWarp: opened,
	}, nil
}

func upExecutables(ctx context.Context, configuration *manifest.Manifest) (string, string, string, error) {
	pcExecutable, err := exec.LookPath(configuration.Runtime.ProcessCompose.Executable)
	if err != nil {
		return "", "", "", errs.Wrap(errs.ExitDependency, "RG1103", "resolve Process Compose executable", err)
	}
	pcVersion, err := processcompose.Version(ctx, pcExecutable)
	if err != nil {
		return "", "", "", err
	}
	rungridExecutable, err := processcompose.ExecutablePath()
	if err != nil {
		return "", "", "", errs.Wrap(errs.ExitFailure, "RG1104", "resolve Rungrid executable", err)
	}
	return pcExecutable, pcVersion, rungridExecutable, nil
}

func waitExternalServices(ctx context.Context, loaded *manifest.Loaded) error {
	for index := range loaded.Manifest.Services {
		service := &loaded.Manifest.Services[index]
		if service.Source != "external" {
			continue
		}
		waitContext, cancel := context.WithTimeout(ctx, loaded.Manifest.Runtime.StartupTimeout.Duration)
		err := serviceexec.WaitExternal(waitContext, &loaded.Manifest, loaded.WorkspaceRoot, service)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func rollbackUp(layout state.Layout, journal *workspace.Journal, configuration *manifest.Manifest, primary error) error {
	budget := configuration.Runtime.ShutdownTimeout.Duration + 10*time.Second
	for _, command := range configuration.Lifecycle.AfterDown {
		budget += command.Timeout.Duration
	}
	cleanupContext, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	if cleanupErr := cleanupJournalLocked(cleanupContext, layout, journal); cleanupErr != nil {
		return errs.New(errs.ExitPartial, "RG1144", fmt.Sprintf("workspace startup failed: %v; rollback failed: %v", primary, cleanupErr))
	}
	return primary
}

func effectiveUpManifest(loaded *manifest.Loaded, headless bool) (*manifest.Loaded, error) {
	if !headless || loaded.Manifest.Terminal.Mode == "headless" {
		return loaded, nil
	}
	copyLoaded := *loaded
	copyLoaded.Manifest = loaded.Manifest
	copyLoaded.Manifest.Terminal.Mode = "headless"
	copyLoaded.Manifest.Terminal.Open = manifest.Bool(false)
	content, err := yaml.Marshal(copyLoaded.Manifest)
	if err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG1126", "encode headless manifest override", err)
	}
	copyLoaded.MergedYAML = content
	return &copyLoaded, nil
}

func validateRequested(configuration *manifest.Manifest, requested []string, options UpOptions) error {
	for _, name := range requested {
		service, exists := manifest.FindService(configuration, name)
		if !exists {
			return errs.New(errs.ExitUsage, "RG1106", "unknown requested service: "+name)
		}
		shouldOpen := options.Open && !options.Headless && configuration.Terminal.Mode == "warp"
		if service.Activation == "tab" && !shouldOpen {
			return errs.New(errs.ExitUsage, "RG1107", "requested tab service requires Warp opening or a separate rungrid session")
		}
	}
	return nil
}
