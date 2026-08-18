package maintenance

import (
	"io"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
)

// WriteSyncHuman renders the repository fast-forward report.
func WriteSyncHuman(writer io.Writer, style present.Style, report SyncReport) error {
	if err := style.HeaderCount(writer, present.EmojiSync, "Repository sync", len(report.Repositories)); err != nil {
		return err
	}
	table := style.NewTable("", "REPOSITORY", "DEFAULT", "LOCAL", "REMOTE", "SERVICES", "RESULT")
	for _, repository := range report.Repositories {
		table.Row(
			present.ActionGlyph(repository.Action),
			repository.Name,
			repository.DefaultBranch,
			shortOID(repository.LocalOID),
			shortOID(repository.RemoteOID),
			strings.Join(repository.Services, ","),
			syncResult(repository),
		)
	}
	if err := table.Render(writer, "no repositories were selected"); err != nil {
		return err
	}
	return writeFailures(writer, style, report.Failures)
}

// WritePruneHuman renders the worktree removal decisions and their proof.
func WritePruneHuman(writer io.Writer, style present.Style, report PruneReport) error {
	decisions := 0
	for _, repository := range report.Repositories {
		decisions += len(repository.Worktrees)
	}
	if err := style.HeaderCount(writer, present.EmojiWorktrees, "Worktrees", decisions); err != nil {
		return err
	}
	table := style.NewTable("", "REPOSITORY", "WORKTREE", "BRANCH", "PR", "ACTION", "REASON")
	for _, repository := range report.Repositories {
		for _, decision := range repository.Worktrees {
			pullRequest := ""
			if decision.PullRequest != nil {
				pullRequest = "#" + strconv.Itoa(decision.PullRequest.Number)
			}
			table.Row(
				present.ActionGlyph(decision.Action),
				repository.Name,
				decision.Path,
				decision.Branch,
				pullRequest,
				decision.Action,
				decision.Reason,
			)
		}
	}
	if err := table.Render(writer, "no worktrees were evaluated"); err != nil {
		return err
	}
	return writeFailures(writer, style, report.Failures)
}

func writeFailures(writer io.Writer, style present.Style, failures []Failure) error {
	for _, failure := range failures {
		detail := failure.Repository + " " + failure.Operation + ": " + failure.Error
		if err := style.Warning(writer, detail); err != nil {
			return err
		}
	}
	return nil
}

func syncResult(repository SyncRepository) string {
	if repository.Action != "preserved" {
		return repository.Action
	}
	if repository.Detail != "" {
		return repository.State + ": " + repository.Detail
	}
	return repository.State
}

func RemovalCount(report PruneReport) int {
	count := 0
	for _, repository := range report.Repositories {
		for _, decision := range repository.Worktrees {
			if decision.Action == "remove" || decision.Action == "would-remove" {
				count++
			}
		}
	}
	return count
}

func shortOID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
