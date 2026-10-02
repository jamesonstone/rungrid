package serviceexec

import (
	"fmt"
	"io"

	"github.com/jamesonstone/rungrid/internal/override"
)

// writeOverrideBanner marks an overridden start in the service log, which is
// what the Overview shows, before the service replaces this process.
func writeOverrideBanner(w io.Writer, service string, execution override.Execution) {
	if execution.Override == nil {
		return
	}
	entry := execution.Override
	branch := entry.Branch
	if branch == "" {
		branch = "detached"
	}
	_, _ = fmt.Fprintf(w, "rungrid: %s runs from override %s (%s) for repository %s instead of %s\n",
		service, execution.WorkingDirectory, branch, entry.Repository, entry.OriginalPath)
	for _, change := range entry.Reanchored {
		if change.Service == service {
			_, _ = fmt.Fprintf(w, "rungrid: %s %s re-anchored to %s\n", change.Field, change.Original, change.Resolved)
		}
	}
}
