package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/agentexec"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/reconcile"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"github.com/spf13/cobra"
)

func newReconcileCommand(opt *options) *cobra.Command {
	var dryRun, includeSubmodules bool
	var agent string
	command := &cobra.Command{
		Use:     "reconcile [path]",
		Short:   "Safely reconcile a repository or recursive repository tree",
		Example: "  rungrid reconcile\n  rungrid reconcile ~/src --dry-run --json\n  rungrid reconcile ~/src --agent=select",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			target := "."
			if len(args) != 0 {
				target = args[0]
			}
			absolute, err := filepath.Abs(target)
			if err != nil {
				return errs.Wrap(errs.ExitUsage, "RG1631", "resolve reconciliation target", err)
			}
			ctx := command.Context()
			stopSignals := func() {}
			if agent == "" && !dryRun {
				ctx, stopSignals = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			}
			defer stopSignals()
			if agent != "" {
				if dryRun || opt.json {
					return errs.New(errs.ExitUsage, "RG1635", "--agent cannot be combined with --dry-run or --json")
				}
				provider, selectErr := resolveAgent(command, agent)
				if selectErr != nil {
					return selectErr
				}
				invocation, buildErr := agentexec.Build(provider, absolute, agentexec.ReconcilePrompt(absolute, includeSubmodules))
				if buildErr != nil {
					return buildErr
				}
				return agentexec.Run(ctx, invocation, command.InOrStdin(), command.OutOrStdout(), command.ErrOrStderr())
			}
			return runNativeReconcile(ctx, command, opt, absolute, dryRun, includeSubmodules)
		},
	}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "query live state without fetching or changing anything")
	command.Flags().BoolVar(&includeSubmodules, "include-submodules", false, "include Git submodule repositories")
	command.Flags().StringVar(&agent, "agent", "", "delegate to copilot, claude, warp, codex, or select interactively")
	command.Flags().Lookup("agent").NoOptDefVal = "copilot"
	return command
}

func runNativeReconcile(ctx context.Context, command *cobra.Command, opt *options, target string, dryRun, includeSubmodules bool) error {
	runner := maintenance.CommandRunner{}
	coordinator := maintenance.Coordinator(maintenance.NoopCoordinator{})
	recoveryTimeout := 2 * time.Minute
	projectID := ""
	var lock *workspace.Lock
	var loaded *manifest.Loaded
	if opt.projectID != "" {
		active, err := lifecycle.LoadActive(ctx, opt.projectID, opt.stateDir)
		if err != nil {
			return err
		}
		loaded = activeLoaded(active)
		coordinator = lifecycle.NewMaintenanceCoordinator(active)
	} else {
		var loadErr error
		loaded, loadErr = optionalReconcileManifest(opt, target)
		if loadErr != nil {
			return loadErr
		}
	}
	if loaded != nil {
		projectID = loaded.Manifest.Project.ID
		recoveryTimeout = loaded.Manifest.Runtime.StartupTimeout.Duration + loaded.Manifest.Runtime.ShutdownTimeout.Duration
		active, hasActive, err := optionalActive(ctx, loaded, opt.stateDir)
		if err != nil {
			return err
		}
		if hasActive {
			coordinator = lifecycle.NewMaintenanceCoordinator(active)
		}
		if !dryRun {
			lock, err = acquireMaintenanceLock(ctx, loaded, opt.stateDir)
			if err != nil {
				return err
			}
		}
	}
	report, runErr := reconcile.Run(ctx, reconcile.Options{
		Target: target, DryRun: dryRun, IncludeSubmodules: includeSubmodules,
		Runner: runner, Coordinator: coordinator, RecoveryTimeout: recoveryTimeout,
	})
	writeErr := writeReconcileReport(command, opt, projectID, report)
	var releaseErr error
	if lock != nil {
		releaseErr = lock.Release()
	}
	return errors.Join(runErr, writeErr, releaseErr)
}

func optionalReconcileManifest(opt *options, target string) (*manifest.Loaded, error) {
	path := opt.configPath
	if !filepath.IsAbs(path) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		path = absolute
	}
	if _, err := os.Stat(path); os.IsNotExist(err) && opt.configPath == ".rungrid.yaml" {
		path = filepath.Join(target, ".rungrid.yaml")
		if _, targetErr := os.Stat(path); os.IsNotExist(targetErr) {
			return nil, nil
		} else if targetErr != nil {
			return nil, errs.Wrap(errs.ExitUsage, "RG1639", "inspect reconciliation manifest", targetErr)
		}
	} else if err != nil {
		return nil, errs.Wrap(errs.ExitUsage, "RG1639", "inspect reconciliation manifest", err)
	}
	return manifest.Load(path, opt.localPath)
}

func writeReconcileReport(command *cobra.Command, opt *options, projectID string, report reconcile.Report) error {
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "RepositoryReconcileReport", projectID, report, []output.Diagnostic{})
	}
	if opt.quiet {
		return nil
	}
	return reconcile.WriteHuman(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), report)
}

func resolveAgent(command *cobra.Command, value string) (agentexec.Provider, error) {
	if value != "select" {
		provider := agentexec.Provider(value)
		if validAgent(provider) {
			return provider, nil
		}
		return "", errs.New(errs.ExitUsage, "RG1632", "unknown reconciliation agent: "+value)
	}
	if _, err := exec.LookPath("fzf"); err == nil {
		return selectAgentFZF(command)
	}
	return selectAgentNumbered(command)
}

func validAgent(provider agentexec.Provider) bool {
	for _, candidate := range agentexec.Providers {
		if provider == candidate {
			return true
		}
	}
	return false
}

func selectAgentFZF(command *cobra.Command) (agentexec.Provider, error) {
	values := make([]string, 0, len(agentexec.Providers))
	for _, provider := range agentexec.Providers {
		values = append(values, string(provider))
	}
	selector := exec.CommandContext(command.Context(), "fzf", "--prompt", "Reconciliation agent> ", "--height", "40%")
	selector.Stdin = strings.NewReader(strings.Join(values, "\n") + "\n")
	selector.Stderr = command.ErrOrStderr()
	content, err := selector.Output()
	if err != nil {
		return "", errs.Wrap(errs.ExitInterrupted, "RG1636", "select reconciliation agent", err)
	}
	return agentexec.Provider(strings.TrimSpace(string(content))), nil
}

func selectAgentNumbered(command *cobra.Command) (agentexec.Provider, error) {
	input, ok := command.InOrStdin().(*os.File)
	if !ok {
		return "", errs.New(errs.ExitUsage, "RG1637", "agent selection requires a terminal")
	}
	info, err := input.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return "", errs.New(errs.ExitUsage, "RG1637", "agent selection requires a terminal")
	}
	for index, provider := range agentexec.Providers {
		_, _ = fmt.Fprintf(command.OutOrStdout(), "%d. %s\n", index+1, provider)
	}
	_, _ = fmt.Fprint(command.OutOrStdout(), "Select reconciliation agent: ")
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return "", errs.Wrap(errs.ExitInterrupted, "RG1636", "select reconciliation agent", err)
	}
	index, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || index < 1 || index > len(agentexec.Providers) {
		return "", errs.New(errs.ExitUsage, "RG1638", "invalid reconciliation agent selection")
	}
	return agentexec.Providers[index-1], nil
}
