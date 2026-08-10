package versions

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/subprocess"
)

type sourceVersion struct {
	branch   string
	commit   string
	gitState string
	worktree string
	root     string
}

func gitVersion(ctx context.Context, directory string) (branch, commit, gitState, worktree string) {
	version := captureGitVersion(ctx, directory, "")
	return version.branch, version.commit, version.gitState, version.worktree
}

func captureGitVersion(ctx context.Context, directory, knownRoot string) sourceVersion {
	root := knownRoot
	if root == "" {
		root = runGit(ctx, directory, "rev-parse", "--show-toplevel")
	}
	command := exec.CommandContext(ctx, "git", "-C", directory, "status", "--porcelain=v2", "--branch", "--untracked-files=normal")
	result, err := subprocess.Run(command)
	if err != nil {
		return sourceVersion{gitState: "unavailable", root: root, worktree: worktreeName(root)}
	}
	version := parseGitStatus(string(result.Stdout))
	version.root, version.worktree = root, worktreeName(root)
	return version
}

func parseGitStatus(content string) sourceVersion {
	result := sourceVersion{gitState: "clean"}
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			result.commit = strings.TrimPrefix(line, "# branch.oid ")
			if len(result.commit) > 7 {
				result.commit = result.commit[:7]
			}
		case strings.HasPrefix(line, "# branch.head "):
			result.branch = strings.TrimPrefix(line, "# branch.head ")
			if result.branch == "(detached)" {
				result.branch = ""
			}
		case line != "" && !strings.HasPrefix(line, "# "):
			result.gitState = "dirty"
		}
	}
	if result.branch == "" && result.commit == "" {
		result.gitState = "unavailable"
	}
	return result
}

func runGit(ctx context.Context, directory string, arguments ...string) string {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, arguments...)...)
	result, err := subprocess.Run(command)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(result.Stdout))
}

func worktreeName(root string) string {
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}
