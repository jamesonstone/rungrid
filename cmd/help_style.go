package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/jamesonstone/rungrid/internal/present"
)

const (
	ansiReset     = "\033[0m"
	ansiDim       = "\033[38;5;245m"
	ansiWhiteBold = "\033[1;37m"
	ansiGray      = "\033[38;5;240m"
	ansiManifest  = "\033[38;5;45m"
	ansiPlan      = "\033[38;5;39m"
	ansiLifecycle = "\033[38;5;208m"
	ansiRuntime   = "\033[38;5;82m"
	ansiOverview  = "\033[38;5;213m"
	ansiVersions  = "\033[38;5;220m"
	ansiService   = "\033[38;5;141m"
)

var terminalWriterCheck = isTerminalWriter

type helpStyle struct {
	enabled bool
}

func helpStyleForWriter(w io.Writer, noColor bool) helpStyle {
	noColorEnvironment := os.Getenv("NO_COLOR") != ""
	return helpStyle{enabled: !noColor && !noColorEnvironment && terminalWriterCheck(w)}
}

// presentStyle resolves the command-output color decision using the same gate
// as help output: an interactive writer with neither --no-color nor NO_COLOR.
// Command output differs from help output in that its emoji are always emitted;
// only color is gated here.
func presentStyle(w io.Writer, noColor bool) present.Style {
	return present.New(helpStyleForWriter(w, noColor).enabled)
}

func isTerminalWriter(w io.Writer) bool {
	fileLike, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(fileLike.Fd())
}

func (s helpStyle) title(emoji, text string) string {
	if !s.enabled {
		return text
	}
	return ansiWhiteBold + emoji + " " + text + ansiReset
}

func (s helpStyle) label(text string) string {
	if !s.enabled {
		return text
	}
	return ansiWhiteBold + text + ansiReset
}

func (s helpStyle) muted(text string) string {
	return s.color(ansiDim, text)
}

func (s helpStyle) color(color, text string) string {
	if !s.enabled {
		return text
	}
	return color + text + ansiReset
}

func helpTemplate(style helpStyle) string {
	usageHeader := "Usage:"
	aliasesHeader := "Aliases:"
	examplesHeader := "Examples:"
	commandsHeader := "Available Commands:"
	flagsHeader := "Flags:"
	globalFlagsHeader := "Global Flags:"
	additionalHelpHeader := "Additional Help Topics:"
	moreInfoLabel := "Use"

	if style.enabled {
		usageHeader = style.title("🚀", "Usage")
		aliasesHeader = style.title("🏷️", "Aliases")
		examplesHeader = style.title("🧪", "Examples")
		commandsHeader = style.title("🧰", "Available Commands")
		flagsHeader = style.title("⚙️", "Flags")
		globalFlagsHeader = style.title("🌐", "Global Flags")
		additionalHelpHeader = style.title("📚", "Additional Help Topics")
		moreInfoLabel = style.title("🔎", "Use")
	}

	return fmt.Sprintf(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}{{end}}

%s
  {{if .Runnable}}{{.UseLine}}{{end}}{{if and .Runnable .HasAvailableSubCommands}}
  {{end}}{{if .HasAvailableSubCommands}}{{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

%s
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

%s
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

%s
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

%s
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

%s
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

%s
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}  {{rpad .CommandPath .CommandPathPadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

%s "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}{{if not .HasAvailableSubCommands}}
{{end}}`,
		usageHeader,
		aliasesHeader,
		examplesHeader,
		commandsHeader,
		flagsHeader,
		globalFlagsHeader,
		additionalHelpHeader,
		moreInfoLabel,
	)
}

func usageTemplate(style helpStyle) string {
	usageHeader := "Usage:"
	aliasesHeader := "Aliases:"
	commandsHeader := "Available Commands:"
	flagsHeader := "Flags:"
	globalFlagsHeader := "Global Flags:"
	if style.enabled {
		usageHeader = style.title("🚀", "Usage")
		aliasesHeader = style.title("🏷️", "Aliases")
		commandsHeader = style.title("🧰", "Available Commands")
		flagsHeader = style.title("⚙️", "Flags")
		globalFlagsHeader = style.title("🌐", "Global Flags")
	}
	return fmt.Sprintf(`%s
  {{.UseLine}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

%s
  {{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}

%s
{{range .Commands}}{{if .IsAvailableCommand}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

%s
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

%s
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`, usageHeader, aliasesHeader, commandsHeader, flagsHeader, globalFlagsHeader)
}

func trimHelpText(value string) string {
	return strings.TrimRight(value, "\n")
}
