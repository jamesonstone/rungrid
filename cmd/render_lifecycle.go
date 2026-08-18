package cmd

import (
	"fmt"
	"io"

	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/spf13/cobra"
)

// announceUp states what the workspace is about to do before Process Compose
// takes over. Rungrid hands the whole runtime to Process Compose in one call,
// so these lines describe intent; rungrid status remains the authoritative view
// of what each service is actually doing.
func announceUp(w io.Writer, style present.Style, configuration *manifest.Manifest) {
	_, _ = fmt.Fprintf(w, "%s %s  %s\n", present.EmojiUp, style.Bold("Starting workspace"), configuration.Project.ID)
	_ = present.Blank(w)
	if count := len(configuration.Lifecycle.BeforeUp); count > 0 {
		_ = style.Note(w, present.EmojiHooks, fmt.Sprintf("running %s before starting services", pluralCommands(count)))
	}
	for index := range configuration.Services {
		service := &configuration.Services[index]
		switch {
		case service.Source == "external":
			_ = style.Note(w, present.GlyphExtern, service.Name+" is external; Rungrid waits for it but never starts it")
		case service.Activation == "tab":
			_ = style.Note(w, present.GlyphIdle, service.Name+" is tab-owned; start it with rungrid session "+service.Name)
		default:
			_ = style.Step(w, "starting "+service.Name+" …")
		}
	}
	_ = present.Blank(w)
}

func summarizeUp(w io.Writer, style present.Style, result lifecycle.UpResult) {
	headline := "Workspace running"
	if result.Reused {
		headline = "Workspace already running; reused the active generation"
	}
	detail := fmt.Sprintf(
		"%s  %s %d  %s %s",
		headline,
		style.Muted("pid"), result.RuntimePID,
		style.Muted("generation"), result.Generation,
	)
	_ = style.Result(w, present.GlyphOK, detail)
	if result.OpenedWarp {
		_ = style.Note(w, present.GlyphStep, "opened the Warp workspace")
	}
}

// announceDown states the shutdown order. stopRuntime walks services in reverse
// declaration order, so the announcement mirrors that walk.
func announceDown(w io.Writer, style present.Style, projectID string, status lifecycle.WorkspaceStatus, known bool) {
	_, _ = fmt.Fprintf(w, "%s %s  %s\n", present.EmojiDown, style.Bold("Stopping workspace"), projectID)
	_ = present.Blank(w)
	if !known {
		_ = style.Note(w, present.GlyphStep, "runtime state could not be inspected; attempting shutdown anyway")
		_ = present.Blank(w)
		return
	}
	stopping := 0
	for index := len(status.Services) - 1; index >= 0; index-- {
		service := status.Services[index]
		switch {
		case service.Source == "external":
			_ = style.Note(w, present.GlyphExtern, service.Name+" is external; Rungrid leaves it running")
		case !lifecycle.Stoppable(service.Status):
			_ = style.Note(w, present.GlyphIdle, service.Name+" is already "+service.Status)
		default:
			_ = style.Step(w, "stopping "+service.Name+" …")
			stopping++
		}
	}
	if status.Lifecycle != nil && status.Lifecycle.TeardownRequired {
		_ = style.Note(w, present.EmojiHooks, "running after_down teardown commands")
	}
	if stopping == 0 && status.Runtime != "active" {
		_ = style.Note(w, present.GlyphIdle, "no active runtime was found")
	}
	_ = present.Blank(w)
}

func summarizeDown(w io.Writer, style present.Style, wasActive bool) {
	if !wasActive {
		_ = style.Result(w, present.GlyphOK, "Workspace is stopped")
		return
	}
	_ = style.Result(w, present.GlyphOK, "Workspace stopped")
}

func writeCommandResult(command *cobra.Command, opt *options, glyph, text string) {
	if opt.quiet || opt.json {
		return
	}
	style := presentStyle(command.OutOrStdout(), opt.noColor)
	_ = style.Result(command.OutOrStdout(), glyph, text)
}

func pluralCommands(count int) string {
	if count == 1 {
		return "1 before_up command"
	}
	return fmt.Sprintf("%d before_up commands", count)
}
