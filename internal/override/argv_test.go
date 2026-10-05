package override

import (
	"reflect"
	"testing"
)

func TestRewriteArgvReanchorsOnlyPathsThatLeaveTheCheckout(t *testing.T) {
	t.Parallel()
	view := mapping{original: "/ws/svc", override: "/lanes/GH-1"}
	tests := []struct {
		name     string
		argument string
		want     string
		changed  bool
	}{
		{"plain word", "make", "make", false},
		{"dot", ".", ".", false},
		{"inside path follows override", "./cmd/server", "./cmd/server", false},
		{"nested inside path", "tooling/run.sh", "tooling/run.sh", false},
		{"absolute path is never touched", "/ws/platform/run.sh", "/ws/platform/run.sh", false},
		{"url is never touched", "http://localhost:4566/health", "http://localhost:4566/health", false},
		{"outside path pins to original", "../platform/run.sh", "/ws/platform/run.sh", true},
		{"leave and re-enter maps into override", "../svc/config.yaml", "/lanes/GH-1/config.yaml", true},
		{"flag value outside", "--file=../compose.yaml", "--file=/ws/compose.yaml", true},
		{"flag without value", "--verbose", "--verbose", false},
		{"environment assignment outside", "CONFIG=../shared/env", "CONFIG=/ws/shared/env", true},
		{"environment assignment inside", "TEMPORAL_ADDRESS=temporal:7233", "TEMPORAL_ADDRESS=temporal:7233", false},
		{"parent then back inside", "sub/../file", "sub/../file", false},
	}
	for _, test := range tests {
		got, changed := view.rewriteArgument(test.argument, "/ws/svc")
		if got != test.want || changed != test.changed {
			t.Errorf("%s: rewriteArgument(%q) = %q, %t; want %q, %t", test.name, test.argument, got, changed, test.want, test.changed)
		}
	}
}

func TestRewriteArgvFromNestedWorkingDirectory(t *testing.T) {
	t.Parallel()
	view := mapping{original: "/ws/svc", override: "/lanes/GH-1"}
	got, changes := view.rewriteArgv([]string{"go", "run", "../cmd/server", "../../shared/run.sh"}, "/ws/svc/api")
	want := []string{"go", "run", "../cmd/server", "/ws/shared/run.sh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rewriteArgv = %v, want %v", got, want)
	}
	if len(changes) != 1 || changes[0].Original != "../../shared/run.sh" || changes[0].Resolved != "/ws/shared/run.sh" {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestRewriteArgvDoesNotModifyInput(t *testing.T) {
	t.Parallel()
	view := mapping{original: "/ws/svc", override: "/lanes/GH-1"}
	input := []string{"../x"}
	_, _ = view.rewriteArgv(input, "/ws/svc")
	if input[0] != "../x" {
		t.Fatal("rewriteArgv modified its input")
	}
}

func TestMapPathRelocatesOnlyPathsInsideTheOriginalCheckout(t *testing.T) {
	t.Parallel()
	view := mapping{original: "/ws/svc", override: "/lanes/GH-1"}
	for input, want := range map[string]string{
		"/ws/svc":         "/lanes/GH-1",
		"/ws/svc/cmd/api": "/lanes/GH-1/cmd/api",
		"/ws":             "/ws",
		"/ws/svc-other":   "/ws/svc-other",
	} {
		if got := view.mapPath(input); got != want {
			t.Errorf("mapPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeRemoteComparesEquivalentLocations(t *testing.T) {
	t.Parallel()
	same := []string{
		"git@github.com:acme/svc.git",
		"https://github.com/acme/svc.git",
		"https://user:token@GitHub.com/Acme/svc",
		"ssh://git@github.com:22/acme/svc.git",
		"git@github.com:Acme/svc/",
	}
	for _, remote := range same {
		if got := normalizeRemote(remote); got != "github.com/acme/svc" {
			t.Errorf("normalizeRemote(%q) = %q", remote, got)
		}
	}
	if normalizeRemote("") != "" || normalizeRemote("/srv/git/svc.git") != "/srv/git/svc" {
		t.Fatal("empty or local remotes normalized incorrectly")
	}
}
