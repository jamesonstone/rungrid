package planner

import (
	"fmt"
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
)

// WriteHuman renders the deterministic plan: identity, lifecycle phases,
// declared repositories, recovery intent, services, and generated artifacts.
func (p Plan) WriteHuman(w io.Writer, style present.Style) {
	_ = style.Header(w, present.EmojiPlan, "Plan")
	_ = style.Field(w, "project", p.ProjectID)
	_ = style.Field(w, "generation", p.GenerationID)
	_ = style.Field(w, "manifest", p.ManifestDirectory)
	_ = style.Field(w, "workspace", p.WorkspaceRoot)
	_ = style.Field(w, "terminal", p.TerminalMode)
	_ = present.Blank(w)

	p.Lifecycle.writeHuman(w, style)
	p.writeRepositories(w, style)
	p.writeRecovery(w, style)
	p.writeServices(w, style)
	p.writeArtifacts(w, style)
}

func (p Plan) writeRepositories(w io.Writer, style present.Style) {
	_ = style.HeaderCount(w, present.EmojiRepos, "Repositories", len(p.Repositories))
	table := style.NewTable("REPOSITORY", "PATH", "REMOTE", "DEFAULT BRANCH")
	for _, repository := range p.Repositories {
		defaultBranch := repository.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = "<remote HEAD>"
		}
		table.Row(repository.Name, repository.Path, repository.Remote, defaultBranch)
	}
	_ = table.Render(w, "no repositories are declared")
	_ = present.Blank(w)
}

func (p Plan) writeRecovery(w io.Writer, style present.Style) {
	if p.Recovery == nil {
		return
	}
	_ = style.Header(w, present.EmojiRecovery, "Recovery")
	if p.Recovery.Generation == "" {
		_ = style.Note(w, present.GlyphStep, "start; no recorded lifecycle generation")
	} else {
		detail := fmt.Sprintf(
			"%s; recorded generation %s is %s (teardown-required=%t)",
			p.Recovery.Action, p.Recovery.Generation, p.Recovery.State, p.Recovery.TeardownRequired,
		)
		_ = style.Note(w, present.GlyphStep, detail)
	}
	_ = present.Blank(w)
}

func (p Plan) writeServices(w io.Writer, style present.Style) {
	_ = style.HeaderCount(w, present.EmojiServices, "Services", len(p.Services))
	table := style.NewTable("SERVICE", "REPOSITORY", "SOURCE", "ACTIVATION", "STATE")
	for _, service := range p.Services {
		stateText := "enabled"
		if service.Disabled {
			stateText = "disabled until session ownership"
		} else if !service.Process {
			stateText = "observed only"
		}
		table.Row(service.Name, service.Repository, service.Source, service.Activation, stateText)
	}
	_ = table.Render(w, "no services are declared")
	_ = present.Blank(w)
}

func (p Plan) writeArtifacts(w io.Writer, style present.Style) {
	_ = style.HeaderCount(w, present.EmojiArtifacts, "Artifacts", len(p.Artifacts))
	for _, artifact := range p.Artifacts {
		_ = style.Step(w, artifact)
	}
	_ = present.Blank(w)
	_ = style.Header(w, present.EmojiGenerate, "Required executables")
	_ = style.Note(w, present.GlyphStep, strings.Join(p.Executables, ", "))
}

func (p LifecyclePlan) writeHuman(w io.Writer, style present.Style) {
	_ = style.Header(w, present.EmojiHooks, "Lifecycle")
	writePhase(w, style, "before_up", p.BeforeUp)
	writePhase(w, style, "after_down", p.AfterDown)
	_ = style.Note(w, present.GlyphStep, "failure after prerequisites: stop runtime, then run all after_down commands")
	_ = present.Blank(w)
}

func writePhase(w io.Writer, style present.Style, name string, commands []LifecycleCommandPlan) {
	if len(commands) == 0 {
		_ = style.Field(w, name, style.Muted("none"))
		return
	}
	_ = style.Field(w, name, style.Muted(fmt.Sprintf("%d command(s)", len(commands))))
	for _, command := range commands {
		// %q over the argv slice keeps the exact, redacted argument vector
		// visible; the manifest contract requires it to stay reproducible.
		_, _ = fmt.Fprintf(
			w,
			"      %s %s %s argv=%q\n",
			present.GlyphStep,
			command.Name,
			style.Muted(fmt.Sprintf("(dir=%s timeout=%s)", command.WorkingDirectory, command.Timeout)),
			command.Argv,
		)
	}
}
