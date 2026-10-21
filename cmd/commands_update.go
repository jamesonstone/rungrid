package cmd

import (
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/update"
	"github.com/spf13/cobra"
)

const updateLong = `Bring the workspace up to date in place, without down and up.

For each configured repository, update fast-forwards the checked-out default
branch with the sync contract. Dirty, ahead, diverged, and unavailable
checkouts are skipped with a reason. It then runs, only when their inputs
changed since the last successful run:

  - go mod download when go.mod or go.sum changed
  - npm ci, pnpm install --frozen-lockfile, or yarn install when the lockfile
    or package.json changed (or node_modules is missing)
  - each service's declared refresh.steps when their watched paths changed

Finally it restarts only the affected running services through Process
Compose. Hot-reloading development servers (detected, or declared with
refresh.restart: dependencies) keep running unless their dependencies changed.
A second run with nothing new runs nothing and restarts nothing. Input
fingerprints are kept in the Rungrid state directory.`

func newUpdateCommand(opt *options) *cobra.Command {
	var repositories []string
	var dryRun, baseline bool
	command := &cobra.Command{
		Use:     "update",
		Short:   "Fast-forward, install changed dependencies, and restart affected services",
		Long:    updateLong,
		Example: "  rungrid update\n  rungrid update --dry-run\n  rungrid update --repository api --json\n  rungrid update --baseline",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if dryRun && baseline {
				return errs.New(errs.ExitUsage, "RG1904", "--dry-run and --baseline are mutually exclusive")
			}
			ctx := command.Context()
			stopSignals := func() {}
			if !dryRun {
				ctx, stopSignals = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			}
			defer stopSignals()
			loaded, err := opt.load()
			if err != nil {
				return err
			}
			active, hasActive, err := optionalActive(ctx, loaded, opt.stateDir)
			if err != nil {
				return err
			}
			var activeRuntime *lifecycle.Active
			if hasActive {
				loaded, activeRuntime = activeLoaded(active), &active
			}
			layout, err := state.NewLayout(loaded.Manifest.Project.ID, opt.stateDir)
			if err != nil {
				return err
			}
			release := func() error { return nil }
			if !dryRun {
				lock, err := acquireMaintenanceLock(ctx, loaded, opt.stateDir)
				if err != nil {
					return err
				}
				release = lock.Release
			}
			report, runErr := update.Run(ctx, loaded, update.Options{Repositories: repositories, DryRun: dryRun, Baseline: baseline}, update.Dependencies{
				Layout:      layout,
				Coordinator: lifecycle.NewUpdateCoordinator(loaded, layout, activeRuntime),
				Executor:    update.LifecycleExecutor{Layout: layout},
			})
			writeErr := writeWorkspaceUpdateReport(command, opt, loaded.Manifest.Project.ID, report)
			return errors.Join(runErr, writeErr, release())
		},
	}
	command.Flags().StringArrayVar(&repositories, "repository", nil, "select one logical repository (repeatable)")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan without fetching, installing, recording, or restarting")
	command.Flags().BoolVar(&baseline, "baseline", false, "record current inputs as up to date without fetching, running, or restarting")
	return command
}

func writeWorkspaceUpdateReport(command *cobra.Command, opt *options, projectID string, report update.Report) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "WorkspaceUpdateReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return update.WriteHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}
