package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

var issueBranchPattern = regexp.MustCompile(`^GH-([1-9][0-9]*)$`)

func exactMergedPR(ctx context.Context, repository maintenance.Repository, branch, defaultBranch, headOID string, runner maintenance.Runner) (bool, error) {
	if repository.RemoteSlug == "" {
		return false, fmt.Errorf("origin is not a supported GitHub repository")
	}
	content, err := runner.Run(ctx, repository.Primary, "gh", "pr", "list", "--repo", repository.RemoteSlug,
		"--state", "all", "--limit", "100", "--head", branch, "--json",
		"number,state,mergedAt,baseRefName,headRefName,headRefOid,isCrossRepository,url")
	if err != nil {
		return false, fmt.Errorf("inspect pull request: %w", err)
	}
	var requests []maintenance.PullRequest
	if err := json.Unmarshal(content, &requests); err != nil {
		return false, fmt.Errorf("decode pull requests: %w", err)
	}
	var exact []maintenance.PullRequest
	for _, request := range requests {
		if request.HeadRefName == branch {
			exact = append(exact, request)
		}
	}
	if len(exact) != 1 {
		return false, nil
	}
	request := exact[0]
	return !request.IsCrossRepository && strings.EqualFold(request.State, "MERGED") && request.MergedAt != nil &&
		request.BaseRefName == defaultBranch && request.HeadRefOID == headOID, nil
}

func validateWIP(ctx context.Context, repository maintenance.Repository, state rootState, runner maintenance.Runner) (string, []string, error) {
	match := issueBranchPattern.FindStringSubmatch(state.branch)
	if len(match) != 2 {
		return "", nil, fmt.Errorf("dirty primary branch must match GH-<number>")
	}
	if state.staged {
		return "", nil, fmt.Errorf("primary index already contains staged changes")
	}
	if state.conflicted {
		return "", nil, fmt.Errorf("primary contains unmerged paths")
	}
	if len(state.ignored) != 0 {
		return "", nil, fmt.Errorf("primary contains ignored material")
	}
	if repository.RemoteSlug == "" {
		return "", nil, fmt.Errorf("origin is not a supported GitHub repository")
	}
	if err := validateIdentity(ctx, repository.Primary, runner); err != nil {
		return "", nil, err
	}
	if err := validateSafePaths(ctx, repository.Primary, state.dirtyPaths, runner); err != nil {
		return "", nil, err
	}
	if _, err := runner.Run(ctx, repository.Primary, "gitleaks", "version"); err != nil {
		return "", nil, fmt.Errorf("gitleaks is required for WIP recovery: %w", err)
	}
	content, err := runner.Run(ctx, repository.Primary, "gh", "issue", "view", match[1], "--repo", repository.RemoteSlug, "--json", "number,title")
	if err != nil {
		return "", nil, fmt.Errorf("resolve issue %s: %w", state.branch, err)
	}
	var issue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(content, &issue); err != nil || issue.Number == 0 || sanitizeTitle(issue.Title) == "" {
		return "", nil, fmt.Errorf("decode issue %s", state.branch)
	}
	expectedNumber, _ := strconv.Atoi(match[1])
	if issue.Number != expectedNumber {
		return "", nil, fmt.Errorf("issue response does not match %s", state.branch)
	}
	return sanitizeTitle(issue.Title), append([]string(nil), state.dirtyPaths...), nil
}

func validateIdentity(ctx context.Context, directory string, runner maintenance.Runner) error {
	name, nameErr := gitText(ctx, runner, directory, "config", "user.name")
	email, emailErr := gitText(ctx, runner, directory, "config", "user.email")
	author, authorErr := gitText(ctx, runner, directory, "var", "GIT_AUTHOR_IDENT")
	committer, committerErr := gitText(ctx, runner, directory, "var", "GIT_COMMITTER_IDENT")
	if nameErr != nil || emailErr != nil || authorErr != nil || committerErr != nil || name == "" || email == "" {
		return fmt.Errorf("human Git author and committer identity is unavailable")
	}
	identity := strings.ToLower(name + " " + email + " " + author + " " + committer)
	for _, marker := range []string{"[bot]", "copilot", "codex", "claude", " coding agent", " bot "} {
		if strings.Contains(identity, marker) {
			return fmt.Errorf("git identity appears automated")
		}
	}
	return nil
}

func validateSafePaths(ctx context.Context, root string, paths []string, runner maintenance.Runner) error {
	submodules, err := submodulePaths(ctx, root, runner)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if unsafeRecoveryName(path) {
			return fmt.Errorf("unsafe recovery path: %s", path)
		}
		for _, submodule := range submodules {
			if path == submodule || strings.HasPrefix(path, submodule+"/") {
				return fmt.Errorf("submodule recovery path is not allowed: %s", path)
			}
		}
		absolute := filepath.Join(root, filepath.FromSlash(path))
		info, statErr := os.Lstat(absolute)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("inspect recovery path %s: %w", path, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, resolveErr := filepath.EvalSymlinks(absolute)
			if resolveErr != nil || !pathInside(root, target) {
				return fmt.Errorf("unsafe recovery symlink: %s", path)
			}
		}
	}
	return nil
}

func unsafeRecoveryName(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == ".env" || strings.HasPrefix(base, ".env.") || base == "credentials" ||
		base == "credentials.json" || base == "id_rsa" || base == "id_ed25519" {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	default:
		return false
	}
}

func submodulePaths(ctx context.Context, root string, runner maintenance.Runner) ([]string, error) {
	content, err := runner.Run(ctx, root, "git", "ls-files", "--stage", "-z")
	if err != nil {
		return nil, fmt.Errorf("inspect submodule paths: %w", err)
	}
	var result []string
	for _, record := range strings.Split(string(content), "\x00") {
		metadata, path, found := strings.Cut(record, "\t")
		if found && strings.HasPrefix(metadata, "160000 ") {
			result = append(result, filepath.ToSlash(path))
		}
	}
	return result, nil
}

func commitWIP(ctx context.Context, repository maintenance.Repository, state rootState, issueTitle string, paths []string, runner maintenance.Runner) (string, error) {
	arguments := append([]string{"add", "--"}, paths...)
	if _, err := runner.Run(ctx, repository.Primary, "git", arguments...); err != nil {
		_ = unstageExact(ctx, repository.Primary, paths, runner)
		return "", fmt.Errorf("stage WIP paths: %w", err)
	}
	if _, err := runner.Run(ctx, repository.Primary, "gitleaks", "git", "--staged", "--redact", "--no-banner"); err != nil {
		return "", fmt.Errorf("scan staged WIP paths: %w", joinErrors(err, unstageExact(ctx, repository.Primary, paths, runner)))
	}
	message := fmt.Sprintf("wip(%s): :construction: work in progress: %s", state.branch, issueTitle)
	if _, err := runner.Run(ctx, repository.Primary, "git", "commit", "-m", message); err != nil {
		return "", fmt.Errorf("commit WIP paths: %w", joinErrors(err, unstageExact(ctx, repository.Primary, paths, runner)))
	}
	return gitText(ctx, runner, repository.Primary, "rev-parse", "HEAD")
}

func unstageExact(ctx context.Context, root string, paths []string, runner maintenance.Runner) error {
	arguments := append([]string{"restore", "--staged", "--"}, paths...)
	_, restoreErr := runner.Run(ctx, root, "git", arguments...)
	staged, inspectErr := gitPaths(ctx, runner, root, "diff", "--cached", "--name-only", "-z")
	if restoreErr != nil || inspectErr != nil || len(staged) != 0 {
		return fmt.Errorf("restore previously empty index: %v; inspect: %v", restoreErr, inspectErr)
	}
	return nil
}
