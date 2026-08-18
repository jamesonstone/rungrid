package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/doctor"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/generate"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/planner"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/spf13/cobra"
)

func newDoctorCommand(opt *options) *cobra.Command {
	var fix bool
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Validate configuration and local dependencies",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loaded, err := opt.load()
			if err != nil {
				return err
			}
			report := doctor.Run(command.Context(), loaded, opt.stateDir, fix)
			if opt.json {
				if err := output.WriteJSON(command.OutOrStdout(), "DoctorReport", loaded.Manifest.Project.ID, report, nil); err != nil {
					return err
				}
			} else if !opt.quiet {
				style := presentStyle(command.OutOrStdout(), opt.noColor)
				if err := doctor.WriteHuman(command.OutOrStdout(), style, report); err != nil {
					return err
				}
			}
			if !report.OK {
				return errs.New(errs.ExitDependency, "RG1201", "doctor found blocking problems")
			}
			return nil
		},
	}
	command.Flags().BoolVar(&fix, "fix", false, "repair safe project-owned state")
	return command
}

func newPlanCommand(opt *options) *cobra.Command {
	var format string
	command := &cobra.Command{
		Use:   "plan",
		Short: "Show deterministic generation and lifecycle actions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loaded, err := opt.load()
			if err != nil {
				return err
			}
			plan := planner.Build(loaded, Version)
			layout, err := state.NewLayout(loaded.Manifest.Project.ID, opt.stateDir)
			if err != nil {
				return err
			}
			plan.Recovery, err = planner.InspectRecovery(layout, plan)
			if err != nil {
				return err
			}
			if opt.json || format == "json" {
				return output.WriteJSON(command.OutOrStdout(), "Plan", loaded.Manifest.Project.ID, plan, nil)
			}
			if format != "human" {
				return errs.New(errs.ExitUsage, "RG1202", "plan output must be human or json")
			}
			if !opt.quiet {
				plan.WriteHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor))
			}
			return nil
		},
	}
	command.Flags().StringVar(&format, "output", "human", "output format: human or json")
	return command
}

func newGenerateCommand(opt *options) *cobra.Command {
	var check bool
	command := &cobra.Command{
		Use:   "generate",
		Short: "Generate owned project runtime artifacts",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loaded, err := opt.load()
			if err != nil {
				return err
			}
			result, err := generate.Run(loaded, opt.stateDir, Version, check)
			if err != nil {
				return err
			}
			if opt.json {
				return output.WriteJSON(command.OutOrStdout(), "Generation", loaded.Manifest.Project.ID, result, nil)
			}
			if !opt.quiet {
				verb := "reused"
				if result.Created {
					verb = "created"
				}
				style := presentStyle(command.OutOrStdout(), opt.noColor)
				detail := fmt.Sprintf(
					"%s generation %s  %s %s",
					verb, result.Plan.GenerationID, style.Muted("at"), result.Directory,
				)
				_ = style.Result(command.OutOrStdout(), present.EmojiGenerate, detail)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&check, "check", false, "verify generation without writing")
	return command
}

func newUpCommand(opt *options) *cobra.Command {
	var headless, noOpen, doSync bool
	command := &cobra.Command{
		Use:   "up [service ...]",
		Short: "Generate and start the detached workspace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			loaded, err := opt.load()
			if err != nil {
				return err
			}

			// If requested, fast-forward configured repositories before starting
			if doSync {
				// Prepare a cancellable context for sync (respect signals)
				syncCtx, stopSignals := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
				defer stopSignals()
				active, hasActive, err := optionalActive(syncCtx, loaded, opt.stateDir)
				if err != nil {
					return err
				}
				if hasActive {
					result, runErr := lifecycle.StartMaintenanceJob(syncCtx, active, maintenance.OperationSync, nil)
					if len(result.Data) == 0 {
						return runErr
					}
					report, decodeErr := lifecycle.DecodeSyncJob(result)
					if writeErr := writeSyncReport(command, opt, loaded.Manifest.Project.ID, report); writeErr != nil {
						return errors.Join(runErr, decodeErr, writeErr)
					}
				} else {
					lock, err := acquireMaintenanceLock(syncCtx, loaded, opt.stateDir)
					if err != nil {
						return err
					}
					coordinator := maintenance.Coordinator(maintenance.NoopCoordinator{})
					if hasActive {
						coordinator = lifecycle.NewMaintenanceCoordinator(active)
					}
					report, runErr := maintenance.Sync(syncCtx, loaded, maintenance.Options{Repositories: nil, DryRun: false}, nil, coordinator)
					writeErr := writeSyncReport(command, opt, loaded.Manifest.Project.ID, report)
					var releaseErr error
					if lock != nil {
						releaseErr = lock.Release()
					}
					if err := errors.Join(runErr, writeErr, releaseErr); err != nil {
						return err
					}
				}
			}

			open := loaded.Manifest.Terminal.Open != nil && *loaded.Manifest.Terminal.Open && !noOpen
			ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer cancel()
			verbose := !opt.json && !opt.quiet
			style := presentStyle(command.OutOrStdout(), opt.noColor)
			if verbose {
				announceUp(command.OutOrStdout(), style, &loaded.Manifest)
			}
			result, err := lifecycle.Up(ctx, loaded, lifecycle.UpOptions{
				StateOverride: opt.stateDir, GeneratorVersion: Version, Headless: headless, Open: open, Requested: args,
			})
			if err != nil {
				return err
			}
			if opt.json {
				return output.WriteJSON(command.OutOrStdout(), "Up", loaded.Manifest.Project.ID, result, nil)
			}
			if verbose {
				summarizeUp(command.OutOrStdout(), style, result)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&headless, "headless", false, "do not create or open terminal files")
	command.Flags().BoolVar(&noOpen, "no-open", false, "do not open Warp")
	command.Flags().BoolVar(&doSync, "sync", false, "fast-forward configured repositories' default branches before starting")
	return command
}
