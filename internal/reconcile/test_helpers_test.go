package reconcile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

type reconcileFixture struct {
	root    string
	remote  string
	primary string
	writer  string
}

func newReconcileFixture(t *testing.T) reconcileFixture {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	primary := filepath.Join(root, "primary")
	writer := filepath.Join(root, "writer")
	gitFixture(t, root, "init", "--bare", "--initial-branch=main", remote)
	gitFixture(t, root, "init", "--initial-branch=main", primary)
	configureFixtureIdentity(t, primary)
	writeFixture(t, filepath.Join(primary, "README.md"), "initial\n")
	gitFixture(t, primary, "add", "README.md")
	gitFixture(t, primary, "commit", "-m", "initial")
	gitFixture(t, primary, "remote", "add", "origin", remote)
	gitFixture(t, primary, "push", "-u", "origin", "main")
	gitFixture(t, root, "clone", remote, writer)
	configureFixtureIdentity(t, writer)
	return reconcileFixture{root: root, remote: remote, primary: primary, writer: writer}
}

func (fixture reconcileFixture) advance(t *testing.T, value string) string {
	t.Helper()
	writeFixture(t, filepath.Join(fixture.writer, "README.md"), value+"\n")
	gitFixture(t, fixture.writer, "add", "README.md")
	gitFixture(t, fixture.writer, "commit", "-m", value)
	gitFixture(t, fixture.writer, "push", "origin", "main")
	return gitFixture(t, fixture.writer, "rev-parse", "HEAD")
}

type safeRunner struct {
	maintenance.CommandRunner
	failSecretScan bool
	realLsof       bool
	mergedHead     string
}

func (runner safeRunner) Run(ctx context.Context, directory, executable string, arguments ...string) ([]byte, error) {
	joined := strings.Join(arguments, "\x00")
	switch {
	case executable == "lsof" && !runner.realLsof:
		return nil, nil
	case executable == "gitleaks" && len(arguments) != 0 && arguments[0] == "version":
		return []byte("8.29.1\n"), nil
	case executable == "gitleaks" && runner.failSecretScan:
		return nil, errors.New("secret detected")
	case executable == "gitleaks":
		return nil, nil
	case executable == "gh" && len(arguments) >= 2 && arguments[0] == "issue":
		return []byte(`{"number":1,"title":"Preserve active work"}`), nil
	case executable == "gh" && runner.mergedHead != "":
		return []byte(`[{"number":1,"state":"MERGED","mergedAt":"2026-08-01T00:00:00Z","baseRefName":"main","headRefName":"GH-1","headRefOid":"` + runner.mergedHead + `","isCrossRepository":false,"url":"https://github.com/example/repository/pull/1"}]`), nil
	case executable == "gh":
		return []byte("[]"), nil
	case executable == "git" && joined == "remote\x00get-url\x00origin":
		return []byte("git@github.com:example/repository.git\n"), nil
	default:
		return runner.CommandRunner.Run(ctx, directory, executable, arguments...)
	}
}

func gitFixture(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	content, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(arguments, " "), err, content)
	}
	return strings.TrimSpace(string(content))
}

func configureFixtureIdentity(t *testing.T, directory string) {
	t.Helper()
	gitFixture(t, directory, "config", "user.name", "Example User")
	gitFixture(t, directory, "config", "user.email", "example@example.com")
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
