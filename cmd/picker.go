package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/spf13/cobra"
)

const pickerHelp = "↑/↓ or j/k move · enter select · q or esc cancel"

// pickerModel is a single-choice list navigated with arrow keys or j/k.
type pickerModel struct {
	title  string
	header string
	rows   []string
	cursor int
	chosen int
	done   bool
}

func newPickerModel(title, header string, rows []string) pickerModel {
	return pickerModel{title: title, header: header, rows: rows, chosen: -1}
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "enter":
		m.chosen, m.done = m.cursor, true
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		m.chosen, m.done = -1, true
		return m, tea.Quit
	}
	return m, nil
}

func (m pickerModel) View() string {
	if m.done {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(m.title + "\n\n")
	if m.header != "" {
		builder.WriteString("    " + m.header + "\n")
	}
	for index, row := range m.rows {
		marker := "    "
		if index == m.cursor {
			marker = "  › "
			row = ansi.Style{}.Bold().Styled(row)
		}
		builder.WriteString(marker + row + "\n")
	}
	builder.WriteString("\n" + pickerHelp + "\n")
	return builder.String()
}

// runPicker shows rows and returns the chosen index. Cancelling is an error so
// callers never act on a selection the operator did not make.
func runPicker(command *cobra.Command, title, header string, rows []string) (int, error) {
	if len(rows) == 0 {
		return 0, errs.New(errs.ExitConflict, "RG1812", "there is nothing to choose from")
	}
	program := tea.NewProgram(newPickerModel(title, header, rows), tea.WithInput(command.InOrStdin()), tea.WithOutput(command.OutOrStdout()))
	final, err := program.Run()
	if err != nil {
		return 0, errs.Wrap(errs.ExitFailure, "RG1819", "run interactive picker", err)
	}
	result, ok := final.(pickerModel)
	if !ok || result.chosen < 0 {
		return 0, errs.New(errs.ExitInterrupted, "RG1822", "selection cancelled; nothing changed")
	}
	return result.chosen, nil
}

// alignColumns pads cells into display-width-aligned columns and returns the
// header line and one line per row.
func alignColumns(headers []string, rows [][]string) (string, []string) {
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = ansi.StringWidth(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			if width := ansi.StringWidth(cell); width > widths[index] {
				widths[index] = width
			}
		}
	}
	line := func(cells []string) string {
		parts := make([]string, len(cells))
		for index, cell := range cells {
			parts[index] = cell + strings.Repeat(" ", widths[index]-ansi.StringWidth(cell))
		}
		return strings.TrimRight(strings.Join(parts, "  "), " ")
	}
	lines := make([]string, len(rows))
	for index, row := range rows {
		lines[index] = line(row)
	}
	return line(headers), lines
}
