package doctor

import (
	"fmt"
	"io"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
)

// WriteHuman renders the doctor report: a headline verdict followed by a
// box-drawn table of every check, its status word, and its detail.
func WriteHuman(w io.Writer, style present.Style, report Report) error {
	verdict, glyph := "configuration and local dependencies are ready", present.GlyphOK
	if !report.OK {
		verdict, glyph = "blocking problems were found", present.GlyphError
	}
	if _, err := fmt.Fprintf(w, "%s %s  %s %s\n\n", present.EmojiDoctor, style.Bold("Doctor"), glyph, verdict); err != nil {
		return err
	}
	table := style.NewTable("", "CHECK", "SUMMARY", "DETAIL")
	for _, check := range report.Checks {
		table.Row(present.CheckGlyph(check.Status), check.Name, check.Summary, check.Detail)
	}
	if err := table.Render(w, "no checks ran"); err != nil {
		return err
	}
	return writeDoctorTotals(w, style, report)
}

func writeDoctorTotals(w io.Writer, style present.Style, report Report) error {
	counts := map[string]int{}
	for _, check := range report.Checks {
		counts[strings.ToLower(check.Status)]++
	}
	summary := fmt.Sprintf(
		"%s %d ok   %s %d warning   %s %d error",
		present.GlyphOK, counts["ok"],
		present.GlyphWarning, counts["warning"],
		present.GlyphError, counts["error"],
	)
	if err := present.Blank(w); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "   %s\n", style.Muted(summary))
	return err
}
