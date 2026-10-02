package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/spf13/cobra"
)

// overrideActive loads the verified runtime that an override mutation targets.
// Overrides are runtime state, so a workspace that is not running is refused
// with the up flag that starts it with the same override.
func overrideActive(ctx context.Context, opt *options, hint string) (lifecycle.Active, *manifest.Loaded, error) {
	active, err := opt.active(ctx)
	if err != nil {
		if errs.Diagnostic(err) == "RG1102" {
			return lifecycle.Active{}, nil, errs.New(errs.ExitConflict, "RG1815", "no active Rungrid runtime; overrides apply to a running workspace"+hint)
		}
		return lifecycle.Active{}, nil, err
	}
	return active, &manifest.Loaded{Manifest: *active.Manifest, WorkspaceRoot: active.Runtime.WorkspaceRoot}, nil
}

func overrideContext(command *cobra.Command) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
}

func runOverrideSet(command *cobra.Command, opt *options, target, selector string) error {
	ctx, cancel := overrideContext(command)
	defer cancel()
	active, loaded, err := overrideActive(ctx, opt, "; start it with rungrid up --override "+target+"="+selector)
	if err != nil {
		return err
	}
	planned, err := override.Plan(ctx, loaded, override.Request{Target: target, Selector: selector, Source: override.SourceCLI}, nil)
	if err != nil {
		return err
	}
	report, err := lifecycle.ApplyOverrides(ctx, active, lifecycle.OverrideChange{Operation: "set", Set: []override.Planned{planned}})
	return errors.Join(err, writeOverrideReport(command, opt, active.Layout.ProjectID, report))
}

func runOverrideClear(command *cobra.Command, opt *options, target string) error {
	ctx, cancel := overrideContext(command)
	defer cancel()
	active, loaded, err := overrideActive(ctx, opt, "")
	if err != nil {
		return err
	}
	change := lifecycle.OverrideChange{Operation: "clear", Exclusive: true}
	if target != "" {
		repository, _, findErr := override.FindTarget(loaded, override.Repositories(ctx, loaded, nil), target)
		if findErr != nil {
			return findErr
		}
		change = lifecycle.OverrideChange{Operation: "clear", Clear: []string{repository.Name}}
	}
	report, err := lifecycle.ApplyOverrides(ctx, active, change)
	return errors.Join(err, writeOverrideReport(command, opt, active.Layout.ProjectID, report))
}

func runOverrideSync(command *cobra.Command, opt *options) error {
	ctx, cancel := overrideContext(command)
	defer cancel()
	declared, err := opt.load()
	if err != nil {
		return err
	}
	active, loaded, err := overrideActive(ctx, opt, "; rungrid up applies worktree declarations when it starts")
	if err != nil {
		return err
	}
	planned, err := override.PlanAll(ctx, loaded, override.Declared(declared), nil)
	if err != nil {
		return err
	}
	report, err := lifecycle.ApplyOverrides(ctx, active, lifecycle.OverrideChange{Operation: "sync", Set: planned, Exclusive: true})
	return errors.Join(err, writeOverrideReport(command, opt, active.Layout.ProjectID, report))
}

func runOverrideList(command *cobra.Command, opt *options) error {
	ctx, cancel := overrideContext(command)
	defer cancel()
	report := override.ListReport{Overrides: []override.Entry{}}
	projectID := opt.projectID
	active, err := opt.active(ctx)
	switch {
	case err == nil:
		projectID = active.Layout.ProjectID
		entries, loadErr := override.Load(active.Layout, active.Runtime.GenerationID)
		if loadErr != nil {
			return loadErr
		}
		report.Generation = active.Runtime.GenerationID
		report.Overrides = override.Refresh(ctx, override.Sorted(entries), nil)
	case errs.Diagnostic(err) != "RG1102":
		return err
	}
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "OverrideList", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return override.WriteListHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func writeOverrideReport(command *cobra.Command, opt *options, projectID string, report override.Report) error {
	if report.Operation == "" {
		return nil
	}
	if report.Services == nil {
		report.Services = []override.ServiceAction{}
	}
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "OverrideReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return override.WriteReportHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}
