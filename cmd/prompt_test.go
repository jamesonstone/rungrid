package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/spf13/cobra"
)

func TestParseChoiceAndChoices(t *testing.T) {
	t.Parallel()
	index, err := parseChoice("2", 3)
	if err != nil || index != 1 {
		t.Fatalf("parseChoice = %d, %v", index, err)
	}
	if _, err := parseChoice("0", 3); errs.Code(err) != errs.ExitUsage {
		t.Fatalf("expected out-of-range, got %v", err)
	}
	indexes, err := parseChoices("1,3", 3)
	if err != nil || len(indexes) != 2 || indexes[0] != 0 || indexes[1] != 2 {
		t.Fatalf("parseChoices = %#v, %v", indexes, err)
	}
	all, err := parseChoices("all", 2)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %#v, %v", all, err)
	}
}

func TestRequireInteractiveFailsClosed(t *testing.T) {
	t.Parallel()
	command := &cobra.Command{}
	command.SetIn(bytes.NewBufferString("1\n"))
	if err := requireInteractive(command, &options{json: true}, "needs a TTY"); errs.Code(err) != errs.ExitUsage {
		t.Fatalf("json: %v", err)
	}
	if err := requireInteractive(command, &options{}, "needs a TTY"); errs.Code(err) != errs.ExitUsage {
		t.Fatalf("pipe: %v", err)
	}
}

func TestWorktreesInteractiveCommandsFailClosed(t *testing.T) {
	for _, arguments := range [][]string{
		{"worktrees"},
		{"worktrees", "use"},
		{"worktrees", "use", "--json"},
		{"worktrees", "--json"},
	} {
		root := newRootCommand()
		root.SetIn(bytes.NewBufferString("1\n"))
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(arguments)
		if err := root.Execute(); errs.Code(err) != errs.ExitUsage {
			t.Fatalf("args = %s, code = %d, err = %v", strings.Join(arguments, " "), errs.Code(err), err)
		}
	}
}
