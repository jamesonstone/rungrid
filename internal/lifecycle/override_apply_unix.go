//go:build darwin || linux

package lifecycle

import (
	"context"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/warp"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

// OverrideChange describes one override mutation of the active runtime.
// Exclusive makes Set the complete override set, clearing every other
// repository, as `override sync` and `override clear` without arguments need.
type OverrideChange struct {
	Operation string
	Set       []override.Planned
	Clear     []string
	Exclusive bool
}

// serviceController is the slice of Process Compose control that applying an
// override needs; tests substitute it.
type serviceController interface {
	Get(context.Context, string) (processcompose.ProcessState, error)
	Stop(context.Context, string) error
}

// ApplyOverrides records an override change for the live generation and then
// restarts only the affected services that were running. It never runs global
// lifecycle hooks, never touches unaffected services, and never starts a
// service the operator stopped.
func ApplyOverrides(ctx context.Context, active Active, change OverrideChange) (override.Report, error) {
	lock, err := workspace.Acquire(ctx, active.Layout)
	if err != nil {
		return override.Report{}, err
	}
	defer func() { _ = lock.Release() }()
	current, err := supervisor.Read(active.Layout)
	if err != nil || current.GenerationID != active.Runtime.GenerationID || current.PID != active.Runtime.PID {
		return override.Report{}, errs.New(errs.ExitConflict, "RG1824", "the runtime changed while waiting for the lifecycle lock; nothing was changed, rerun the command")
	}
	report, affected, err := recordOverrideChange(active, change)
	if err != nil {
		return report, err
	}
	client := supervisor.Client(active.Layout, active.Runtime)
	start := func(ctx context.Context, name string) error {
		_, startErr := Start(ctx, active, name, false)
		return startErr
	}
	report.Services = restartAffected(ctx, active, client, start, affected)
	if active.Manifest.Terminal.Mode == "warp" {
		reinstallWarpTitles(active)
	}
	var failed []string
	for _, item := range report.Services {
		if item.Action == override.ActionRestartFailed {
			failed = append(failed, item.Name)
		}
	}
	if len(failed) > 0 {
		return report, errs.New(errs.ExitPartial, "RG1818", "override recorded but these services did not restart: "+strings.Join(failed, ", "))
	}
	return report, nil
}

// recordOverrideChange persists the change and returns the names of services
// whose effective checkout changed, keyed to their repository.
func recordOverrideChange(active Active, change OverrideChange) (override.Report, map[string]string, error) {
	report := override.Report{Operation: change.Operation, Set: []override.Entry{}, Cleared: []override.Entry{}, Warnings: []string{}}
	current, err := override.Load(active.Layout, active.Runtime.GenerationID)
	if err != nil {
		return report, nil, err
	}
	next := make(map[string]override.Entry, len(current))
	for name, entry := range current {
		next[name] = entry
	}
	affected := map[string]string{}
	clear := func(repository string) {
		if entry, exists := next[repository]; exists {
			delete(next, repository)
			report.Cleared = append(report.Cleared, entry)
			markAffected(affected, entry)
		}
	}
	keep := map[string]bool{}
	for _, planned := range change.Set {
		keep[planned.Entry.Repository] = true
		report.Warnings = append(report.Warnings, planned.Warnings...)
		if planned.Clear {
			clear(planned.Entry.Repository)
			continue
		}
		entry := planned.Entry
		entry.SetAt = time.Now().UTC().Format(time.RFC3339Nano)
		if previous, exists := next[entry.Repository]; !exists || previous.Path != entry.Path {
			markAffected(affected, entry)
		}
		next[entry.Repository] = entry
		report.Set = append(report.Set, entry)
	}
	for _, repository := range change.Clear {
		if _, exists := next[repository]; !exists {
			report.Warnings = append(report.Warnings, "no override is active for "+repository)
		}
		clear(repository)
	}
	if change.Exclusive {
		for _, entry := range override.Sorted(next) {
			if !keep[entry.Repository] {
				clear(entry.Repository)
			}
		}
	}
	return report, affected, override.Save(active.Layout, active.Runtime.GenerationID, next)
}

func markAffected(affected map[string]string, entry override.Entry) {
	for _, service := range entry.Services {
		affected[service] = entry.Repository
	}
}

// restartAffected visits affected services in manifest order. A running
// workspace service is stopped without recording a stop intent and started
// through the ordinary start path; anything not running is left alone so an
// operator stop intent survives and the next start uses the new checkout.
func restartAffected(
	ctx context.Context,
	active Active,
	client serviceController,
	start func(context.Context, string) error,
	affected map[string]string,
) []override.ServiceAction {
	result := []override.ServiceAction{}
	for index := range active.Manifest.Services {
		service := &active.Manifest.Services[index]
		repository, changed := affected[service.Name]
		if !changed || service.Source == "external" {
			continue
		}
		item := override.ServiceAction{Name: service.Name, Repository: repository, Activation: service.Activation}
		current, err := client.Get(ctx, service.Name)
		running := err == nil && shouldStop(current.Status)
		switch {
		case err != nil:
			item.Action, item.Detail = override.ActionRestartFailed, "inspect service state: "+err.Error()
		case service.Activation == "tab" && running:
			item.Action, item.Detail = override.ActionTabStopped, "rerun "+formatArgv(service.Terminal.TriggerArgv)+" in its tab to start from the new checkout"
			if stopErr := stopForOverride(ctx, active, client, service.Name); stopErr != nil {
				item.Action, item.Detail = override.ActionRestartFailed, stopErr.Error()
			}
		case service.Activation == "tab":
			item.Action, item.Detail = override.ActionTabIdle, "its next session starts from the new checkout"
		case !running && stopIntended(active.Layout, active.Runtime.GenerationID, service.Name):
			item.Action, item.Detail = override.ActionStopped, "stopped with rungrid stop; rungrid start "+service.Name+" runs it from the new checkout"
		case !running:
			item.Action, item.Detail = override.ActionNotRunning, "its next start uses the new checkout"
		default:
			item.Action = override.ActionRestarted
			override.RemoveAck(active.Layout, active.Runtime.GenerationID, service.Name)
			if stopErr := stopForOverride(ctx, active, client, service.Name); stopErr != nil {
				item.Action, item.Detail = override.ActionRestartFailed, stopErr.Error()
			} else if startErr := start(ctx, service.Name); startErr != nil {
				item.Action, item.Detail = override.ActionRestartFailed, startErr.Error()
			} else if problem := verifyExec(ctx, active, service); problem != "" {
				item.Action, item.Detail = override.ActionRestartFailed, problem
			}
		}
		result = append(result, item)
	}
	return result
}

func stopForOverride(ctx context.Context, active Active, client serviceController, name string) error {
	if err := client.Stop(ctx, name); err != nil && !isAlreadyStopped(err) {
		return err
	}
	stopContext, cancel := context.WithTimeout(ctx, active.Manifest.Runtime.ShutdownTimeout.Duration)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := client.Get(stopContext, name)
		status := strings.ToLower(current.Status)
		if err == nil && !shouldStop(status) && !strings.Contains(status, "terminat") && !strings.Contains(status, "restarting") {
			return nil
		}
		select {
		case <-stopContext.Done():
			return errs.Wrap(errs.ExitNotReady, "RG1818", "service did not stop before the override restart: "+name, stopContext.Err())
		case <-ticker.C:
		}
	}
}

// reinstallWarpTitles refreshes owned Tab Configs so a reopened tab shows the
// override. It only rewrites an existing, verified installation.
func reinstallWarpTitles(active Active) {
	if _, err := warp.ReadInstallRecord(active.Layout); err != nil {
		return
	}
	if executable, err := processcompose.ExecutablePath(); err == nil {
		_, _ = warp.Install(active.Layout, active.Manifest, active.Runtime.GenerationID, executable)
	}
}

var _ serviceController = processcompose.Client{}

// seedOverrides runs on every fresh runtime start, before any service starts.
// It replaces leftover overrides with the requested set, or keeps the current
// generation's overrides when resume recovers an interrupted runtime.
func seedOverrides(layout state.Layout, generationID string, options UpOptions) error {
	if options.PreserveOverrides {
		current, err := override.Load(layout, generationID)
		if err != nil {
			return err
		}
		if len(current) > 0 {
			return nil
		}
	}
	if err := override.Remove(layout); err != nil {
		return err
	}
	if len(options.Overrides) == 0 {
		return nil
	}
	entries := make(map[string]override.Entry, len(options.Overrides))
	for _, entry := range options.Overrides {
		entry.SetAt = time.Now().UTC().Format(time.RFC3339Nano)
		entries[entry.Repository] = entry
	}
	return override.Save(layout, generationID, entries)
}
