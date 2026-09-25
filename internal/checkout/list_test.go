package checkout

import (
	"context"
	"testing"
)

func TestListIncludesPrimaryLaneAndSelection(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	loaded, layout := loadedServices(t, fixture)
	if _, err := Use(context.Background(), layout, loaded, "api", "GH-12", nil); err != nil {
		t.Fatal(err)
	}
	report, err := List(context.Background(), layout, loaded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Repositories) != 1 || len(report.Repositories[0].Worktrees) != 2 {
		t.Fatalf("unexpected list %#v", report)
	}
	foundSelected := false
	for _, worktree := range report.Repositories[0].Worktrees {
		if worktree.Branch == "GH-12" {
			foundSelected = len(worktree.SelectedBy) == 1 && worktree.SelectedBy[0] == "api"
		}
	}
	if !foundSelected {
		t.Fatalf("list did not record api selection: %#v", report)
	}
}
