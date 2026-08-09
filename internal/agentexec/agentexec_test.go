package agentexec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
)

func TestBuildUsesExactUnattendedProviderArguments(t *testing.T) {
	target, prompt := "/workspace/root", "reconcile safely"
	tests := []struct {
		provider Provider
		want     Invocation
	}{
		{Copilot, Invocation{Executable: "copilot", Arguments: []string{"-C", target, "-p", prompt, "--allow-all", "--no-ask-user", "--autopilot"}}},
		{Claude, Invocation{Executable: "claude", Arguments: []string{"-p", "--dangerously-skip-permissions", prompt}, Directory: target}},
		{Warp, Invocation{Executable: "oz", Arguments: []string{"agent", "run", "-C", target, "--prompt", prompt}}},
		{Codex, Invocation{Executable: "codex", Arguments: []string{"exec", "-C", target, "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", prompt}}},
	}
	for _, test := range tests {
		t.Run(string(test.provider), func(t *testing.T) {
			got, err := Build(test.provider, target, prompt)
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("invocation = %#v, err = %v, want %#v", got, err, test.want)
			}
		})
	}
}

func TestReconcilePromptConfinesMutationToNativeCommand(t *testing.T) {
	prompt := ReconcilePrompt("/workspace/root", true)
	for _, expected := range []string{
		"rungrid reconcile \"/workspace/root\" --include-submodules --dry-run --json",
		"rungrid reconcile \"/workspace/root\" --include-submodules",
		"sole Git mutation primitive",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt does not contain %q:\n%s", expected, prompt)
		}
	}
}

func TestRunPassesThroughOutputAndProviderExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'provider stdout'\nprintf 'provider stderr' >&2\nexit 17\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), Invocation{Executable: "fake-agent"}, strings.NewReader(""), &stdout, &stderr)
	if errs.Code(err) != 17 || stdout.String() != "provider stdout" || stderr.String() != "provider stderr" {
		t.Fatalf("code = %d, stdout = %q, stderr = %q, err = %v", errs.Code(err), stdout.String(), stderr.String(), err)
	}
}

func TestRunRejectsMissingProviderBeforeLaunch(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Run(context.Background(), Invocation{Executable: "missing-agent"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if errs.Code(err) != errs.ExitDependency {
		t.Fatalf("code = %d, err = %v", errs.Code(err), err)
	}
}

func TestRunPropagatesProviderSignalStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "signaled-agent")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nkill -TERM $$\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := Run(context.Background(), Invocation{Executable: "signaled-agent"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if errs.Code(err) != 143 {
		t.Fatalf("code = %d, err = %v", errs.Code(err), err)
	}
}
