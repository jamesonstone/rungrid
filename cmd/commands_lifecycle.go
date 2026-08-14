package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/session"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/spf13/cobra"
)

func newSessionCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:   "session <service>",
		Short: "Own a tab service until stop or signal",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer cancel()
			return session.Run(ctx, session.Options{Layout: active.Layout, Runtime: active.Runtime, Manifest: active.Manifest, Service: args[0], TabID: os.Getenv("WARP_PANE_ID"), Stdin: command.InOrStdin(), Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr()})
		},
	}
}

func newStartCommand(opt *options) *cobra.Command {
	var resetResourceCircuit bool
	command := &cobra.Command{
		Use: "start <service>", Short: "Start a service using activation-aware behavior", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			message, err := lifecycle.Start(command.Context(), active, args[0], resetResourceCircuit)
			if err != nil {
				return err
			}
			if !opt.quiet {
				_, _ = fmt.Fprintln(command.OutOrStdout(), message)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&resetResourceCircuit, "reset-resource-circuit", false, "close the verified service resource circuit before starting")
	return command
}

func newStopCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use: "stop <service>", Short: "Stop an exact managed service", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			return lifecycle.Stop(command.Context(), active, args[0])
		},
	}
}

func newDownCommand(opt *options) *cobra.Command {
	var timeout time.Duration
	command := &cobra.Command{
		Use: "down", Short: "Perform ordered workspace shutdown", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			projectID := opt.projectID
			if projectID == "" {
				loaded, err := opt.load()
				if err != nil {
					return err
				}
				projectID = loaded.Manifest.Project.ID
			}
			layout, err := state.NewLayout(projectID, opt.stateDir)
			if err != nil {
				return err
			}
			ctx, stopSignals := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer stopSignals()
			cancel := func() {}
			if timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, timeout)
			}
			defer cancel()
			return lifecycle.DownProject(ctx, layout)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 0, "overall shutdown timeout")
	return command
}

func newUninstallCommand(opt *options) *cobra.Command {
	var keepLogs, keepConfig bool
	command := &cobra.Command{
		Use: "uninstall", Short: "Remove only owned project state and Warp files", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			projectID := opt.projectID
			if projectID == "" {
				loaded, err := opt.load()
				if err != nil {
					return err
				}
				projectID = loaded.Manifest.Project.ID
			}
			layout, err := state.NewLayout(projectID, opt.stateDir)
			if err != nil {
				return err
			}
			return lifecycle.Uninstall(command.Context(), layout, keepLogs, keepConfig)
		},
	}
	command.Flags().BoolVar(&keepLogs, "keep-logs", false, "preserve project logs")
	command.Flags().BoolVar(&keepConfig, "keep-config", false, "preserve generated configuration")
	return command
}
