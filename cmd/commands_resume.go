package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"github.com/spf13/cobra"
)

func newResumeCommand(opt *options) *cobra.Command {
	var noOpen, forceOpen bool
	command := &cobra.Command{
		Use:   "resume",
		Short: "Restore Warp windows and services after a terminal restart",
		Long: "Resume reuses a live runtime: it restarts workspace services that stopped without\n" +
			"rungrid stop, then reopens only the Warp windows that did not survive. When the\n" +
			"runtime itself is gone but was never shut down, resume recovers it through up.",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if noOpen && forceOpen {
				return errs.New(errs.ExitUsage, "RG1150", "--no-open and --force-open are mutually exclusive")
			}
			ctx, cancel := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
			defer cancel()
			loaded, projectID, err := resumeProject(opt)
			if err != nil {
				return err
			}
			layout, err := state.NewLayout(projectID, opt.stateDir)
			if err != nil {
				return err
			}
			status, err := lifecycle.InspectStatus(ctx, layout)
			if err != nil {
				return err
			}
			var result lifecycle.ResumeResult
			if status.Runtime == "active" {
				result, err = resumeActive(ctx, opt, projectID, !noOpen, forceOpen)
			} else {
				result, err = recoverWorkspace(ctx, command, opt, loaded, status, !noOpen)
			}
			if writeErr := writeResumeResult(command, opt, projectID, result); writeErr != nil && err == nil {
				err = writeErr
			}
			return err
		},
	}
	command.Flags().BoolVar(&noOpen, "no-open", false, "restore services without opening Warp")
	command.Flags().BoolVar(&forceOpen, "force-open", false, "open the full Warp workspace even when a Rungrid window is live")
	return command
}

// resumeProject resolves the project id. The manifest is optional for a live
// runtime, which resumes from its own generated manifest, but required to
// recover a runtime that is gone.
func resumeProject(opt *options) (*manifest.Loaded, string, error) {
	loaded, err := opt.load()
	if err != nil {
		if opt.projectID == "" {
			return nil, "", err
		}
		return nil, opt.projectID, nil
	}
	if opt.projectID != "" && opt.projectID != loaded.Manifest.Project.ID {
		return nil, opt.projectID, nil
	}
	return loaded, loaded.Manifest.Project.ID, nil
}

func resumeActive(ctx context.Context, opt *options, projectID string, open, forceOpen bool) (lifecycle.ResumeResult, error) {
	active, err := lifecycle.LoadActive(ctx, projectID, opt.stateDir)
	if err != nil {
		return lifecycle.ResumeResult{}, err
	}
	return lifecycle.Resume(ctx, active, lifecycle.ResumeOptions{Open: open, ForceOpen: forceOpen})
}

// recoverWorkspace restarts a workspace whose runtime died without rungrid
// down. A workspace that was never started or was shut down on purpose has
// nothing to resume, so it is refused rather than silently started.
func recoverWorkspace(
	ctx context.Context,
	command *cobra.Command,
	opt *options,
	loaded *manifest.Loaded,
	status lifecycle.WorkspaceStatus,
	open bool,
) (lifecycle.ResumeResult, error) {
	if status.Lifecycle == nil || status.Lifecycle.State == workspace.StateInactive {
		return lifecycle.ResumeResult{}, errs.New(errs.ExitConflict, "RG1151", "no interrupted workspace to resume; start it with rungrid up")
	}
	if loaded == nil {
		return lifecycle.ResumeResult{}, errs.New(errs.ExitUsage, "RG1152", "recovering a stopped runtime requires this project's manifest; run resume from its manifest directory")
	}
	open = open && loaded.Manifest.Terminal.Open != nil && *loaded.Manifest.Terminal.Open
	if !opt.json && !opt.quiet {
		announceUp(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), &loaded.Manifest)
	}
	up, err := lifecycle.Up(ctx, loaded, lifecycle.UpOptions{StateOverride: opt.stateDir, GeneratorVersion: Version, Open: open, PreserveOverrides: true})
	if err != nil {
		return lifecycle.ResumeResult{}, err
	}
	result := lifecycle.ResumeResult{
		Runtime: lifecycle.ResumeRuntimeRecovered, Generation: up.Generation, RuntimePID: up.RuntimePID,
		OpenedWorkspace: up.OpenedWarp, OpenedTabs: []string{}, Services: []lifecycle.ResumeService{},
	}
	layout, err := state.NewLayout(loaded.Manifest.Project.ID, opt.stateDir)
	if err != nil {
		return result, err
	}
	recovered, err := lifecycle.InspectStatus(ctx, layout)
	if err != nil {
		return result, err
	}
	for _, service := range recovered.Services {
		action := lifecycle.ResumeRestarted
		switch {
		case service.Source == "external":
			action = lifecycle.ResumeExternal
		case service.Activation == "tab" && up.OpenedWarp:
			action = lifecycle.ResumeTabOpened
		case service.Activation == "tab":
			action = lifecycle.ResumeTabAbsent
		}
		result.Services = append(result.Services, lifecycle.ResumeService{
			Name: service.Name, Activation: service.Activation, Status: service.Status, Action: action,
		})
	}
	return result, nil
}

func writeResumeResult(command *cobra.Command, opt *options, projectID string, result lifecycle.ResumeResult) error {
	if result.Runtime == "" {
		return nil
	}
	if opt.json {
		return output.WriteJSON(command.OutOrStdout(), "Resume", projectID, result, nil)
	}
	if !opt.quiet {
		summarizeResume(command.OutOrStdout(), presentStyle(command.OutOrStdout(), opt.noColor), result)
	}
	return nil
}
