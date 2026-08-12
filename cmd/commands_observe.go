package cmd

import (
	"fmt"
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
				versions.WriteHuman(command.OutOrStdout(), snapshot)
				return nil
			}
			return watchVersions(command, active)
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	command.Flags().BoolVar(&once, "once", false, "print one snapshot")
	return command
}

func watchVersions(command *cobra.Command, active lifecycle.Active) error {
	return watchVersionsWhileRuntimeActive(command, active, versionsRuntimeActive)
}

func watchVersionsWhileRuntimeActive(command *cobra.Command, active lifecycle.Active, runtimeActive func(state.Layout, supervisor.Runtime) bool) error {
	ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	collector := versions.NewCollector()
	display := newVersionsWatchDisplay(command.OutOrStdout(), isTerminalWriter(command.OutOrStdout()))
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
			if !opt.quiet {
				_, _ = fmt.Fprintf(command.OutOrStdout(), "runtime %s", workspaceStatus.Runtime)
				if workspaceStatus.PID != 0 {
					_, _ = fmt.Fprintf(command.OutOrStdout(), "  PID %d", workspaceStatus.PID)
				}
				if workspaceStatus.Generation != "" {
					_, _ = fmt.Fprintf(command.OutOrStdout(), "  generation %s", workspaceStatus.Generation)
				}
				_, _ = fmt.Fprintln(command.OutOrStdout())
				if workspaceStatus.RuntimeVerification != "" {
					_, _ = fmt.Fprintf(command.OutOrStdout(), "runtime verification: %s\n", workspaceStatus.RuntimeVerification)
				}
				if workspaceStatus.Lifecycle != nil {
					_, _ = fmt.Fprintf(
						command.OutOrStdout(),
						"lifecycle %s  teardown-required=%t  completed-before-up=%d\n",
						workspaceStatus.Lifecycle.State,
						workspaceStatus.Lifecycle.TeardownRequired,
						len(workspaceStatus.Lifecycle.CompletedBefore),
					)
					if workspaceStatus.Lifecycle.CleanupFailure != "" {
						_, _ = fmt.Fprintf(command.OutOrStdout(), "cleanup failure: %s\n", workspaceStatus.Lifecycle.CleanupFailure)
					}
					if workspaceStatus.Lifecycle.LastFailure != nil {
						_, _ = fmt.Fprintf(
							command.OutOrStdout(),
							"last failed command: %s/%s (%s)\n",
							workspaceStatus.Lifecycle.LastFailure.Phase,
							workspaceStatus.Lifecycle.LastFailure.Name,
							workspaceStatus.Lifecycle.LastFailure.Status,
						)
					}
				}
				if guard := workspaceStatus.ResourceGuard; guard != nil {
					_, _ = fmt.Fprintf(command.OutOrStdout(), "resource guard %s  scope-valid=%t  heartbeat=%s", guard.Health, guard.AuthorityValid, guard.HeartbeatAt)
					if guard.DegradedReason != "" {
						_, _ = fmt.Fprintf(command.OutOrStdout(), "  degraded=%s", guard.DegradedReason)
					}
					_, _ = fmt.Fprintf(command.OutOrStdout(), "  guard-pid=%d cpu=%.1f%% rss=%d sampler=%.1fms\n", guard.GuardPID, guard.GuardCPUPercent, guard.GuardRSSBytes, guard.SamplerDurationMS)
					_, _ = fmt.Fprintf(
						command.OutOrStdout(),
						"  scope project=%s generation=%s manifest=%s runtime-pid=%d socket=%s\n",
						guard.Scope.ProjectID,
						guard.Scope.GenerationID,
						shortHash(guard.Scope.EffectiveManifestSHA256),
						guard.Scope.RuntimePID,
						guard.Scope.SocketPath,
					)
					if incident := guard.LatestControlIncident; incident != nil {
						_, _ = fmt.Fprintf(command.OutOrStdout(), "  latest control incident=%s trigger=%s action=%s\n", incident.OccurredAt, incident.Trigger, incident.Action)
					}
				}
				for _, item := range workspaceStatus.Services {
					_, _ = fmt.Fprintf(command.OutOrStdout(), "%-20s %-10s %-9s %-14s pid=%d health=%s session=%t tab=%t\n", item.Name, item.Source, item.Activation, item.Status, item.PID, item.Health, item.SessionOwned, item.TabRegistered)
					if guard := item.ResourceGuard; guard != nil {
						_, _ = fmt.Fprintf(command.OutOrStdout(), "  guard=%s enforcement=%s scope-valid=%t cpu=%.1f%%/%.1f%% memory=%.1f%%/%.1f%% processes=%d/%d threads=%d/%d learning=%s mature=%t restarts=%d circuit=%s\n", guard.State, guard.Enforcement, guard.AuthorityValid, guard.Metrics.CPUPercent, guard.EffectiveLimits.CPUPercent, guard.Metrics.MemoryPercent, guard.EffectiveLimits.MemoryPercent, guard.Metrics.Processes, guard.EffectiveLimits.Processes, guard.Metrics.Threads, guard.EffectiveLimits.Threads, guard.Baseline.HealthyDuration, guard.Baseline.Mature, guard.RestartCount, guard.CircuitState)
						if guard.LatestIncident != nil {
							_, _ = fmt.Fprintf(command.OutOrStdout(), "  latest incident=%s tier=%s trigger=%s action=%s\n", guard.LatestIncident.OccurredAt, guard.LatestIncident.Tier, guard.LatestIncident.Trigger, guard.LatestIncident.Action)
						}
					}
				}
			}
			return nil
		},
	}
}

func shortHash(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
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
