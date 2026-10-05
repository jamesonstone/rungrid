//go:build darwin || linux

package lifecycle

import (
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/state"
)

// attachOverrides marks every service of an overridden repository and lists
// the active overrides of the runtime generation.
func attachOverrides(layout state.Layout, status *WorkspaceStatus) error {
	entries, err := override.Load(layout, status.Generation)
	if err != nil {
		return err
	}
	status.Overrides = override.Sorted(entries)
	for _, entry := range status.Overrides {
		for index := range status.Services {
			if contains(entry.Services, status.Services[index].Name) {
				status.Services[index].Override = &ServiceOverride{Repository: entry.Repository, Path: entry.Path, Branch: entry.Branch}
			}
		}
	}
	return nil
}

func serviceNameCell(service ServiceStatus) string {
	name := present.ServiceGlyph(service.Status, service.Health) + " " + service.Name
	if service.Override != nil {
		name += " " + present.EmojiOverride
	}
	return name
}

func writeStatusOverrides(w io.Writer, style present.Style, entries []override.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := present.Blank(w); err != nil {
		return err
	}
	if err := style.HeaderCount(w, present.EmojiOverride, "Overrides", len(entries)); err != nil {
		return err
	}
	table := style.NewTable("REPOSITORY", "PATH", "BRANCH", "DIRTY", "SERVICES")
	for _, entry := range entries {
		table.Row(entry.Repository, entry.Path, present.Fallback(entry.Branch), yesNo(entry.Dirty), strings.Join(entry.Services, ", "))
	}
	return table.Render(w, "")
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
