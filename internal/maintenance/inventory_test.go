package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
)

func TestImplicitWorkspaceSelectionRequiresWorkspaceOwnership(t *testing.T) {
	configuration := manifest.Manifest{
		Repositories: map[string]manifest.Repository{"api": {Path: "api", Remote: "origin"}},
	}
	configuration.ApplyDefaults()
	names, failures := selectedRepositoryNames(&configuration, nil)
	if len(failures) != 0 || len(names) != 1 || names[0] != "api" {
		t.Fatalf("declared-only selection = %v, failures = %v", names, failures)
	}
	configuration.Services = []manifest.Service{{Name: "control", Repository: manifest.WorkspaceRepository}}
	names, failures = selectedRepositoryNames(&configuration, nil)
	if len(failures) != 0 || len(names) != 2 || names[0] != "api" || names[1] != manifest.WorkspaceRepository {
		t.Fatalf("workspace-owned selection = %v, failures = %v", names, failures)
	}
}

func TestDiscoverDeduplicatesWorktreesFromOneRepository(t *testing.T) {
	fixture := newRepositoryFixture(t)
	feature := filepath.Join(fixture.root, "feature")
	gitTest(t, fixture.primary, "worktree", "add", "-b", "feature", feature, "main")
	loaded := loadedRepository(t, fixture.root, fixture.primary)
	relative, err := filepath.Rel(fixture.root, feature)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Manifest.Repositories["web"] = manifest.Repository{Path: relative, Remote: "origin"}
	repositories, failures := Discover(context.Background(), loaded, nil, githubRunner{})
	if len(failures) != 0 || len(repositories) != 1 {
		t.Fatalf("repositories = %#v, failures = %#v", repositories, failures)
	}
	if len(repositories[0].Aliases) != 1 || repositories[0].Aliases[0] != "web" || len(repositories[0].DeclaredPaths) != 2 {
		t.Fatalf("deduplicated repository = %#v", repositories[0])
	}
}

func TestDiscoverAcceptsDeclaredDirectoryInsideGitWorktree(t *testing.T) {
	fixture := newRepositoryFixture(t)
	subdirectory := filepath.Join(fixture.primary, "services", "api")
	if err := os.MkdirAll(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	loaded := loadedRepository(t, fixture.root, subdirectory)
	repositories, failures := Discover(context.Background(), loaded, []string{"api"}, githubRunner{})
	if len(failures) != 0 || len(repositories) != 1 {
		t.Fatalf("repositories = %#v, failures = %#v", repositories, failures)
	}
	top, _ := physicalPath(fixture.primary)
	if repositories[0].TopLevel != top || len(repositories[0].DeclaredPaths) != 1 || repositories[0].DeclaredPaths[0] != top {
		t.Fatalf("repository top-level = %#v", repositories[0])
	}
}

func TestDiscoverInfersWorkspaceRepositoriesFromServiceDirectories(t *testing.T) {
	root := t.TempDir()
	api := initializeInventoryRepository(t, root, "api")
	web := initializeInventoryRepository(t, root, "web")
	initializeInventoryRepository(t, root, "unconfigured")
	for _, directory := range []string{filepath.Join(api, "services", "one"), filepath.Join(api, "services", "two")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configuration := manifest.Manifest{
		Repositories: map[string]manifest.Repository{},
		Services: []manifest.Service{
			{Name: "api-one", WorkingDirectory: "api/services/one"},
			{Name: "api-two", WorkingDirectory: "api/services/two"},
			{Name: "web", WorkingDirectory: "web"},
		},
	}
	configuration.ApplyDefaults()
	loaded := &manifest.Loaded{Manifest: configuration, WorkspaceRoot: root, ManifestDir: api}
	repositories, failures := Discover(context.Background(), loaded, nil, githubRunner{})
	if len(failures) != 0 || len(repositories) != 2 {
		t.Fatalf("repositories = %#v, failures = %#v", repositories, failures)
	}
	byName := make(map[string]Repository, len(repositories))
	for _, repository := range repositories {
		byName[repository.Name] = repository
	}
	physicalAPI, err := physicalPath(api)
	if err != nil {
		t.Fatal(err)
	}
	physicalWeb, err := physicalPath(web)
	if err != nil {
		t.Fatal(err)
	}
	if byName["api"].TopLevel != physicalAPI || len(byName["api"].Aliases) != 0 || len(byName["api"].DeclaredPaths) != 2 {
		t.Fatalf("inferred api repository = %#v", byName["api"])
	}
	if byName["web"].TopLevel != physicalWeb || len(byName["web"].Aliases) != 0 || len(byName["web"].DeclaredPaths) != 1 {
		t.Fatalf("inferred web repository = %#v", byName["web"])
	}
	report, err := Sync(context.Background(), loaded, Options{DryRun: true}, githubRunner{}, nil)
	if err != nil || len(report.Repositories) != 2 || len(report.Failures) != 0 {
		t.Fatalf("sync report = %#v, error = %v", report, err)
	}
}

func TestDiscoverRejectsNonGitDeclaredRepository(t *testing.T) {
	root := t.TempDir()
	declared := filepath.Join(root, "api")
	if err := os.Mkdir(declared, 0o700); err != nil {
		t.Fatal(err)
	}
	loaded := loadedRepository(t, root, declared)
	repositories, failures := Discover(context.Background(), loaded, nil, githubRunner{})
	if len(repositories) != 0 || len(failures) != 1 || failures[0].Repository != "api" || failures[0].Error != errRepositoryNotWorktree.Error() {
		t.Fatalf("repositories = %#v, failures = %#v", repositories, failures)
	}
}

func initializeInventoryRepository(t *testing.T, root, name string) string {
	t.Helper()
	remote := filepath.Join(root, "."+name+".git")
	directory := filepath.Join(root, name)
	gitTest(t, root, "init", "--bare", "--initial-branch=main", remote)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, directory, "init", "--initial-branch=main")
	configureGitUser(t, directory)
	writeTestFile(t, filepath.Join(directory, "README.md"), name+"\n")
	gitTest(t, directory, "add", "README.md")
	gitTest(t, directory, "commit", "-m", "initial")
	gitTest(t, directory, "remote", "add", "origin", remote)
	gitTest(t, directory, "push", "-u", "origin", "main")
	return directory
}

func TestGitHubSlugRejectsTraversalAndUnexpectedPaths(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:../repository.git",
		"https://github.com/example/../repository.git",
		"https://github.com/example/repository/extra.git",
	} {
		if slug := githubSlug(remote); slug != "" {
			t.Errorf("githubSlug(%q) = %q", remote, slug)
		}
	}
	if slug := githubSlug("git@github.com:example/repository.git"); slug != "example/repository" {
		t.Fatalf("valid slug = %q", slug)
	}
}
