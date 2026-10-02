package override

import (
	"context"
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/present"
)

// Refresh re-reads each override checkout's branch, HEAD, and dirty state so
// listings report the checkout as it is now rather than when it was set.
func Refresh(ctx context.Context, entries []Entry, runner maintenance.Runner) []Entry {
	runner = defaultRunner(runner)
	result := make([]Entry, len(entries))
	for index, entry := range entries {
		entry.Branch, entry.HeadOID = headIdentity(ctx, runner, entry.Path)
		status, err := gitText(ctx, runner, entry.Path, "status", "--porcelain")
		entry.Dirty = err == nil && status != ""
		if entry.Reanchored == nil {
			entry.Reanchored = []Reanchor{}
		}
		result[index] = entry
	}
	return result
}

// WriteReportHuman renders the outcome of set, clear, sync, or up seeding.
func WriteReportHuman(w io.Writer, style present.Style, report Report) error {
	if err := style.Header(w, present.EmojiOverride, "Overrides "+report.Operation); err != nil {
		return err
	}
	for _, entry := range report.Set {
		_ = style.Result(w, present.GlyphOK, entry.Repository+" now runs from "+entry.Path+" ("+branchLabel(entry)+")")
		_ = style.Note(w, present.GlyphStep, "services: "+strings.Join(entry.Services, ", "))
		writeReanchors(w, style, entry.Reanchored)
	}
	for _, entry := range report.Cleared {
		_ = style.Result(w, present.GlyphOK, entry.Repository+" restored to "+entry.OriginalPath)
	}
	if len(report.Set) == 0 && len(report.Cleared) == 0 {
		_ = style.Note(w, present.GlyphIdle, "no override changed")
	}
	for _, warning := range report.Warnings {
		_ = style.Warning(w, warning)
	}
	if len(report.Services) == 0 {
		return nil
	}
	if err := present.Blank(w); err != nil {
		return err
	}
	table := style.NewTable("SERVICE", "REPOSITORY", "ACTION", "DETAIL")
	for _, item := range report.Services {
		table.Row(actionGlyph(item.Action)+" "+item.Name, item.Repository, item.Action, item.Detail)
	}
	return table.Render(w, "")
}

// WriteListHuman renders the active overrides of the runtime generation.
func WriteListHuman(w io.Writer, style present.Style, report ListReport) error {
	if err := style.HeaderCount(w, present.EmojiOverride, "Overrides", len(report.Overrides)); err != nil {
		return err
	}
	table := style.NewTable("REPOSITORY", "PATH", "BRANCH", "HEAD", "DIRTY", "SERVICES")
	for _, entry := range report.Overrides {
		dirty := "no"
		if entry.Dirty {
			dirty = "yes"
		}
		table.Row(entry.Repository, entry.Path, branchLabel(entry), shortOID(entry.HeadOID), dirty, strings.Join(entry.Services, ", "))
	}
	if err := table.Render(w, "no overrides are active; every service runs from its declared checkout"); err != nil {
		return err
	}
	for _, entry := range report.Overrides {
		writeReanchors(w, style, entry.Reanchored)
	}
	return nil
}

func writeReanchors(w io.Writer, style present.Style, changes []Reanchor) {
	for _, change := range changes {
		_ = style.Note(w, present.GlyphWarning, "re-anchored "+change.Service+" "+change.Field+": "+change.Original+" → "+change.Resolved)
	}
}

func actionGlyph(action string) string {
	switch action {
	case ActionRestarted:
		return present.GlyphOK
	case ActionRestartFailed:
		return present.GlyphError
	case ActionTabStopped:
		return present.GlyphWarning
	default:
		return present.GlyphIdle
	}
}

func branchLabel(entry Entry) string {
	if entry.Branch == "" {
		return "detached"
	}
	return entry.Branch
}

func shortOID(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
