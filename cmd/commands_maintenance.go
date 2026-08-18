package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"github.com/spf13/cobra"
)

func newSyncCommand(opt *options) *cobra.Command {
	var repositories []string
	var dryRun bool
	command := &cobra.Command{
		Use:     "sync",
		Short:   "Fast-forward configured repositories' default branches",
		Example: "  rungrid sync\n  rungrid sync --dry-run\n  rungrid sync --repository api --json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
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
			if hasActive {
				loaded = activeLoaded(active)
			}
			if hasActive && !dryRun {
				result, runErr := lifecycle.StartMaintenanceJob(ctx, active, maintenance.OperationSync, repositories)
				if len(result.Data) == 0 {
					return runErr
				}
				report, decodeErr := lifecycle.DecodeSyncJob(result)
				writeErr := writeSyncReport(command, opt, loaded.Manifest.Project.ID, report)
				return errors.Join(runErr, decodeErr, writeErr)
			}
			var lock *workspace.Lock
			if !dryRun {
				lock, err = acquireMaintenanceLock(ctx, loaded, opt.stateDir)
				if err != nil {
					return err
				}
			}
			coordinator := maintenance.Coordinator(maintenance.NoopCoordinator{})
			if hasActive {
				coordinator = lifecycle.NewMaintenanceCoordinator(active)
			}
			report, runErr := maintenance.Sync(ctx, loaded, maintenance.Options{Repositories: repositories, DryRun: dryRun}, nil, coordinator)
			writeErr := writeSyncReport(command, opt, loaded.Manifest.Project.ID, report)
			var releaseErr error
			if lock != nil {
				releaseErr = lock.Release()
			}
			return errors.Join(runErr, writeErr, releaseErr)
		},
	}
	command.Flags().StringArrayVar(&repositories, "repository", nil, "select one logical repository (repeatable)")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "query live state without fetching or changing anything")
	return command
}

func newWorktreesCommand(opt *options) *cobra.Command {
	command := &cobra.Command{Use: "worktrees", Short: "Inspect and safely maintain linked Git worktrees"}
	command.AddCommand(newWorktreesPruneCommand(opt))
	return command
}

func newWorktreesPruneCommand(opt *options) *cobra.Command {
	var repositories []string
	var dryRun, yes bool
	command := &cobra.Command{
		Use:     "prune",
		Short:   "Remove clean merged worktrees whose remote branches are deleted",
		Example: "  rungrid worktrees prune --dry-run\n  rungrid worktrees prune\n  rungrid worktrees prune --yes --json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loaded, err := opt.load()
			if err != nil {
				return err
			}
			active, hasActive, err := optionalActive(command.Context(), loaded, opt.stateDir)
			if err != nil {
				return err
			}
			if hasActive {
				loaded = activeLoaded(active)
			}
			selection := maintenance.Options{Repositories: repositories, DryRun: true}
			preview, planErr := maintenance.Prune(command.Context(), loaded, selection, nil)
			if dryRun || maintenance.RemovalCount(preview) == 0 {
				return errors.Join(planErr, writePruneReport(command, opt, loaded.Manifest.Project.ID, preview))
			}
			if !opt.json || !yes {
				if err := writePruneReport(command, opt, loaded.Manifest.Project.ID, preview); err != nil {
					return err
				}
			}
			if err := confirmPrune(command, maintenance.RemovalCount(preview), yes, opt.json); err != nil {
				return err
			}
			applyContext, stopSignals := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer stopSignals()
			if hasActive {
				result, runErr := lifecycle.StartMaintenanceJob(applyContext, active, maintenance.OperationPrune, repositories)
				if len(result.Data) == 0 {
					return runErr
				}
				report, decodeErr := lifecycle.DecodePruneJob(result)
				writeErr := writePruneReport(command, opt, loaded.Manifest.Project.ID, report)
				return errors.Join(runErr, decodeErr, writeErr)
			}
			lock, err := acquireMaintenanceLock(applyContext, loaded, opt.stateDir)
			if err != nil {
				return err
			}
			report, runErr := maintenance.Prune(applyContext, loaded, maintenance.Options{Repositories: repositories}, nil)
			writeErr := writePruneReport(command, opt, loaded.Manifest.Project.ID, report)
			return errors.Join(runErr, writeErr, lock.Release())
		},
	}
	command.Flags().StringArrayVar(&repositories, "repository", nil, "select one logical repository (repeatable)")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "query live state without fetching or changing anything")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "confirm every currently eligible removal")
	return command
}

func writeSyncReport(command *cobra.Command, opt *options, projectID string, report maintenance.SyncReport) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "RepositorySyncReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return maintenance.WriteSyncHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func writePruneReport(command *cobra.Command, opt *options, projectID string, report maintenance.PruneReport) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "WorktreePruneReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return maintenance.WritePruneHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func confirmPrune(command *cobra.Command, count int, yes, jsonOutput bool) error {
	if yes {
		return nil
	}
	input, ok := command.InOrStdin().(*os.File)
	if jsonOutput || !ok {
		return errs.New(errs.ExitUsage, "RG1603", "worktree removal requires an interactive confirmation or --yes")
	}
	info, err := input.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return errs.New(errs.ExitUsage, "RG1603", "worktree removal requires an interactive confirmation or --yes")
	}
	_, _ = fmt.Fprintf(command.OutOrStdout(), "Remove %d worktree(s)? [y/N] ", count)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return errs.Wrap(errs.ExitInterrupted, "RG1604", "read worktree removal confirmation", err)
	}
	if normalized := strings.ToLower(strings.TrimSpace(answer)); normalized != "y" && normalized != "yes" {
		return errs.New(errs.ExitInterrupted, "RG1605", "worktree removal cancelled")
	}
	return nil
}

func acquireMaintenanceLock(ctx context.Context, loaded *manifest.Loaded, stateOverride string) (*workspace.Lock, error) {
	layout, err := state.NewLayout(loaded.Manifest.Project.ID, stateOverride)
	if err != nil {
		return nil, err
	}
	return workspace.Acquire(ctx, layout)
}

func optionalActive(ctx context.Context, loaded *manifest.Loaded, stateOverride string) (lifecycle.Active, bool, error) {
	layout, err := state.NewLayout(loaded.Manifest.Project.ID, stateOverride)
	if err != nil {
		return lifecycle.Active{}, false, err
	}
	if _, err := supervisor.Read(layout); os.IsNotExist(err) {
		return lifecycle.Active{}, false, nil
	} else if err != nil {
		return lifecycle.Active{}, false, err
	}
	active, err := lifecycle.LoadActive(ctx, loaded.Manifest.Project.ID, stateOverride)
	if err != nil {
		return lifecycle.Active{}, false, err
	}
	return active, true, nil
}

func activeLoaded(active lifecycle.Active) *manifest.Loaded {
	return &manifest.Loaded{Manifest: *active.Manifest, WorkspaceRoot: active.Runtime.WorkspaceRoot}
}
