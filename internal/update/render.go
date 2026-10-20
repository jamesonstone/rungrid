package update

import (
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
)

// WriteHuman renders one repository, step, and service table. A dry run
// labels planned changes WOULD UPDATE.
func WriteHuman(writer io.Writer, style present.Style, report Report) error {
	title := "Workspace update"
	switch {
	case report.DryRun:
		title += " plan (dry run)"
	case report.Baseline:
		title += " baseline"
	}
	if err := style.HeaderCount(writer, present.EmojiSync, title, len(report.Repositories)); err != nil {
		return err
	}
	repositories := style.NewTable("", "REPOSITORY", "DEFAULT", "FROM", "TO", "GIT", "STATUS", "REASON")
	steps := style.NewTable("", "REPOSITORY", "STEP", "DIRECTORY", "SERVICES", "STATUS", "REASON")
	services := style.NewTable("", "REPOSITORY", "SERVICE", "RESTART", "STATUS", "REASON")
	for _, repository := range report.Repositories {
		repositories.Row(statusGlyph(repository.Status, report.DryRun), repository.Name, repository.DefaultBranch,
			shortOID(repository.FromOID), shortOID(repository.ToOID), repository.Git,
			statusWord(repository.Status, report.DryRun), repository.Reason)
		for _, step := range repository.Steps {
			steps.Row(statusGlyph(step.Status, report.DryRun), repository.Name, step.Name, step.Directory,
				strings.Join(step.Services, ","), statusWord(step.Status, report.DryRun), step.Reason)
		}
		for _, service := range repository.Services {
			services.Row(statusGlyph(service.Status, report.DryRun), repository.Name, service.Name, service.Restart,
				statusWord(service.Status, report.DryRun), service.Reason)
		}
	}
	if err := repositories.Render(writer, "no repositories were selected"); err != nil {
		return err
	}
	if steps.Len() > 0 {
		if err := present.Blank(writer); err != nil {
			return err
		}
		if err := steps.Render(writer, ""); err != nil {
			return err
		}
	}
	if services.Len() > 0 {
		if err := present.Blank(writer); err != nil {
			return err
		}
		if err := services.Render(writer, ""); err != nil {
			return err
		}
	}
	for _, failure := range report.Failures {
		if err := style.Warning(writer, failure.Repository+" "+failure.Operation+": "+failure.Error); err != nil {
			return err
		}
	}
	return nil
}

func statusWord(status string, dryRun bool) string {
	if dryRun && status == StatusUpdated {
		return "WOULD UPDATE"
	}
	return status
}

func statusGlyph(status string, dryRun bool) string {
	switch status {
	case StatusUpdated:
		if dryRun {
			return present.GlyphStep
		}
		return present.GlyphOK
	case StatusFailed:
		return present.GlyphError
	case StatusSkipped:
		return present.GlyphWarning
	default:
		return present.GlyphIdle
	}
}

func shortOID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
