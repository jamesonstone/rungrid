package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/present"
)

func summarizeResume(w io.Writer, style present.Style, result lifecycle.ResumeResult) {
	_ = style.HeaderCount(w, present.EmojiRecovery, "Resume", len(result.Services))
	for _, service := range result.Services {
		glyph, text := resumeServiceLine(service)
		if service.Detail != "" {
			text += style.Muted("  " + service.Detail)
		}
		_ = style.Note(w, glyph, text)
	}
	_ = present.Blank(w)
	headline := "Workspace resumed on the live runtime"
	if result.Runtime == lifecycle.ResumeRuntimeRecovered {
		headline = "Workspace recovered; the runtime was restarted"
	}
	_ = style.Result(w, present.GlyphOK, fmt.Sprintf(
		"%s  %s %d  %s %s",
		headline, style.Muted("pid"), result.RuntimePID, style.Muted("generation"), result.Generation,
	))
	switch {
	case result.OpenedWorkspace:
		_ = style.Note(w, present.GlyphStep, "opened the Warp workspace")
	case len(result.OpenedTabs) > 0:
		_ = style.Note(w, present.GlyphStep, "reopened Warp tabs: "+strings.Join(result.OpenedTabs, ", "))
	case result.WindowsLive:
		_ = style.Note(w, present.GlyphStep, "a Rungrid Warp window is still live; use --force-open to open another")
	}
}

func resumeServiceLine(service lifecycle.ResumeService) (string, string) {
	switch service.Action {
	case lifecycle.ResumeRunning:
		return present.GlyphRunning, service.Name + " is " + service.Status
	case lifecycle.ResumeRestarted:
		return present.GlyphRunning, service.Name + " restarted"
	case lifecycle.ResumeFailed:
		return present.GlyphFailed, service.Name + " could not restart"
	case lifecycle.ResumeStopped:
		return present.GlyphIdle, service.Name + " left stopped"
	case lifecycle.ResumeExternal:
		return present.GlyphExtern, service.Name + " is external"
	case lifecycle.ResumeTabLive:
		return present.GlyphRunning, service.Name + " tab is live"
	case lifecycle.ResumeTabOpened:
		return present.GlyphPending, service.Name + " tab reopened"
	default:
		return present.GlyphIdle, service.Name + " tab is closed"
	}
}
