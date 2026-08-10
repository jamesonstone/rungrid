package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type helpCommandSection struct {
	title    string
	commands []string
}

var rootHelpSections = []helpCommandSection{
	{title: "Configure", commands: []string{"init", "instructions", "doctor", "config"}},
	{title: "Build & Launch", commands: []string{"plan", "generate", "up", "open"}},
	{title: "Observe", commands: []string{"attach", "versions", "status", "logs"}},
	{title: "Maintain", commands: []string{"sync", "reconcile", "worktrees"}},
	{title: "Control", commands: []string{"session", "start", "stop", "down"}},
	{title: "Cleanup & Utilities", commands: []string{"uninstall", "completion", "version", "help"}},
}

func configureHelp(root *cobra.Command, opt *options) {
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(command *cobra.Command, args []string) {
		if command == root {
			_ = renderRootHelp(command, opt)
			return
		}
		style := helpStyleForWriter(command.OutOrStdout(), opt.noColor)
		command.SetHelpTemplate(helpTemplate(style))
		defaultHelp(command, args)
	})

	defaultUsage := root.UsageFunc()
	root.SetUsageFunc(func(command *cobra.Command) error {
		style := helpStyleForWriter(command.OutOrStdout(), opt.noColor)
		command.SetUsageTemplate(usageTemplate(style))
		return defaultUsage(command)
	})
}

func renderRootHelp(command *cobra.Command, opt *options) error {
	out := command.OutOrStdout()
	style := helpStyleForWriter(out, opt.noColor)
	if _, err := fmt.Fprintln(out, trimHelpText(rootHelpIntroduction(style))); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "\n"+style.title("🚀", "Usage")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  %s [command]\n", command.CommandPath()); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "\n"+style.title("🧰", "Available Commands")); err != nil {
		return err
	}

	padding := rootHelpNamePadding(command)
	for _, section := range rootHelpSections {
		rendered := false
		for _, name := range section.commands {
			subcommand := visibleSubcommand(command, name)
			if subcommand == nil {
				continue
			}
			if !rendered {
				if _, err := fmt.Fprintln(out, "\n"+style.label(section.title)); err != nil {
					return err
				}
				rendered = true
			}
			if _, err := fmt.Fprintf(out, "  %s %s\n", padHelpName(subcommand.Name(), padding), subcommand.Short); err != nil {
				return err
			}
		}
	}

	flags := strings.TrimRight(command.Flags().FlagUsages(), "\n")
	if strings.TrimSpace(flags) != "" {
		if _, err := fmt.Fprintln(out, "\n\n"+style.title("⚙️", "Flags")); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, flags); err != nil {
			return err
		}
	}
	label := "Use"
	if style.enabled {
		label = "🔎 Use"
	}
	_, err := fmt.Fprintf(out, "\n%s \"%s [command] --help\" for more information about a command.\n",
		label, command.CommandPath())
	return err
}

func visibleSubcommand(command *cobra.Command, name string) *cobra.Command {
	for _, subcommand := range command.Commands() {
		if subcommand.Name() != name {
			continue
		}
		if !subcommand.IsAvailableCommand() && subcommand.Name() != "help" {
			return nil
		}
		return subcommand
	}
	return nil
}

func rootHelpNamePadding(command *cobra.Command) int {
	width := 0
	for _, section := range rootHelpSections {
		for _, name := range section.commands {
			if subcommand := visibleSubcommand(command, name); subcommand != nil && len(name) > width {
				width = len(name)
			}
		}
	}
	if width == 0 {
		return 12
	}
	return width + 2
}

func padHelpName(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func rootHelpIntroduction(style helpStyle) string {
	return rootHelpBanner(style) + `
Rungrid compiles a portable .rungrid.yaml into one observable local
development workspace. Process Compose owns managed-service lifecycle while
Warp or a headless shell provides the operator surface.

` + rootLifecycleDiagram(style)
}

func rootHelpBanner(style helpStyle) string {
	colors := []string{
		"\033[38;5;51m",
		"\033[38;5;45m",
		"\033[38;5;39m",
		"\033[38;5;75m",
		"\033[38;5;111m",
		"\033[38;5;147m",
	}
	lines := []string{
		"██████╗ ██╗   ██╗███╗   ██╗ ██████╗ ██████╗ ██╗██████╗",
		"██╔══██╗██║   ██║████╗  ██║██╔════╝ ██╔══██╗██║██╔══██╗",
		"██████╔╝██║   ██║██╔██╗ ██║██║  ███╗██████╔╝██║██║  ██║",
		"██╔══██╗██║   ██║██║╚██╗██║██║   ██║██╔══██╗██║██║  ██║",
		"██║  ██║╚██████╔╝██║ ╚████║╚██████╔╝██║  ██║██║██████╔╝",
		"╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝╚═╝╚═════╝",
	}
	var result strings.Builder
	for index, line := range lines {
		result.WriteString("           ")
		result.WriteString(style.color(colors[index], line))
		result.WriteByte('\n')
	}
	result.WriteByte('\n')
	result.WriteString("                       ")
	result.WriteString(style.muted("one workspace. truthful lifecycle."))
	result.WriteByte('\n')
	return result.String()
}

func rootLifecycleDiagram(style helpStyle) string {
	manifest := style.color(ansiManifest, ".rungrid.yaml")
	plan := style.color(ansiPlan, "plan / generate")
	up := style.color(ansiLifecycle, "rungrid up")
	beforeUp := style.color(ansiLifecycle, "lifecycle.before_up")
	runtime := style.color(ansiRuntime, "Process Compose")
	overview := style.color(ansiOverview, "Overview")
	versions := style.color(ansiVersions, "Versions")
	serviceTabs := style.color(ansiService, "Service tabs")
	maintenance := style.color(ansiPlan, "Maintenance jobs")
	down := style.color(ansiLifecycle, "rungrid down")
	afterDown := style.color(ansiLifecycle, "lifecycle.after_down")
	branch := func(value string) string { return style.color(ansiGray, value) }

	lines := []string{
		rootHelpHeading(style, "🔁 Workspace Lifecycle") + style.muted(" (single managed-service authority):"),
		"  " + manifest + style.muted(" → portable workspace authority"),
		"    " + rootHelpArrow(style),
		"  " + plan + style.muted(" → deterministic preview and derived state"),
		"    " + rootHelpArrow(style),
		"  " + up,
		"    " + branch("├─ ") + beforeUp + style.muted(" → ordered prerequisites"),
		"    " + branch("└─ ") + runtime + style.muted(" → lifecycle and logs"),
		"         " + branch("├─ ") + overview + style.muted(" → read-only TUI and selectable logs"),
		"         " + branch("├─ ") + versions + style.muted(" → process, ports, and Git state"),
		"         " + branch("├─ ") + serviceTabs + style.muted(" → exclusive tab-owned sessions"),
		"         " + branch("└─ ") + maintenance + style.muted(" → CLI-authorized sync and prune logs"),
		"    " + rootHelpArrow(style),
		"  " + down + style.muted(" → ") + afterDown + style.muted(" → ordered teardown"),
		"",
		rootHelpHeading(style, "🧩 Service Ownership"),
		"  " + style.color(ansiRuntime, "workspace") + style.muted("  starts with rungrid up"),
		"  " + style.color(ansiService, "tab") + style.muted("        belongs to one foreground session"),
		"  " + style.color(ansiVersions, "external") + style.muted("   is observed and never managed"),
	}
	return strings.Join(lines, "\n")
}

func rootHelpHeading(style helpStyle, text string) string {
	if !style.enabled {
		return text
	}
	return ansiWhiteBold + text + ansiReset
}

func rootHelpArrow(style helpStyle) string {
	return style.color(ansiGray, "│") + "\n    " + style.color(ansiGray, "▼")
}
