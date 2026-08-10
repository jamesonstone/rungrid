package agentexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/jamesonstone/rungrid/internal/errs"
)

type Provider string

const (
	Copilot Provider = "copilot"
	Claude  Provider = "claude"
	Warp    Provider = "warp"
	Codex   Provider = "codex"
)

var Providers = []Provider{Copilot, Claude, Warp, Codex}

type Invocation struct {
	Executable string
	Arguments  []string
	Directory  string
}

func ReconcilePrompt(target string, includeSubmodules bool) string {
	flag := ""
	if includeSubmodules {
		flag = " --include-submodules"
	}
	return fmt.Sprintf(`Safely reconcile Git repositories beneath the exact target %q.

Obey every repository's agent instructions and preservation rules. Use Rungrid's native command as the sole Git mutation primitive. First run:

rungrid reconcile %q%s --dry-run --json

Inspect the complete report and relevant repository, process, worktree, remote, and GitHub evidence. Preserve every active, ambiguous, or failed case. If and only if the native report's safety gates permit application, run:

rungrid reconcile %q%s

Do not use reset, clean, rebase, force operations, direct recursive deletion, remote-branch deletion, manual stash manipulation, or alternate Git commands to bypass a preservation decision. Finish with a concise per-repository summary and the native command's final status.`,
		target, target, flag, target, flag)
}

func Build(provider Provider, target, prompt string) (Invocation, error) {
	switch provider {
	case Copilot:
		return Invocation{Executable: "copilot", Arguments: []string{"-C", target, "-p", prompt, "--allow-all", "--no-ask-user", "--autopilot"}}, nil
	case Claude:
		return Invocation{Executable: "claude", Arguments: []string{"-p", "--dangerously-skip-permissions", prompt}, Directory: target}, nil
	case Warp:
		return Invocation{Executable: "oz", Arguments: []string{"agent", "run", "-C", target, "--prompt", prompt}}, nil
	case Codex:
		return Invocation{Executable: "codex", Arguments: []string{"exec", "-C", target, "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", prompt}}, nil
	default:
		return Invocation{}, errs.New(errs.ExitUsage, "RG1632", "unknown reconciliation agent: "+string(provider))
	}
}

func Run(ctx context.Context, invocation Invocation, stdin io.Reader, stdout, stderr io.Writer) error {
	path, err := exec.LookPath(invocation.Executable)
	if err != nil {
		return errs.Wrap(errs.ExitDependency, "RG1633", "reconciliation agent executable is unavailable", err)
	}
	command := exec.CommandContext(ctx, path, invocation.Arguments...)
	command.Dir = invocation.Directory
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			code := exitError.ExitCode()
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				code = 128 + int(status.Signal())
			}
			return errs.Wrap(code, "RG1634", "reconciliation agent failed", err)
		}
		return errs.Wrap(errs.ExitFailure, "RG1634", "reconciliation agent failed", err)
	}
	return nil
}
