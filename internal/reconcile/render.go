package reconcile

import (
	"fmt"
	"io"
)

func WriteHuman(writer io.Writer, report Report) error {
	if _, err := fmt.Fprintln(writer, "REPOSITORY  DEFAULT  SYNC              ROOT                         CLEANUP"); err != nil {
		return err
	}
	for _, repository := range report.Repositories {
		cleanup := cleanupSummary(repository)
		root := repository.Root.Action + ":" + repository.Root.Reason
		sync := repository.Sync.Action + ":" + repository.Sync.State
		if _, err := fmt.Fprintf(writer, "%-11s %-8s %-17s %-28s %s\n",
			repository.Name, dash(repository.DefaultBranch), sync, root, cleanup); err != nil {
			return err
		}
	}
	for _, failure := range report.Failures {
		path := ""
		if failure.Path != "" {
			path = " " + failure.Path
		}
		if _, err := fmt.Fprintf(writer, "warning: %s %s%s: %s\n", failure.Repository, failure.Operation, path, failure.Error); err != nil {
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

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
