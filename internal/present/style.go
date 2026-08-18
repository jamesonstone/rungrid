// Package present holds the shared human-output vocabulary: the color gate,
// status glyphs, box-drawn tables, and section primitives. Emoji is content and
// is always emitted; ANSI color is decoration and is emitted only when the
// caller resolves the destination as an interactive, color-enabled terminal.
package present

const (
	Reset     = "\033[0m"
	Dim       = "\033[38;5;245m"
	WhiteBold = "\033[1;37m"
	Gray      = "\033[38;5;240m"
	Manifest  = "\033[38;5;45m"
	Plan      = "\033[38;5;39m"
	Lifecycle = "\033[38;5;208m"
	Runtime   = "\033[38;5;82m"
	Overview  = "\033[38;5;213m"
	Versions  = "\033[38;5;220m"
	Service   = "\033[38;5;141m"
)

// Style carries the resolved color decision. It performs no terminal detection;
// callers resolve that once and pass the answer in.
type Style struct {
	Color bool
}

// New builds a Style. Pass true only when the destination writer is an
// interactive terminal and neither --no-color nor NO_COLOR is set.
func New(colorEnabled bool) Style { return Style{Color: colorEnabled} }

// Paint wraps text in an ANSI color when color is enabled, and returns it
// untouched otherwise.
func (s Style) Paint(color, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return color + text + Reset
}

// Bold emphasizes a label.
func (s Style) Bold(text string) string { return s.Paint(WhiteBold, text) }

// Muted de-emphasizes secondary detail.
func (s Style) Muted(text string) string { return s.Paint(Dim, text) }
