package checkout

import (
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
)

func WriteListHuman(writer io.Writer, style present.Style, report ListReport) error {
	count := 0
	for _, repository := range report.Repositories {
		count += len(repository.Worktrees)
	}
	if err := style.HeaderCount(writer, present.EmojiWorktrees, "Worktrees", count); err != nil {
		return err
	}
	table := style.NewTable("", "REPOSITORY", "WORKTREE", "BRANCH", "HEAD", "SELECTED BY")
	for _, repository := range report.Repositories {
		for _, worktree := range repository.Worktrees {
			label := worktree.Path
			if worktree.Primary {
				label = worktree.Path + " (primary)"
			}
			table.Row(
				present.ActionGlyph("none"),
				repository.Name,
				label,
				worktree.Branch,
				shortOID(worktree.HeadOID),
				strings.Join(worktree.SelectedBy, ","),
			)
		}
	}
	if err := table.Render(writer, "no registered worktrees were found"); err != nil {
		return err
	}
	return writeFailures(writer, style, report.Failures)
}

func WriteUseHuman(writer io.Writer, style present.Style, report UseReport) error {
	if err := style.HeaderCount(writer, present.EmojiWorktrees, "Worktree selection", 1); err != nil {
		return err
	}
	table := style.NewTable("", "SERVICE", "WORKTREE", "BRANCH", "ACTION")
	table.Row(present.ActionGlyph(report.Action), report.Service, report.Path, report.Branch, report.Action)
	if err := table.Render(writer, "no selection changed"); err != nil {
		return err
	}
	if report.Detail != "" {
		return style.Note(writer, present.GlyphStep, report.Detail)
	}
	return nil
}

func WriteUpdateHuman(writer io.Writer, style present.Style, report UpdateReport) error {
	if err := style.HeaderCount(writer, present.EmojiWorktrees, "Worktree update", len(report.Targets)); err != nil {
		return err
	}
	table := style.NewTable("", "SERVICE", "WORKTREE", "BRANCH", "RESULT")
	for _, target := range report.Targets {
		result := target.Action
		if target.Action == "preserved" && target.Detail != "" {
			result = target.State + ": " + target.Detail
		} else if target.Action == "none" {
			result = target.State
		}
		table.Row(present.ActionGlyph(target.Action), target.Service, target.Path, target.Branch, result)
	}
	if err := table.Render(writer, "no selected feature worktrees were updated"); err != nil {
		return err
	}
	return writeFailures(writer, style, report.Failures)
}

func writeFailures(writer io.Writer, style present.Style, failures []Failure) error {
	for _, failure := range failures {
		detail := failure.Operation + ": " + failure.Error
		if failure.Service != "" {
			detail = failure.Service + " " + detail
		} else if failure.Repository != "" {
			detail = failure.Repository + " " + detail
		}
		if err := style.Warning(writer, detail); err != nil {
			return err
		}
	}
	return nil
}

func shortOID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
