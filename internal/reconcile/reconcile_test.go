package reconcile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDryRunDoesNotFetchOrWriteGitState(t *testing.T) {
	fixture := newReconcileFixture(t)
	remoteOID := fixture.advance(t, "remote")
	writeFixture(t, filepath.Join(fixture.primary, "STASHED.md"), "preserve\n")
	gitFixture(t, fixture.primary, "stash", "push", "--include-untracked", "-m", "existing")
	beforeMain := gitFixture(t, fixture.primary, "rev-parse", "refs/heads/main")
	beforeWorktrees := gitFixture(t, fixture.primary, "worktree", "list", "--porcelain")
	beforeRefs := gitFixture(t, fixture.primary, "show-ref")
	beforeIndex := gitFixture(t, fixture.primary, "write-tree")
	beforeStash := gitFixture(t, fixture.primary, "rev-parse", "refs/stash")
	beforeReadme, readErr := os.ReadFile(filepath.Join(fixture.primary, "README.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}

	report, err := Run(context.Background(), Options{Target: fixture.primary, DryRun: true, Runner: safeRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Repositories) != 1 || report.Repositories[0].Sync.Action != "would-fetch" {
		t.Fatalf("report = %#v", report)
	}
	if current := gitFixture(t, fixture.primary, "rev-parse", "refs/heads/main"); current != beforeMain || current == remoteOID {
		t.Fatalf("main changed from %s to %s", beforeMain, current)
	}
	if current := gitFixture(t, fixture.primary, "worktree", "list", "--porcelain"); current != beforeWorktrees {
		t.Fatal("worktree registrations changed during dry-run")
	}
	if current := gitFixture(t, fixture.primary, "show-ref"); current != beforeRefs {
		t.Fatal("refs changed during dry-run")
	}
	if current := gitFixture(t, fixture.primary, "write-tree"); current != beforeIndex {
		t.Fatal("index changed during dry-run")
	}
	if current := gitFixture(t, fixture.primary, "rev-parse", "refs/stash"); current != beforeStash {
		t.Fatal("stash changed during dry-run")
	}
	if current, err := os.ReadFile(filepath.Join(fixture.primary, "README.md")); err != nil || string(current) != string(beforeReadme) {
		t.Fatal("tracked filesystem state changed during dry-run")
	}
	if output := gitFixture(t, fixture.primary, "status", "--porcelain"); output != "" {
		t.Fatalf("working tree changed: %s", output)
	}
}

func TestApplyFastForwardsCleanDefault(t *testing.T) {
	fixture := newReconcileFixture(t)
	remoteOID := fixture.advance(t, "remote")

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	result := report.Repositories[0]
	if result.Sync.State != "current" || result.Sync.Action != "fast-forwarded" {
		t.Fatalf("sync = %#v", result.Sync)
	}
	if current := gitFixture(t, fixture.primary, "rev-parse", "refs/heads/main"); current != remoteOID {
		t.Fatalf("main = %s, want %s", current, remoteOID)
	}
}

func TestRecentFeatureRootIsPreservedWhileDefaultAdvances(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	remoteOID := fixture.advance(t, "remote")

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	result := report.Repositories[0]
	if result.Root.Reason != "recent-clean-feature-root" || result.Root.Action != "preserved" {
		t.Fatalf("root = %#v", result.Root)
	}
	if branch := gitFixture(t, fixture.primary, "branch", "--show-current"); branch != "GH-1" {
		t.Fatalf("branch = %s", branch)
	}
	if current := gitFixture(t, fixture.primary, "rev-parse", "refs/heads/main"); current != remoteOID {
		t.Fatalf("main = %s, want %s", current, remoteOID)
	}
}

func TestStaleDirtyGHRootCreatesWIPAndSwitches(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	writeFixture(t, filepath.Join(fixture.primary, "README.md"), "unfinished\n")
	now := time.Now().Add(96 * time.Hour)

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("err = %v, report = %#v", err, report)
	}
	root := report.Repositories[0].Root
	if root.Action != "committed-and-switched" || root.WIPCommitOID == "" {
		t.Fatalf("root = %#v", root)
	}
	if branch := gitFixture(t, fixture.primary, "branch", "--show-current"); branch != "main" {
		t.Fatalf("branch = %s", branch)
	}
	message := gitFixture(t, fixture.primary, "log", "-1", "--format=%s", "GH-1")
	if message != "wip(GH-1): :construction: work in progress: Preserve active work" {
		t.Fatalf("message = %q", message)
	}
}

func TestMergedFeatureRootSwitchesWithoutWaiting(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	head := gitFixture(t, fixture.primary, "rev-parse", "HEAD")

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{mergedHead: head}})
	if err != nil {
		t.Fatal(err)
	}
	root := report.Repositories[0].Root
	if root.Action != "switched" || root.Reason != "merged-feature-root" {
		t.Fatalf("root = %#v", root)
	}
}

func TestWIPRecoveryStagesBothSidesOfRename(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	if err := os.Rename(filepath.Join(fixture.primary, "README.md"), filepath.Join(fixture.primary, "RENAMED.md")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(96 * time.Hour)

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("err = %v, report = %#v", err, report)
	}
	files := gitFixture(t, fixture.primary, "ls-tree", "--name-only", "GH-1")
	if strings.Contains(files, "README.md") || !strings.Contains(files, "RENAMED.md") {
		t.Fatalf("renamed tree = %q", files)
	}
}

func TestPreexistingStagedWorkBlocksWIPRecovery(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	writeFixture(t, filepath.Join(fixture.primary, "README.md"), "unfinished\n")
	gitFixture(t, fixture.primary, "add", "README.md")
	now := time.Now().Add(96 * time.Hour)

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{}, Now: func() time.Time { return now }})
	if err == nil || report.Repositories[0].Root.Reason != "wip-validation-failed" {
		t.Fatalf("err = %v, report = %#v", err, report)
	}
	if staged := gitFixture(t, fixture.primary, "diff", "--cached", "--name-only"); staged != "README.md" {
		t.Fatalf("pre-existing index was changed: %q", staged)
	}
}

func TestFailedSecretScanRestoresEmptyIndex(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	writeFixture(t, filepath.Join(fixture.primary, "README.md"), "unfinished\n")
	now := time.Now().Add(96 * time.Hour)

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{failSecretScan: true}, Now: func() time.Time { return now }})
	if err == nil || len(report.Failures) == 0 {
		t.Fatalf("report = %#v, err = %v", report, err)
	}
	if staged := gitFixture(t, fixture.primary, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("staged paths remain: %s", staged)
	}
	if branch := gitFixture(t, fixture.primary, "branch", "--show-current"); branch != "GH-1" {
		t.Fatalf("branch = %s", branch)
	}
	if status := gitFixture(t, fixture.primary, "status", "--porcelain"); !strings.Contains(status, "README.md") {
		t.Fatalf("working change was not preserved: %s", status)
	}
}

func TestStaleDirtyDefaultIsStashedAndFastForwarded(t *testing.T) {
	fixture := newReconcileFixture(t)
	writeFixture(t, filepath.Join(fixture.primary, "LOCAL.md"), "local\n")
	remoteOID := fixture.advance(t, "remote")
	now := time.Now().Add(96 * time.Hour)

	report, err := Run(context.Background(), Options{Target: fixture.primary, Runner: safeRunner{}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	root := report.Repositories[0].Root
	if root.Action != "stashed" || root.StashOID == "" {
		t.Fatalf("root = %#v", root)
	}
	if current := gitFixture(t, fixture.primary, "rev-parse", "refs/heads/main"); current != remoteOID {
		t.Fatalf("main = %s, want %s", current, remoteOID)
	}
	if stash := gitFixture(t, fixture.primary, "rev-parse", "refs/stash"); stash != root.StashOID {
		t.Fatalf("stash = %s, want %s", stash, root.StashOID)
	}
}

func TestAheadDefaultBlocksFeatureRootSwitch(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "commit", "--allow-empty", "-m", "local main")
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")

	report, err := Run(context.Background(), Options{
		Target: fixture.primary, Runner: safeRunner{},
		Now: func() time.Time { return time.Now().Add(96 * time.Hour) },
	})
	if err == nil {
		t.Fatalf("expected partial failure: %#v", report)
	}
	result := report.Repositories[0]
	if result.Sync.State != "ahead" || result.Root.Reason != "default-not-current" {
		t.Fatalf("result = %#v", result)
	}
	if branch := gitFixture(t, fixture.primary, "branch", "--show-current"); branch != "GH-1" {
		t.Fatalf("branch = %s", branch)
	}
}

func TestReportIncludesVersionedActivityAndOIDState(t *testing.T) {
	fixture := newReconcileFixture(t)
	report, err := Run(context.Background(), Options{Target: fixture.primary, DryRun: true, Runner: safeRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	root := report.Repositories[0].Root
	if root.HeadOID == "" || root.HeadCommitAt == "" || root.HeadReflogAt == "" {
		t.Fatalf("activity evidence = %#v", root)
	}
	content, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(content), `"local_oid"`) || !strings.Contains(string(content), `"activity_at"`) {
		t.Fatalf("json = %s, err = %v", content, err)
	}
}

func TestRecursiveReconcileContinuesAcrossRepositoryFailure(t *testing.T) {
	fixture := newReconcileFixture(t)
	scanRoot := t.TempDir()
	good := filepath.Join(scanRoot, "good")
	bad := filepath.Join(scanRoot, "bad")
	gitFixture(t, scanRoot, "clone", fixture.remote, good)
	configureFixtureIdentity(t, good)
	gitFixture(t, scanRoot, "init", "--initial-branch=main", bad)
	configureFixtureIdentity(t, bad)
	writeFixture(t, filepath.Join(bad, "README.md"), "no remote\n")
	gitFixture(t, bad, "add", "README.md")
	gitFixture(t, bad, "commit", "-m", "initial")

	report, err := Run(context.Background(), Options{Target: scanRoot, DryRun: true, Runner: safeRunner{}})
	if err == nil || len(report.Repositories) != 2 || len(report.Failures) == 0 {
		t.Fatalf("err = %v, report = %#v", err, report)
	}
	foundGood := false
	for _, repository := range report.Repositories {
		if repository.Name == "good" && repository.Sync.State == "current" {
			foundGood = true
		}
	}
	if !foundGood {
		t.Fatalf("independent repository was not completed: %#v", report.Repositories)
	}
}
