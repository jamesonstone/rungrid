package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/spf13/cobra"
)

// serviceActions are the interactive actions offered by `rungrid <service>`.
var serviceActions = []string{"worktree  run this service's repository from another worktree"}

// runServiceShortcut handles `rungrid <service> [worktree]`. Built-in commands
// always win, so this only sees arguments that named no command.
func runServiceShortcut(command *cobra.Command, opt *options, args []string) error {
	name := args[0]
	loaded, err := opt.load()
	if err != nil {
		return unknownCommand(command, name, "no manifest was found to declare it as a service")
	}
	if _, exists := manifest.FindService(&loaded.Manifest, name); !exists {
		return unknownCommand(command, name, "it is not a service in this manifest")
	}
	switch len(args) {
	case 2:
		return runOverrideSet(command, opt, name, args[1])
	case 1:
	default:
		return errs.New(errs.ExitUsage, "RG1820", "usage: rungrid <service> [worktree]")
	}
	if opt.json || !inputIsTTY(command) {
		return errs.New(errs.ExitUsage, "RG1819", "rungrid "+name+" without a worktree is interactive; pass a worktree or use rungrid override set "+name+" <worktree>")
	}
	if _, err := runPicker(command, "🧩 "+name, "", serviceActions); err != nil {
		return err
	}
	return runWorktreePicker(command, opt, name)
}

// unknownCommand keeps cobra's typo suggestions, which a runnable root would
// otherwise lose.
func unknownCommand(command *cobra.Command, name, reason string) error {
	message := fmt.Sprintf("unknown command or service %q: %s", name, reason)
	if command.SuggestionsMinimumDistance <= 0 {
		command.SuggestionsMinimumDistance = 2
	}
	if suggestions := command.SuggestionsFor(name); len(suggestions) > 0 {
		message += "\n\nDid you mean this?\n\t" + strings.Join(suggestions, "\n\t")
	}
	return errs.New(errs.ExitUsage, "RG1820", message+"\n\nRun 'rungrid --help' for usage.")
}

// runWorktreePicker lists the service repository's worktrees, most recently
// updated first, and applies the chosen one as an override.
func runWorktreePicker(command *cobra.Command, opt *options, name string) error {
	ctx, cancel := overrideContext(command)
	defer cancel()
	active, loaded, err := overrideActive(ctx, opt, "; start it with rungrid up --override "+name+"=<worktree>")
	if err != nil {
		return err
	}
	repository, candidates, err := override.Candidates(ctx, loaded, active.Layout, active.Runtime.GenerationID, name, nil)
	if err != nil {
		return err
	}
	header, rows := worktreeRows(candidates)
	title := present.EmojiWorktrees + " " + repository.Name + " worktrees for " + name + " (most recently updated first, ● current)"
	index, err := runPicker(command, title, header, rows)
	if err != nil {
		return err
	}
	chosen := candidates[index]
	if chosen.Current {
		writeCommandResult(command, opt, present.GlyphOK, repository.Name+" already runs from "+chosen.Path+"; nothing changed")
		return nil
	}
	return runOverrideSet(command, opt, name, chosen.Path)
}

func worktreeRows(candidates []override.Candidate) (string, []string) {
	cells := make([][]string, len(candidates))
	for index, candidate := range candidates {
		marker := " "
		if candidate.Current {
			marker = "●"
		}
		branch := candidate.Branch
		if branch == "" {
			branch = "(detached)"
		}
		if candidate.Primary {
			branch += " (primary)"
		}
		dirty := "clean"
		if candidate.Dirty {
			dirty = "dirty"
		}
		cells[index] = []string{
			marker, branch, shortHead(candidate.HeadOID), formatPickerTime(candidate.UpdatedAt),
			formatPickerTime(candidate.CreatedAt), dirty, truncate(candidate.Subject, 48), candidate.Path,
		}
	}
	return alignColumns([]string{" ", "BRANCH", "HEAD", "UPDATED", "CREATED", "STATE", "SUBJECT", "PATH"}, cells)
}

func formatPickerTime(value time.Time) string {
	if value.IsZero() {
		return present.Dash
	}
	return value.Local().Format("2006-01-02 15:04")
}

func shortHead(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}
