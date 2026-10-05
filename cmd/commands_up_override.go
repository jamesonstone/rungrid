package cmd

import (
	"context"
	"errors"

	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/spf13/cobra"
)

// planUpOverrides validates the manifest's worktree declarations and the
// --override flags before up mutates anything. Flags win over declarations
// for the same repository. The flagged plans alone are returned separately
// because a reused runtime applies only explicit flags.
func planUpOverrides(ctx context.Context, loaded *manifest.Loaded, flags []string) ([]override.Planned, []override.Planned, error) {
	requests := override.Declared(loaded)
	flagRequests := make([]override.Request, 0, len(flags))
	for _, value := range flags {
		request, err := override.ParseFlag(value)
		if err != nil {
			return nil, nil, err
		}
		flagRequests = append(flagRequests, request)
	}
	if len(requests) == 0 && len(flagRequests) == 0 {
		return nil, nil, nil
	}
	seeded, err := override.PlanAll(ctx, loaded, append(requests, flagRequests...), nil)
	if err != nil {
		return nil, nil, err
	}
	flagged, err := override.PlanAll(ctx, loaded, flagRequests, nil)
	return seeded, flagged, err
}

// finishUpOverrides applies --override flags to a reused runtime through the
// normal restart path, then reports up with the generation's overrides.
func finishUpOverrides(
	ctx context.Context,
	command *cobra.Command,
	opt *options,
	loaded *manifest.Loaded,
	result lifecycle.UpResult,
	flagged []override.Planned,
) error {
	var applyErr error
	var report override.Report
	if result.Reused && len(flagged) > 0 {
		active, err := lifecycle.LoadActive(ctx, loaded.Manifest.Project.ID, opt.stateDir)
		if err != nil {
			return err
		}
		report, applyErr = lifecycle.ApplyOverrides(ctx, active, lifecycle.OverrideChange{Operation: "up", Set: flagged})
	}
	layout, err := state.NewLayout(loaded.Manifest.Project.ID, opt.stateDir)
	if err != nil {
		return errors.Join(applyErr, err)
	}
	entries, err := override.Load(layout, result.Generation)
	if err != nil {
		return errors.Join(applyErr, err)
	}
	if opt.json {
		data := struct {
			lifecycle.UpResult
			Overrides []override.Entry `json:"overrides"`
		}{result, override.Sorted(entries)}
		return errors.Join(applyErr, output.WriteJSON(command.OutOrStdout(), "Up", loaded.Manifest.Project.ID, data, nil))
	}
	if opt.quiet {
		return applyErr
	}
	style := presentStyle(command.OutOrStdout(), opt.noColor)
	summarizeUp(command.OutOrStdout(), style, result)
	if report.Operation != "" {
		return errors.Join(applyErr, override.WriteReportHuman(command.OutOrStdout(), style, report))
	}
	for _, entry := range override.Sorted(entries) {
		_ = style.Note(command.OutOrStdout(), present.EmojiOverride, entry.Repository+" runs from override "+entry.Path)
	}
	return applyErr
}
