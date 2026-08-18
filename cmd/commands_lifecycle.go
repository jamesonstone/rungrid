package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/present"
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
			writeCommandResult(command, opt, present.GlyphOK, args[0]+": "+message)
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
			if err := lifecycle.Stop(command.Context(), active, args[0]); err != nil {
				return err
			}
			writeCommandResult(command, opt, present.GlyphOK, "stopped "+args[0])
			return nil
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
			verbose := !opt.json && !opt.quiet
			style := presentStyle(command.OutOrStdout(), opt.noColor)
			// Inspection is best-effort: it only supplies the announcement. A
			// project in a conflicted state must still be able to shut down, so
			// an inspection failure never blocks DownProject.
			status, inspectErr := lifecycle.InspectStatus(ctx, layout)
			if verbose {
				announceDown(command.OutOrStdout(), style, layout.ProjectID, status, inspectErr == nil)
			}
			if err := lifecycle.DownProject(ctx, layout); err != nil {
				return err
			}
			if verbose {
				summarizeDown(command.OutOrStdout(), style, inspectErr != nil || status.Runtime == "active")
			}
			return nil
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
			if err := lifecycle.Uninstall(command.Context(), layout, keepLogs, keepConfig); err != nil {
				return err
			}
			preserved := []string{}
			if keepLogs {
				preserved = append(preserved, "logs")
			}
			if keepConfig {
				preserved = append(preserved, "configuration")
			}
			detail := "removed project-owned state for " + layout.ProjectID
			if len(preserved) > 0 {
				detail += "; preserved " + strings.Join(preserved, " and ")
			}
			writeCommandResult(command, opt, present.EmojiUninstall, detail)
			return nil
		},
	}
	command.Flags().BoolVar(&keepLogs, "keep-logs", false, "preserve project logs")
	command.Flags().BoolVar(&keepConfig, "keep-config", false, "preserve generated configuration")
	return command
}
