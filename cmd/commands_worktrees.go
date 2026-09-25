package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"github.com/spf13/cobra"
)

func newWorktreesListCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List registered worktrees and current service selections",
		Example: "  rungrid worktrees list\n  rungrid worktrees list --json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loaded, layout, err := loadedLayout(opt)
			if err != nil {
				return err
			}
			report, runErr := checkout.List(command.Context(), layout, loaded, nil)
			return errors.Join(runErr, writeListReport(command, opt, loaded.Manifest.Project.ID, report))
		},
	}
}

func newWorktreesUseCommand(opt *options) *cobra.Command {
	var clear bool
	command := &cobra.Command{
		Use:     "use <service> [selector]",
		Short:   "Run one service from an existing registered worktree",
		Example: "  rungrid worktrees use api GH-12\n  rungrid worktrees use api primary\n  rungrid worktrees use api --clear",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(command *cobra.Command, args []string) error {
			loaded, layout, err := loadedLayout(opt)
			if err != nil {
				return err
			}
			serviceName := args[0]
			if clear {
				report, runErr := checkout.Clear(layout, loaded, serviceName)
				return errors.Join(runErr, writeUseReport(command, opt, loaded.Manifest.Project.ID, report))
			}
			selector := ""
			if len(args) == 2 {
				selector = args[1]
			}
			if selector == "" {
				candidates, candidateErr := checkout.Candidates(command.Context(), loaded, serviceName, nil)
				if candidateErr != nil {
					return candidateErr
				}
				selector, err = promptWorktree(command, opt, candidates)
				if err != nil {
					return err
				}
			}
			report, runErr := checkout.Use(command.Context(), layout, loaded, serviceName, selector, nil)
			return errors.Join(runErr, writeUseReport(command, opt, loaded.Manifest.Project.ID, report))
		},
	}
	command.Flags().BoolVar(&clear, "clear", false, "restore the declared repository root")
	return command
}

func newWorktreesUpdateCommand(opt *options) *cobra.Command {
	var services []string
	var syncDefault, dryRun bool
	command := &cobra.Command{
		Use:     "update",
		Short:   "Fast-forward selected feature worktrees",
		Example: "  rungrid worktrees update --dry-run\n  rungrid worktrees update --sync\n  rungrid worktrees update --service api",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx := command.Context()
			stopSignals := func() {}
			if !dryRun {
				ctx, stopSignals = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			}
			defer stopSignals()
			loaded, layout, err := loadedLayout(opt)
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
			coordinator := maintenance.Coordinator(maintenance.NoopCoordinator{})
			if hasActive {
				coordinator = lifecycle.NewMaintenanceCoordinator(active)
			}
			var lock *workspace.Lock
			if !dryRun {
				lock, err = acquireMaintenanceLock(ctx, loaded, opt.stateDir)
				if err != nil {
					return err
				}
			}
			report, runErr := checkout.Update(ctx, layout, loaded, checkout.UpdateOptions{Services: services, Sync: syncDefault, DryRun: dryRun}, nil, coordinator)
			writeErr := writeUpdateReport(command, opt, loaded.Manifest.Project.ID, report)
			var syncErr, syncWrite, releaseErr error
			if syncDefault {
				var syncReport maintenance.SyncReport
				syncReport, syncErr = maintenance.Sync(ctx, loaded, maintenance.Options{DryRun: dryRun}, nil, coordinator)
				syncWrite = writeSyncReport(command, opt, loaded.Manifest.Project.ID, syncReport)
			}
			if lock != nil {
				releaseErr = lock.Release()
			}
			return errors.Join(runErr, writeErr, syncErr, syncWrite, releaseErr)
		},
	}
	command.Flags().StringArrayVar(&services, "service", nil, "select one service (repeatable)")
	command.Flags().BoolVar(&syncDefault, "sync", false, "also fast-forward repository default branches")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "query live state without fetching or changing anything")
	return command
}

func writeListReport(command *cobra.Command, opt *options, projectID string, report checkout.ListReport) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "WorktreeListReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return checkout.WriteListHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func writeUseReport(command *cobra.Command, opt *options, projectID string, report checkout.UseReport) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "WorktreeUseReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return checkout.WriteUseHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func writeUpdateReport(command *cobra.Command, opt *options, projectID string, report checkout.UpdateReport) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "WorktreeUpdateReport", projectID, report, nil)
	}
	if opt.quiet {
		return nil
	}
	return checkout.WriteUpdateHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func promptWorktree(command *cobra.Command, opt *options, candidates []checkout.Worktree) (string, error) {
	if opt.json {
		return "", errs.New(errs.ExitUsage, "RG1717", "worktree selection requires an explicit selector with --json")
	}
	input, ok := command.InOrStdin().(*os.File)
	if !ok {
		return "", errs.New(errs.ExitUsage, "RG1717", "worktree selection requires an interactive terminal or an explicit selector")
	}
	info, err := input.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return "", errs.New(errs.ExitUsage, "RG1717", "worktree selection requires an interactive terminal or an explicit selector")
	}
	if len(candidates) == 0 {
		return "", errs.New(errs.ExitConflict, "RG1713", "no registered worktrees were found")
	}
	for index, candidate := range candidates {
		label := candidate.Branch
		if candidate.Primary {
			label += " (primary)"
		}
		_, _ = fmt.Fprintf(command.OutOrStdout(), "%d) %s  %s\n", index+1, label, candidate.Path)
	}
	_, _ = fmt.Fprintf(command.OutOrStdout(), "Select worktree [1-%d]: ", len(candidates))
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return "", errs.Wrap(errs.ExitInterrupted, "RG1718", "read worktree selection", err)
	}
	choice, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || choice < 1 || choice > len(candidates) {
		return "", errs.New(errs.ExitUsage, "RG1719", "worktree selection is out of range")
	}
	return candidates[choice-1].Path, nil
}

func loadedLayout(opt *options) (*manifest.Loaded, state.Layout, error) {
	loaded, err := opt.load()
	if err != nil {
		return nil, state.Layout{}, err
	}
	layout, err := state.NewLayout(loaded.Manifest.Project.ID, opt.stateDir)
	if err != nil {
		return nil, state.Layout{}, err
	}
	return loaded, layout, nil
}
