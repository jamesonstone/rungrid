package cmd

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/versions"
	"github.com/spf13/cobra"
)

func newOpenCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:   "open [service]",
		Short: "Open the Warp workspace or one service tab",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			service := ""
			if len(args) == 1 {
				service = args[0]
			}
			return lifecycle.Open(command.Context(), active, service)
		},
	}
}

func newAttachCommand(opt *options) *cobra.Command {
	var readOnly bool
	command := &cobra.Command{
		Use:   "attach",
		Short: "Attach to the active Process Compose TUI",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			return lifecycle.Attach(command.Context(), active, readOnly, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	command.Flags().BoolVar(&readOnly, "read-only", true, "disable lifecycle mutations in the TUI")
	return command
}

func newVersionsCommand(opt *options) *cobra.Command {
	var watch, once bool
	command := &cobra.Command{
		Use:   "versions",
		Short: "Show process, listener, and source-control state",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if watch && once {
				return errs.New(errs.ExitUsage, "RG1203", "--watch and --once are mutually exclusive")
			}
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			client := supervisor.Client(active.Layout, active.Runtime)
			if !watch && !once && !opt.json {
				watch = isTerminalWriter(command.OutOrStdout())
			}
			if opt.json || once || !watch {
				snapshot := versions.Capture(command.Context(), active.Manifest, active.Runtime, client)
				if opt.json {
					return output.WriteJSON(command.OutOrStdout(), "Versions", active.Layout.ProjectID, snapshot, nil)
				}
				versions.WriteHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), snapshot)
				return nil
			}
			return watchVersions(command, active, opt)
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	command.Flags().BoolVar(&once, "once", false, "print one snapshot")
	return command
}

func watchVersions(command *cobra.Command, active lifecycle.Active, opt *options) error {
	return watchVersionsWhileRuntimeActive(command, active, opt, versionsRuntimeActive)
}

func watchVersionsWhileRuntimeActive(
	command *cobra.Command,
	active lifecycle.Active,
	opt *options,
	runtimeActive func(state.Layout, supervisor.Runtime) bool,
) error {
	ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	collector := versions.NewCollector()
	display := newVersionsWatchDisplay(
		command.OutOrStdout(),
		isTerminalWriter(command.OutOrStdout()),
		presentStyle(command.OutOrStdout(), opt.noColor),
	)
	display.open()
	defer display.close()
	var previous versions.Snapshot
	for {
		if !runtimeActive(active.Layout, active.Runtime) {
			return nil
		}
		snapshot := collector.Capture(ctx, active.Manifest, active.Runtime, supervisor.Client(active.Layout, active.Runtime))
		if previous.CapturedAt == "" || !versions.MateriallyEqual(previous, snapshot) {
			display.render(snapshot)
			previous = snapshot
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func versionsRuntimeActive(layout state.Layout, runtimeState supervisor.Runtime) bool {
	marker := filepath.Join(layout.ProjectDir, "locks", "down-"+runtimeState.GenerationID+".json")
	if _, err := os.Lstat(marker); err == nil || !os.IsNotExist(err) {
		return false
	}
	return supervisor.StaticScopeMatches(layout, runtimeState)
}

func newStatusCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status [service ...]",
		Short: "Report active runtime and service state",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
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
			workspaceStatus, err := lifecycle.InspectStatus(command.Context(), layout)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				wanted := map[string]bool{}
				for _, name := range args {
					wanted[name] = true
				}
				filtered := workspaceStatus.Services[:0]
				for _, item := range workspaceStatus.Services {
					if wanted[item.Name] {
						filtered = append(filtered, item)
					}
				}
				workspaceStatus.Services = filtered
				if workspaceStatus.ResourceGuard != nil {
					guardServices := workspaceStatus.ResourceGuard.Services[:0]
					for _, item := range workspaceStatus.ResourceGuard.Services {
						if wanted[item.Name] {
							guardServices = append(guardServices, item)
						}
					}
					workspaceStatus.ResourceGuard.Services = guardServices
				}
			}
			if opt.json {
				return output.WriteJSON(command.OutOrStdout(), "Status", layout.ProjectID, workspaceStatus, nil)
			}
			if opt.quiet {
				return nil
			}
			style := presentStyle(command.OutOrStdout(), opt.noColor)
			return lifecycle.WriteStatusHuman(command.OutOrStdout(), style, workspaceStatus)
		},
	}
}

func newLogsCommand(opt *options) *cobra.Command {
	var follow, raw bool
	var tail int
	command := &cobra.Command{
		Use:   "logs [service ...]",
		Short: "Read Process Compose service logs",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			active, err := opt.active(command.Context())
			if err != nil {
				return err
			}
			return lifecycle.Logs(command.Context(), active, args, follow, tail, raw, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
		},
	}
	command.Flags().BoolVarP(&follow, "follow", "f", false, "follow new log output")
	command.Flags().IntVar(&tail, "tail", -1, "number of lines from the end")
	command.Flags().BoolVar(&raw, "raw", false, "omit service prefixes")
	return command
}
