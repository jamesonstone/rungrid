package versions

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
)

// WriteHuman renders the Versions surface: process state, listeners, and
// source-control position for every declared service.
func WriteHuman(w io.Writer, style present.Style, snapshot Snapshot) {
	// Capture records the runtime as "running"; ServiceGlyph applies the same
	// vocabulary here so the surface never invents a second set of state names.
	runtimeGlyph := present.ServiceGlyph(snapshot.Runtime, "")
	_, _ = fmt.Fprintf(
		w,
		"%s %s  %s %s  %s %s  %s\n\n",
		present.EmojiVersions,
		style.Bold("Versions"),
		runtimeGlyph,
		present.Fallback(snapshot.Runtime),
		style.Muted("generation"),
		present.Fallback(snapshot.Generation),
		style.Muted(snapshot.CapturedAt),
	)
	table := style.NewTable("SERVICE", "REPOSITORY", "STATE", "HEALTH", "PID", "PORTS", "BRANCH@COMMIT", "GIT")
	for _, service := range snapshot.Services {
		table.Row(
			present.ServiceGlyph(service.State, service.Health)+" "+service.Name,
			repositoryCell(service),
			service.State,
			service.Health,
			positiveNumber(service.PID),
			formatPorts(service.Ports),
			formatVersion(service),
			service.GitState,
		)
	}
	_ = table.Render(w, "no services are declared for this generation")
	for _, service := range snapshot.Services {
		if service.Override != "" {
			_ = style.Note(w, present.EmojiOverride, service.Name+" runs from override "+service.Override+" ("+present.Fallback(service.Branch)+")")
		}
	}
}

func repositoryCell(service ServiceVersion) string {
	if service.Override == "" {
		return service.Repository
	}
	return service.Repository + " " + present.EmojiOverride + " override"
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return ""
	}
	parts := make([]string, len(ports))
	for index, port := range ports {
		parts[index] = strconv.Itoa(port)
	}
	return strings.Join(parts, ",")
}

func formatVersion(service ServiceVersion) string {
	if service.Branch == "" && service.Commit == "" {
		return ""
	}
	return service.Branch + "@" + service.Commit
}

func positiveNumber(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}
