package override

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

// Request names a repository or service and the checkout to run it from.
type Request struct {
	Target   string
	Selector string
	Source   string
}

// Planned is a validated override. Clear is true when the selector names the
// original checkout, which removes any override instead of setting one.
type Planned struct {
	Entry    Entry
	Clear    bool
	Service  string
	Warnings []string
}

// Plan validates one request without touching project state or processes.
func Plan(ctx context.Context, loaded *manifest.Loaded, request Request, runner maintenance.Runner) (Planned, error) {
	runner = defaultRunner(runner)
	repository, service, err := FindTarget(loaded, Repositories(ctx, loaded, runner), request.Target)
	if err != nil {
		return Planned{}, err
	}
	planned := Planned{Service: service, Entry: Entry{
		Repository: repository.Name, OriginalPath: repository.TopLevel,
		Services: append([]string(nil), repository.Managed...), Source: request.Source,
		Reanchored: []Reanchor{},
	}}
	if service != "" && len(repository.Managed) > 1 {
		planned.Warnings = append(planned.Warnings, "service "+service+" resolves to repository "+repository.Name+"; the override applies to "+strings.Join(repository.Managed, ", "))
	}
	if len(repository.External) > 0 {
		planned.Warnings = append(planned.Warnings, "external services keep their own location: "+strings.Join(repository.External, ", "))
	}
	path, clear, err := resolveSelector(ctx, runner, repository, request.Selector)
	if err != nil {
		return Planned{}, err
	}
	if clear || path == repository.TopLevel {
		planned.Clear = true
		planned.Entry.Path = repository.TopLevel
		return planned, nil
	}
	if err := sameRepository(ctx, runner, repository, path); err != nil {
		return Planned{}, err
	}
	planned.Entry.Path = path
	planned.Entry.Branch, planned.Entry.HeadOID = headIdentity(ctx, runner, path)
	if status, statusErr := gitText(ctx, runner, path, "status", "--porcelain"); statusErr == nil && status != "" {
		planned.Entry.Dirty = true
		planned.Warnings = append(planned.Warnings, "override checkout has uncommitted changes: "+path)
	}
	view := mapping{original: repository.TopLevel, override: path}
	for _, name := range repository.Managed {
		target, _ := manifest.FindService(&loaded.Manifest, name)
		changes, checkErr := checkService(loaded, target, view)
		if checkErr != nil {
			return Planned{}, checkErr
		}
		planned.Entry.Reanchored = append(planned.Entry.Reanchored, changes...)
		if target.Compose != nil && target.Compose.ProjectName == "" {
			planned.Warnings = append(planned.Warnings, "compose service "+name+" has no project_name; Compose derives one from the directory, so the override may get new volumes and containers")
		}
	}
	return planned, nil
}

// resolveSelector turns "primary", a path, a branch, or a worktree directory
// base name into the top level of a checkout.
func resolveSelector(ctx context.Context, runner maintenance.Runner, repository Repository, selector string) (string, bool, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", false, errs.New(errs.ExitUsage, "RG1812", "a worktree path, branch, or name is required")
	}
	if strings.EqualFold(selector, "primary") {
		return "", true, nil
	}
	if looksLikePath(selector) {
		return checkoutAt(ctx, runner, expandHome(selector))
	}
	worktrees, err := listWorktrees(ctx, runner, repository.TopLevel)
	if err != nil {
		return "", false, errs.Wrap(errs.ExitConflict, "RG1809", "list worktrees of "+repository.Name, err)
	}
	var matches []string
	for _, worktree := range worktrees {
		if worktree.branch == selector || filepath.Base(worktree.path) == selector {
			matches = append(matches, worktree.path)
		}
	}
	switch len(matches) {
	case 1:
		return checkoutAt(ctx, runner, matches[0])
	case 0:
		return "", false, errs.New(errs.ExitUsage, "RG1812", "no worktree of "+repository.Name+" has branch or directory "+selector+"; pass a path instead")
	default:
		return "", false, errs.New(errs.ExitUsage, "RG1811", "worktree selector "+selector+" is ambiguous in "+repository.Name+": "+strings.Join(matches, ", "))
	}
}

func checkoutAt(ctx context.Context, runner maintenance.Runner, path string) (string, bool, error) {
	resolved, err := physicalPath(path)
	if err != nil {
		return "", false, errs.Wrap(errs.ExitUsage, "RG1808", "override path does not exist: "+path, err)
	}
	if info, statErr := os.Stat(resolved); statErr != nil || !info.IsDir() {
		return "", false, errs.New(errs.ExitUsage, "RG1808", "override path is not a directory: "+path)
	}
	top, err := topLevel(ctx, runner, resolved)
	if err != nil {
		return "", false, errs.Wrap(errs.ExitConflict, "RG1809", "override path is not a Git checkout: "+path, err)
	}
	return top, false, nil
}

// sameRepository accepts any checkout sharing the original Git common
// directory, such as a registered worktree, or a separate clone whose origin
// URL normalizes to the same location.
func sameRepository(ctx context.Context, runner maintenance.Runner, repository Repository, path string) error {
	if common, err := commonDir(ctx, runner, path); err == nil && repository.CommonDir != "" && common == repository.CommonDir {
		return nil
	}
	origin, _ := gitText(ctx, runner, path, "remote", "get-url", "origin")
	left, right := normalizeRemote(repository.Origin), normalizeRemote(origin)
	if left != "" && left == right {
		return nil
	}
	return errs.New(errs.ExitConflict, "RG1810", path+" is not a checkout of repository "+repository.Name+" (Git common directory and origin both differ)")
}

func headIdentity(ctx context.Context, runner maintenance.Runner, path string) (string, string) {
	head, _ := gitText(ctx, runner, path, "rev-parse", "HEAD")
	branch, err := gitText(ctx, runner, path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		branch = ""
	}
	return branch, head
}

func looksLikePath(selector string) bool {
	return strings.ContainsRune(selector, filepath.Separator) || strings.HasPrefix(selector, "~") ||
		selector == "." || selector == ".." || filepath.IsAbs(selector)
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// normalizeRemote reduces SSH, scp-like, HTTPS, and local remotes to a
// comparable host/path form without credentials, scheme, or .git suffix.
func normalizeRemote(remote string) string {
	value := strings.TrimSpace(remote)
	if value == "" {
		return ""
	}
	value = strings.TrimSuffix(strings.TrimSuffix(value, "/"), ".git")
	if index := strings.Index(value, "://"); index >= 0 {
		value = value[index+3:]
		if at := strings.LastIndex(strings.SplitN(value, "/", 2)[0], "@"); at >= 0 {
			value = value[at+1:]
		}
	} else if at := strings.Index(value, "@"); at >= 0 && strings.Contains(value, ":") && !filepath.IsAbs(value) {
		value = strings.Replace(value[at+1:], ":", "/", 1)
	} else if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	parts := strings.SplitN(value, "/", 2)
	host := strings.ToLower(parts[0])
	if colon := strings.IndexByte(host, ':'); colon >= 0 {
		host = host[:colon]
	}
	if len(parts) == 1 {
		return host
	}
	return host + "/" + strings.ToLower(parts[1])
}
