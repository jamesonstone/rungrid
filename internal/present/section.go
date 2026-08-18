package present

import (
	"fmt"
	"io"
	"strings"
)

// Header writes a section heading such as "🧩 Services".
func (s Style) Header(w io.Writer, emoji, title string) error {
	_, err := fmt.Fprintf(w, "%s %s\n", emoji, s.Bold(title))
	return err
}

// HeaderCount writes a section heading with a record count.
func (s Style) HeaderCount(w io.Writer, emoji, title string, count int) error {
	_, err := fmt.Fprintf(w, "%s %s %s\n", emoji, s.Bold(title), s.Muted(fmt.Sprintf("(%d)", count)))
	return err
}

// Field writes an aligned "key   value" detail line under a heading.
func (s Style) Field(w io.Writer, key, value string) error {
	_, err := fmt.Fprintf(w, "   %s  %s\n", s.Muted(pad(key, 12)), value)
	return err
}

// Step writes an announce line describing work about to happen.
func (s Style) Step(w io.Writer, text string) error {
	_, err := fmt.Fprintf(w, "   %s %s\n", GlyphStep, text)
	return err
}

// Note writes a de-emphasized line under a heading.
func (s Style) Note(w io.Writer, glyph, text string) error {
	_, err := fmt.Fprintf(w, "   %s %s\n", glyph, s.Muted(text))
	return err
}

// Result writes a terminal outcome line for a command.
func (s Style) Result(w io.Writer, glyph, text string) error {
	_, err := fmt.Fprintf(w, "%s %s\n", glyph, text)
	return err
}

// Warning writes a non-fatal problem line.
func (s Style) Warning(w io.Writer, text string) error {
	_, err := fmt.Fprintf(w, "%s %s\n", GlyphWarning, text)
	return err
}

// Blank writes a separating empty line.
func Blank(w io.Writer) error {
	_, err := fmt.Fprintln(w)
	return err
}

func pad(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}
