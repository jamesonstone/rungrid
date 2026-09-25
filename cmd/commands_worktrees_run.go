package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"github.com/spf13/cobra"
)

type updatePrompt struct {
	Services   []string
	Sync       bool
	DryRun     bool
	Yes        bool
	ServiceSet bool
	SyncSet    bool
}

func runWorktreesUse(command *cobra.Command, opt *options, serviceName, selector string, clear bool) error {
	if serviceName == "" || (selector == "" && !clear) {
		if err := requireInteractive(command, opt, "worktree selection requires an interactive terminal or an explicit selector"); err != nil {
			return err
		}
	}
	loaded, layout, err := loadedLayout(opt)
	if err != nil {
		return err
	}
	if serviceName == "" {
		serviceName, err = promptServiceName(command, opt, &loaded.Manifest)
		if err != nil {
			return err
		}
	}
	if clear {
		report, runErr := checkout.Clear(layout, loaded, serviceName)
		return errors.Join(runErr, writeUseReport(command, opt, loaded.Manifest.Project.ID, report))
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
}

func runWorktreesUpdate(command *cobra.Command, opt *options, prompt updatePrompt) error {
	loaded, layout, err := loadedLayout(opt)
	if err != nil {
		return err
	}
	if inputIsTTY(command) && !opt.json {
		if !prompt.ServiceSet {
			prompt.Services, err = promptServiceNames(command, opt, &loaded.Manifest)
			if err != nil {
				return err
			}
		}
		if !prompt.SyncSet {
			prompt.Sync, err = promptConfirm(command, opt, "Also fast-forward default branches?", false)
			if err != nil {
				return err
			}
		}
		if !prompt.DryRun {
			if err := executeWorktreeUpdate(command, opt, loaded, layout, updatePrompt{Services: prompt.Services, Sync: prompt.Sync, DryRun: true}); err != nil {
				return err
			}
			if !prompt.Yes {
				apply, confirmErr := promptConfirm(command, opt, "Apply these updates?", false)
				if confirmErr != nil {
					return confirmErr
				}
				if !apply {
					return errs.New(errs.ExitInterrupted, "RG1720", "worktree update cancelled")
				}
			}
		}
	}
	return executeWorktreeUpdate(command, opt, loaded, layout, prompt)
}

func executeWorktreeUpdate(command *cobra.Command, opt *options, loaded *manifest.Loaded, layout state.Layout, prompt updatePrompt) error {
	ctx := command.Context()
	stopSignals := func() {}
	if !prompt.DryRun {
		ctx, stopSignals = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	}
	defer stopSignals()
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
	if !prompt.DryRun {
		lock, err = acquireMaintenanceLock(ctx, loaded, opt.stateDir)
		if err != nil {
			return err
		}
	}
	return finishWorktreeUpdate(ctx, command, opt, loaded, layout, prompt, coordinator, lock)
}

func finishWorktreeUpdate(
	ctx context.Context,
	command *cobra.Command,
	opt *options,
	loaded *manifest.Loaded,
	layout state.Layout,
	prompt updatePrompt,
	coordinator maintenance.Coordinator,
	lock *workspace.Lock,
) error {
	report, runErr := checkout.Update(ctx, layout, loaded, checkout.UpdateOptions{Services: prompt.Services, Sync: prompt.Sync, DryRun: prompt.DryRun}, nil, coordinator)
	writeErr := writeUpdateReport(command, opt, loaded.Manifest.Project.ID, report)
	var syncErr, syncWrite, releaseErr error
	if prompt.Sync {
		var syncReport maintenance.SyncReport
		syncReport, syncErr = maintenance.Sync(ctx, loaded, maintenance.Options{DryRun: prompt.DryRun}, nil, coordinator)
		syncWrite = writeSyncReport(command, opt, loaded.Manifest.Project.ID, syncReport)
	}
	if lock != nil {
		releaseErr = lock.Release()
	}
	return errors.Join(runErr, writeErr, syncErr, syncWrite, releaseErr)
}
