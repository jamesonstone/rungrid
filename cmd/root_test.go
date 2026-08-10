package cmd

import (
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
)

func TestRootExposesV1Commands(t *testing.T) {
	t.Parallel()
	root := newRootCommand()
	commands := map[string]bool{}
	for _, command := range root.Commands() {
		if !command.Hidden {
			commands[command.Name()] = true
		}
	}
	for _, expected := range []string{
		"init", "instructions", "doctor", "plan", "generate", "up", "open", "attach", "versions",
		"status", "logs", "session", "start", "stop", "down", "uninstall", "config",
		"sync", "reconcile", "worktrees",
		"completion", "version",
	} {
		if !commands[expected] {
			t.Errorf("missing command %q", expected)
		}
	}
}

func TestRedactManifestCoversLifecycleEnvironment(t *testing.T) {
	t.Parallel()
	configuration := manifest.Manifest{
		Lifecycle: manifest.Lifecycle{
			BeforeUp: []manifest.LifecycleCommand{{
				Name: "prepare", Environment: manifest.Environment{Values: map[string]string{"VALUE": "private"}},
			}},
		},
	}
	redacted := redactManifest(configuration)
	if redacted.Lifecycle.BeforeUp[0].Environment.Values["VALUE"] != "<redacted>" {
		t.Fatal("lifecycle environment value was not redacted")
	}
	if configuration.Lifecycle.BeforeUp[0].Environment.Values["VALUE"] != "private" {
		t.Fatal("redaction mutated the source manifest")
	}
}
