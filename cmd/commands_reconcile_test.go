package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/rungrid/internal/agentexec"
	"github.com/jamesonstone/rungrid/internal/errs"
)

func TestReconcileAgentRejectsNativeOutputModes(t *testing.T) {
	for _, arguments := range [][]string{
		{"--json", "reconcile", "--agent=codex"},
		{"reconcile", "--dry-run", "--agent=codex"},
	} {
		root := newRootCommand()
		root.SetArgs(arguments)
		if err := root.Execute(); errs.Code(err) != errs.ExitUsage {
			t.Fatalf("args = %#v, code = %d, err = %v", arguments, errs.Code(err), err)
		}
	}
}

func TestReconcileAgentSelectionFailsWithoutTTY(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := newRootCommand()
	root.SetIn(bytes.NewBufferString("1\n"))
	root.SetArgs([]string{"reconcile", "--agent=select"})
	if err := root.Execute(); errs.Code(err) != errs.ExitUsage {
		t.Fatalf("code = %d, err = %v", errs.Code(err), err)
	}
}

func TestReconcileFZFSelection(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "fzf")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'claude\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := newReconcileCommand(&options{})
	command.SetContext(context.Background())
	provider, err := resolveAgent(command, "select")
	if err != nil || provider != agentexec.Claude {
		t.Fatalf("provider = %q, err = %v", provider, err)
	}
}
