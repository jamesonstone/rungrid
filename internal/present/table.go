package present

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Column widths are measured with ansi.StringWidth, which ignores escape
// sequences and counts wide characters as two cells. A status glyph occupies
// two terminal cells, so byte- or rune-based padding would skew every column to
// its right as soon as emoji appear.
//
// The box is drawn here rather than by a table library because lipgloss renders
// through a shared renderer that probes the terminal for its background color
// on every call. A CLI that promises "--no-color emits no escape sequences"
// cannot write terminal queries as a side effect of printing a table.
type Table struct {
	style   Style
	headers []string
	rows    [][]string
}

// NewTable starts a table with the given column headers.
func (s Style) NewTable(headers ...string) *Table {
	return &Table{style: s, headers: headers}
}

// Row appends one record. Empty cells become an em-dash.
func (t *Table) Row(cells ...string) *Table {
	row := make([]string, len(cells))
	for index, cell := range cells {
		row[index] = Fallback(cell)
	}
	t.rows = append(t.rows, row)
	return t
}

// Len reports how many rows have been added.
func (t *Table) Len() int { return len(t.rows) }

// Render writes the table indented by one space so it sits under its section
// heading. With no rows it writes only the empty-state note.
func (t *Table) Render(w io.Writer, emptyNote string) error {
	if len(t.rows) == 0 {
		if emptyNote == "" {
			return nil
		}
		_, err := fmt.Fprintf(w, "   %s\n", t.style.Muted(emptyNote))
		return err
	}
	widths := t.columnWidths()
	lines := []string{
		t.rule(widths, "┌", "┬", "┐"),
		t.line(widths, t.headers, true),
		t.rule(widths, "├", "┼", "┤"),
	}
	for _, row := range t.rows {
		lines = append(lines, t.line(widths, row, false))
	}
	lines = append(lines, t.rule(widths, "└", "┴", "┘"))
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func (t *Table) columnWidths() []int {
	widths := make([]int, len(t.headers))
	for index, header := range t.headers {
		widths[index] = ansi.StringWidth(header)
	}
	for _, row := range t.rows {
		for index, cell := range row {
			if index >= len(widths) {
				continue
			}
			if width := ansi.StringWidth(cell); width > widths[index] {
				widths[index] = width
			}
		}
	}
	return widths
}

func (t *Table) rule(widths []int, left, middle, right string) string {
	segments := make([]string, len(widths))
	for index, width := range widths {
		segments[index] = strings.Repeat("─", width+2)
	}
	return " " + t.style.Paint(Gray, left+strings.Join(segments, middle)+right)
}

func (t *Table) line(widths []int, cells []string, header bool) string {
	bar := t.style.Paint(Gray, "│")
	parts := make([]string, 0, len(widths))
	for index, width := range widths {
		cell := ""
		if index < len(cells) {
			cell = cells[index]
		}
		padding := width - ansi.StringWidth(cell)
		if padding < 0 {
			padding = 0
		}
		if header {
			cell = t.style.Bold(cell)
		}
		parts = append(parts, " "+cell+strings.Repeat(" ", padding)+" ")
	}
	return " " + bar + strings.Join(parts, bar) + bar
}
