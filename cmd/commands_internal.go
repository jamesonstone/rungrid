package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/resourceguard"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/terminalshell"
	"github.com/spf13/cobra"
)

func newInternalCommand(opt *options) *cobra.Command {
	internal := &cobra.Command{Use: "internal", Hidden: true}
	internal.AddCommand(newExecServiceCommand(opt, false), newExecServiceCommand(opt, true), newServiceShellCommand(opt), newTriggerCommand(opt), newMaintenanceWorkerCommand(opt), newResourceGuardWorkerCommand(opt))
	return internal
}

func newResourceGuardWorkerCommand(opt *options) *cobra.Command {
	var projectID, generation string
	command := &cobra.Command{
		Use: "resource-guard-worker", Hidden: true, Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			runtimeContext, err := serviceexec.LoadContext(projectID, generation, opt.stateDir, "")
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer cancel()
			return resourceguard.Run(ctx, resourceguard.WorkerOptions{RuntimeContext: runtimeContext, Stdout: command.OutOrStdout()})
		},
	}
	command.Flags().StringVar(&projectID, "project-id", "", "project id")
	command.Flags().StringVar(&generation, "generation", "", "generation id")
	_ = command.MarkFlagRequired("project-id")
	_ = command.MarkFlagRequired("generation")
	return command
}

func newMaintenanceWorkerCommand(opt *options) *cobra.Command {
	var projectID, generation, operation string
	command := &cobra.Command{
		Use: "maintenance-worker", Hidden: true, Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if operation != maintenance.OperationSync && operation != maintenance.OperationPrune {
				return errs.New(errs.ExitUsage, "RG1209", "unknown maintenance operation")
			}
			ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer cancel()
			return lifecycle.RunMaintenanceWorker(ctx, projectID, generation, operation, opt.stateDir, command.OutOrStdout())
		},
	}
	command.Flags().StringVar(&projectID, "project-id", "", "project id")
	command.Flags().StringVar(&generation, "generation", "", "generation id")
	command.Flags().StringVar(&operation, "operation", "", "maintenance operation")
	_ = command.MarkFlagRequired("project-id")
	_ = command.MarkFlagRequired("generation")
	_ = command.MarkFlagRequired("operation")
	return command
}

func newExecServiceCommand(opt *options, health bool) *cobra.Command {
	var projectID, generation, service string
	name := "exec-service"
	if health {
		name = "health-service"
	}
	command := &cobra.Command{
		Use: name, Args: cobra.NoArgs, Hidden: true,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, err := serviceexec.LoadContext(projectID, generation, opt.stateDir, "")
			if err != nil {
				return err
			}
			if health {
				return serviceexec.CheckHealth(command.Context(), ctx, service)
			}
			return serviceexec.Exec(command.Context(), ctx, service)
		},
	}
	command.Flags().StringVar(&projectID, "project-id", "", "project id")
	command.Flags().StringVar(&generation, "generation", "", "generation id")
	command.Flags().StringVar(&service, "service", "", "service name")
	_ = command.MarkFlagRequired("project-id")
	_ = command.MarkFlagRequired("generation")
	_ = command.MarkFlagRequired("service")
	return command
}

func newServiceShellCommand(opt *options) *cobra.Command {
	var generation, service string
	command := &cobra.Command{
		Use: "service-shell", Hidden: true, Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			if active.Runtime.GenerationID != generation {
				return errs.New(errs.ExitConflict, "RG1206", "Warp tab generation is stale")
			}
			return terminalshell.RunShell(command.Context(), terminalshell.ShellOptions{Layout: active.Layout, Runtime: active.Runtime, Manifest: active.Manifest, Service: service, Stdin: command.InOrStdin(), Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr()})
		},
	}
	command.Flags().StringVar(&generation, "generation", "", "generation id")
	command.Flags().StringVar(&service, "service", "", "service name")
	_ = command.MarkFlagRequired("generation")
	_ = command.MarkFlagRequired("service")
	return command
}

func newTriggerCommand(opt *options) *cobra.Command {
	var generation, service string
	command := &cobra.Command{
		Use: "trigger", Hidden: true, Args: cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			if active.Runtime.GenerationID != generation {
				return errs.New(errs.ExitConflict, "RG1207", "managed trigger generation is stale")
			}
			ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer cancel()
			return terminalshell.RunTrigger(ctx, active.Layout, active.Runtime, active.Manifest, service, args, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&generation, "generation", "", "generation id")
	command.Flags().StringVar(&service, "service", "", "service name")
	_ = command.MarkFlagRequired("generation")
	_ = command.MarkFlagRequired("service")
	return command
}
