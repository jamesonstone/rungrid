package override

import (
	"context"
	"sort"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

// ParseFlag parses one `--override <repo|service>=<path|worktree>` value.
func ParseFlag(value string) (Request, error) {
	target, selector, found := strings.Cut(value, "=")
	target, selector = strings.TrimSpace(target), strings.TrimSpace(selector)
	if !found || target == "" || selector == "" {
		return Request{}, errs.New(errs.ExitUsage, "RG1817", "--override must be <repository|service>=<path|worktree>: "+value)
	}
	return Request{Target: target, Selector: selector, Source: SourceUpFlag}, nil
}

// Declared returns one request per repository from the manifest's service
// worktree declarations. Services sharing a repository must agree.
func Declared(loaded *manifest.Loaded) []Request {
	names := make([]string, 0, len(loaded.WorktreeDeclarations))
	for name := range loaded.WorktreeDeclarations {
		names = append(names, name)
	}
	sort.Strings(names)
	requests := make([]Request, 0, len(names))
	for _, name := range names {
		requests = append(requests, Request{Target: name, Selector: loaded.WorktreeDeclarations[name], Source: SourceManifest})
	}
	return requests
}

// PlanAll validates requests in order. Later requests for the same repository
// replace earlier ones only when their source differs, so `--override` flags
// win over declarations; two declarations or two flags that disagree about one
// repository are refused.
func PlanAll(ctx context.Context, loaded *manifest.Loaded, requests []Request, runner maintenance.Runner) ([]Planned, error) {
	runner = defaultRunner(runner)
	var result []Planned
	index := map[string]int{}
	for _, request := range requests {
		planned, err := Plan(ctx, loaded, request, runner)
		if err != nil {
			return nil, err
		}
		repository := planned.Entry.Repository
		position, seen := index[repository]
		if !seen {
			index[repository] = len(result)
			result = append(result, planned)
			continue
		}
		previous := result[position]
		if previous.Entry.Source == planned.Entry.Source && (previous.Entry.Path != planned.Entry.Path || previous.Clear != planned.Clear) {
			return nil, errs.New(errs.ExitUsage, "RG1816", "conflicting "+planned.Entry.Source+" overrides for repository "+repository+": "+previous.Entry.Path+" and "+planned.Entry.Path)
		}
		result[position] = planned
	}
	return result, nil
}

// Entries returns the entries that set an override, skipping clears.
func Entries(planned []Planned) []Entry {
	var result []Entry
	for _, item := range planned {
		if !item.Clear {
			result = append(result, item.Entry)
		}
	}
	return result
}
