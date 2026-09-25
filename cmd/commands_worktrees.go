package cmd

import (
	"errors"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/spf13/cobra"
)

func newWorktreesCommand(opt *options) *cobra.Command {
	command := &cobra.Command{
		Use:     "worktrees",
		Short:   "Inspect, select, update, and prune linked Git worktrees",
		Example: "  rungrid worktrees\n  rungrid worktrees use\n  rungrid worktrees update",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runWorktreesInteractive(command, opt)
		},
	}
	command.AddCommand(newWorktreesListCommand(opt), newWorktreesUseCommand(opt), newWorktreesUpdateCommand(opt), newWorktreesPruneCommand(opt))
	return command
}

func newWorktreesListCommand(opt *options) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List registered worktrees and current service selections",
		Example: "  rungrid worktrees list\n  rungrid worktrees list --json",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runWorktreesList(command, opt)
		},
	}
}

func newWorktreesUseCommand(opt *options) *cobra.Command {
	var clear bool
	command := &cobra.Command{
		Use:     "use [service] [selector]",
		Short:   "Run one service from an existing registered worktree",
		Example: "  rungrid worktrees use\n  rungrid worktrees use api GH-12\n  rungrid worktrees use api --clear",
		Args:    cobra.MaximumNArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			serviceName, selector := "", ""
			if len(args) > 0 {
				serviceName = args[0]
			}
			if len(args) > 1 {
				selector = args[1]
			}
			return runWorktreesUse(command, opt, serviceName, selector, clear)
		},
	}
	command.Flags().BoolVar(&clear, "clear", false, "restore the declared repository root")
	return command
}

func newWorktreesUpdateCommand(opt *options) *cobra.Command {
	var services []string
	var syncDefault, dryRun, yes bool
	command := &cobra.Command{
		Use:     "update",
		Short:   "Fast-forward selected feature worktrees",
		Example: "  rungrid worktrees update\n  rungrid worktrees update --dry-run\n  rungrid worktrees update --sync --yes",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runWorktreesUpdate(command, opt, updatePrompt{
				Services:   services,
				Sync:       syncDefault,
				DryRun:     dryRun,
				Yes:        yes,
				ServiceSet: command.Flags().Changed("service"),
				SyncSet:    command.Flags().Changed("sync"),
			})
		},
	}
	command.Flags().StringArrayVar(&services, "service", nil, "select one service (repeatable)")
	command.Flags().BoolVar(&syncDefault, "sync", false, "also fast-forward repository default branches")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "query live state without fetching or changing anything")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "apply without an interactive confirmation")
	return command
}

func runWorktreesInteractive(command *cobra.Command, opt *options) error {
	if err := requireInteractive(command, opt, "worktree commands require a subcommand with --json or a non-interactive terminal"); err != nil {
		return err
	}
	index, err := promptIndex(command, opt, "action", []string{
		"list registered worktrees",
		"select a service worktree",
		"update selected worktrees",
	})
	if err != nil {
		return err
	}
	switch index {
	case 0:
		return runWorktreesList(command, opt)
	case 1:
		return runWorktreesUse(command, opt, "", "", false)
	default:
		return runWorktreesUpdate(command, opt, updatePrompt{})
	}
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

func promptServiceName(command *cobra.Command, opt *options, m *manifest.Manifest) (string, error) {
	labels := serviceNames(m)
	index, err := promptIndex(command, opt, "service", labels)
	if err != nil {
		return "", err
	}
	return labels[index], nil
}

func promptServiceNames(command *cobra.Command, opt *options, m *manifest.Manifest) ([]string, error) {
	labels := serviceNames(m)
	indexes, err := promptIndexes(command, opt, "service", labels)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(indexes))
	for _, index := range indexes {
		result = append(result, labels[index])
	}
	return result, nil
}

func promptWorktree(command *cobra.Command, opt *options, candidates []checkout.Worktree) (string, error) {
	if len(candidates) == 0 {
		return "", errs.New(errs.ExitConflict, "RG1713", "no registered worktrees were found")
	}
	labels := make([]string, len(candidates))
	for index, candidate := range candidates {
		label := candidate.Branch
		if candidate.Primary {
			label += " (primary)"
		}
		labels[index] = label + "  " + candidate.Path
	}
	index, err := promptIndex(command, opt, "worktree", labels)
	if err != nil {
		return "", err
	}
	return candidates[index].Path, nil
}

func serviceNames(m *manifest.Manifest) []string {
	names := make([]string, 0, len(m.Services))
	for _, service := range m.Services {
		names = append(names, service.Name)
	}
	return names
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

func runWorktreesList(command *cobra.Command, opt *options) error {
	loaded, layout, err := loadedLayout(opt)
	if err != nil {
		return err
	}
	report, runErr := checkout.List(command.Context(), layout, loaded, nil)
	return errors.Join(runErr, writeListReport(command, opt, loaded.Manifest.Project.ID, report))
}
