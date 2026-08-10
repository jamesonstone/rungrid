package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFilesystemInsideLinkedWorktreeDeduplicatesClone(t *testing.T) {
	fixture := newRepositoryFixture(t)
	linked := filepath.Join(fixture.root, "linked")
	gitTest(t, fixture.primary, "worktree", "add", "-b", "GH-1", linked)

	repositories, failures := DiscoverFilesystem(context.Background(), linked, false, githubRunner{})
	if len(failures) != 0 || len(repositories) != 1 {
		t.Fatalf("repositories = %#v, failures = %#v", repositories, failures)
	}
	repository := repositories[0]
	expectedPrimary, err := filepath.EvalSymlinks(fixture.primary)
	if err != nil {
		t.Fatal(err)
	}
	if repository.Primary != expectedPrimary || repository.TopLevel != expectedPrimary || repository.Name != filepath.Base(fixture.primary) {
		t.Fatalf("repository = %#v", repository)
	}
	expectedLinked, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.DeclaredPaths) != 1 || repository.DeclaredPaths[0] != expectedLinked {
		t.Fatalf("protected paths = %#v", repository.DeclaredPaths)
	}
}

func TestDiscoverFilesystemRecursiveDeduplicatesCommonDirectory(t *testing.T) {
	fixture := newRepositoryFixture(t)
	linked := filepath.Join(fixture.root, "linked")
	gitTest(t, fixture.primary, "worktree", "add", "-b", "GH-1", linked)

	repositories, failures := DiscoverFilesystem(context.Background(), fixture.root, false, githubRunner{})
	if len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
	common := repositoriesWithCommonDir(repositories, fixture.primary)
	if common != 1 {
		t.Fatalf("physical clone discovered %d times: %#v", common, repositories)
	}
}

func repositoriesWithCommonDir(repositories []Repository, checkout string) int {
	common, _ := gitText(context.Background(), CommandRunner{}, checkout, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(common) {
		common = filepath.Join(checkout, common)
	}
	common, _ = filepath.EvalSymlinks(common)
	count := 0
	for _, repository := range repositories {
		if repository.CommonDir == common {
			count++
		}
	}
	return count
}

func TestDiscoverFilesystemRecursiveDoesNotFollowDirectorySymlinks(t *testing.T) {
	fixture := newRepositoryFixture(t)
	scanRoot := t.TempDir()
	if err := os.Symlink(fixture.primary, filepath.Join(scanRoot, "repository-link")); err != nil {
		t.Fatal(err)
	}

	repositories, failures := DiscoverFilesystem(context.Background(), scanRoot, false, githubRunner{})
	if len(failures) != 0 || len(repositories) != 0 {
		t.Fatalf("repositories = %#v, failures = %#v", repositories, failures)
	}
}

func TestDiscoverFilesystemSkipsSubmodulesUnlessIncluded(t *testing.T) {
	fixture := newRepositoryFixture(t)
	childRemote := filepath.Join(fixture.root, "child.git")
	childSource := filepath.Join(fixture.root, "child-source")
	gitTest(t, fixture.root, "init", "--bare", "--initial-branch=main", childRemote)
	gitTest(t, fixture.root, "init", "--initial-branch=main", childSource)
	configureGitUser(t, childSource)
	writeTestFile(t, filepath.Join(childSource, "README.md"), "child\n")
	gitTest(t, childSource, "add", "README.md")
	gitTest(t, childSource, "commit", "-m", "initial child")
	gitTest(t, childSource, "remote", "add", "origin", childRemote)
	gitTest(t, childSource, "push", "-u", "origin", "main")
	gitTest(t, fixture.primary, "-c", "protocol.file.allow=always", "submodule", "add", childRemote, "child")
	gitTest(t, fixture.primary, "commit", "-m", "add child")

	repositories, failures := DiscoverFilesystem(context.Background(), fixture.root, false, githubRunner{})
	if len(failures) != 0 || len(repositories) != 3 {
		// Primary, writer, and child source are independent clones; the submodule is skipped.
		t.Fatalf("repositories = %d, failures = %#v", len(repositories), failures)
	}
	included, includeFailures := DiscoverFilesystem(context.Background(), fixture.root, true, githubRunner{})
	if len(includeFailures) != 0 || len(included) != 4 {
		t.Fatalf("included repositories = %d, failures = %#v", len(included), includeFailures)
	}
}
