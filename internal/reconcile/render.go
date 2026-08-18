package reconcile

import (
	"fmt"
	"io"

	"github.com/jamesonstone/rungrid/internal/present"
)

// WriteHuman renders the filesystem reconciliation report: one row per
// repository covering its fast-forward decision, primary-checkout decision, and
// worktree cleanup eligibility.
func WriteHuman(writer io.Writer, style present.Style, report Report) error {
	if err := style.HeaderCount(writer, present.EmojiWorktrees, "Reconcile", len(report.Repositories)); err != nil {
		return err
	}
	if report.DryRun {
		if err := style.Note(writer, present.GlyphStep, "dry run: live state was queried without fetching or changing anything"); err != nil {
			return err
		}
	}
	table := style.NewTable("", "REPOSITORY", "DEFAULT", "SYNC", "ROOT", "CLEANUP")
	for _, repository := range report.Repositories {
		table.Row(
			present.ActionGlyph(repository.Root.Action),
			repository.Name,
			repository.DefaultBranch,
			repository.Sync.Action+":"+repository.Sync.State,
			repository.Root.Action+":"+repository.Root.Reason,
			cleanupSummary(repository),
		)
	}
	if err := table.Render(writer, "no repositories were reconciled"); err != nil {
		return err
	}
	for _, failure := range report.Failures {
		path := ""
		if failure.Path != "" {
			path = " " + failure.Path
		}
		detail := fmt.Sprintf("%s %s%s: %s", failure.Repository, failure.Operation, path, failure.Error)
		if err := style.Warning(writer, detail); err != nil {
			return err
		}
	}
	return nil
}

func cleanupSummary(repository RepositoryResult) string {
	removed, preserved := 0, 0
	for _, worktree := range repository.Worktrees {
		switch worktree.Action {
		case "removed", "remove", "would-remove":
			removed++
		default:
			preserved++
		}
	}
	if removed == 0 && preserved == 0 {
		return "none"
	}
	return fmt.Sprintf("eligible=%d preserved=%d", removed, preserved)
}
