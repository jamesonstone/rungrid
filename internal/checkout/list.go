package checkout

import (
	"context"
	"sort"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

func List(ctx context.Context, layout state.Layout, loaded *manifest.Loaded, runner maintenance.Runner) (ListReport, error) {
	if runner == nil {
		runner = maintenance.CommandRunner{}
	}
	report := ListReport{Operation: "worktrees-list", StartedAt: timestamp()}
	store, err := loadStore(layout)
	if err != nil {
		return report, err
	}
	selectedByPath := map[string][]string{}
	for serviceName, selection := range store.Services {
		if path, pathErr := physicalPath(selection.Path); pathErr == nil {
			selectedByPath[path] = append(selectedByPath[path], serviceName)
		} else {
			selectedByPath[selection.Path] = append(selectedByPath[selection.Path], serviceName)
		}
	}
	for path := range selectedByPath {
		sort.Strings(selectedByPath[path])
	}
	seen := map[string]bool{}
	for index := range loaded.Manifest.Services {
		service := &loaded.Manifest.Services[index]
		if seen[service.Name] {
			continue
		}
		seen[service.Name] = true
		repository, entries, repoErr := repositoryWorktrees(ctx, loaded, service, runner)
		if repoErr != nil {
			report.Failures = append(report.Failures, Failure{Service: service.Name, Repository: serviceRepository(service), Operation: "list", Error: repoErr.Error()})
			continue
		}
		item := RepositoryList{Name: repository.Name, Remote: repository.Remote, Primary: repository.Primary}
		for _, entry := range entries {
			item.Worktrees = append(item.Worktrees, ListedWorktree{
				Path:       entry.Path,
				Branch:     entry.Branch,
				HeadOID:    entry.Head,
				Primary:    entry.Primary,
				Detached:   entry.Detached,
				SelectedBy: append([]string(nil), selectedByPath[entry.Path]...),
			})
		}
		if !repositoryListed(report.Repositories, item.Name, item.Primary) {
			report.Repositories = append(report.Repositories, item)
		}
	}
	sort.Slice(report.Repositories, func(i, j int) bool { return report.Repositories[i].Name < report.Repositories[j].Name })
	report.FinishedAt = timestamp()
	return report, nil
}

func repositoryListed(items []RepositoryList, name, primary string) bool {
	for _, item := range items {
		if item.Name == name && item.Primary == primary {
			return true
		}
	}
	return false
}
