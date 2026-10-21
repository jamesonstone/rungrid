//go:build darwin || linux

package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func queuedOptions(t *testing.T) (Options, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	configuration := &manifest.Manifest{Runtime: manifest.Runtime{StartupTimeout: manifest.Duration{Duration: time.Minute}}}
	return Options{
		Layout: state.Layout{ProjectDir: t.TempDir()}, Runtime: supervisor.Runtime{GenerationID: "0123456789abcdef0123"},
		Manifest: configuration, Service: "worker", Stdout: output,
	}, output
}

func recordStop(t *testing.T, options Options, at time.Time) {
	t.Helper()
	intent := filepath.Join(options.Layout.ProjectDir, "stopped", options.Runtime.GenerationID+"-"+options.Service)
	if err := os.MkdirAll(filepath.Dir(intent), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(intent, []byte("stopped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(intent, at, at); err != nil {
		t.Fatal(err)
	}
}

// Process Compose keeps a disabled tab process "Disabled" while its start
// waits for dependencies. The session must keep ownership through that wait.
func TestQueuedStartKeepsTheSessionAndAnnouncesOnce(t *testing.T) {
	options, output := queuedOptions(t)
	queued := &queuedStart{since: time.Now()}
	for range 3 {
		if action := queued.decide(options, "Disabled", false, time.Now()); action != queueWait {
			t.Fatalf("queued start action = %v, want wait", action)
		}
	}
	if strings.Count(output.String(), "waiting for its dependencies") != 1 {
		t.Fatalf("announcement = %q", output.String())
	}
	if action := queued.decide(options, "Running", false, time.Now()); action != queueNone {
		t.Fatalf("launched start action = %v", action)
	}
	if action := queued.decide(options, "Disabled", false, time.Now()); action != queueNone {
		t.Fatal("a session whose process already ran treated Disabled as a queued start")
	}
}

func TestTerminalStatusesStillEndASession(t *testing.T) {
	options, _ := queuedOptions(t)
	for _, status := range []string{"Completed", "Skipped", "Error", "Stopped"} {
		queued := &queuedStart{since: time.Now()}
		if action := queued.decide(options, status, false, time.Now()); action != queueNone || !isTerminal(status) {
			t.Errorf("%s: action %v terminal %v", status, action, isTerminal(status))
		}
	}
	queued := &queuedStart{since: time.Now()}
	if action := queued.decide(options, "Disabled", true, time.Now()); action != queueNone || !isTerminal("Disabled") {
		t.Error("Disabled after the process ran must end the session as before")
	}
}

func TestStopDuringQueuedStartStopsTheLaunchedProcess(t *testing.T) {
	options, output := queuedOptions(t)
	started := time.Now().Add(-time.Second)
	queued := &queuedStart{since: started}
	recordStop(t, options, time.Now())
	now := time.Now()
	if action := queued.decide(options, "Disabled", false, now); action != queueWait {
		t.Fatalf("stop before launch: action %v, want wait for the queued start", action)
	}
	if !strings.Contains(output.String(), "stopped before it started") {
		t.Fatalf("output = %q", output.String())
	}
	if action := queued.decide(options, "Running", false, now); action != queueStopLaunched {
		t.Fatalf("queued start launched after stop: action %v, want stop", action)
	}
	expired := &queuedStart{since: started}
	expired.decide(options, "Disabled", false, now)
	if action := expired.decide(options, "Disabled", false, now.Add(2*time.Minute)); action != queueEnd {
		t.Fatalf("after the startup timeout: action %v, want end", action)
	}
}

func TestOlderStopIntentDoesNotEndANewSession(t *testing.T) {
	options, _ := queuedOptions(t)
	recordStop(t, options, time.Now().Add(-time.Hour))
	queued := &queuedStart{since: time.Now().Add(-time.Minute)}
	if action := queued.decide(options, "Disabled", false, time.Now()); action != queueWait {
		t.Fatalf("a stop intent older than the session changed it: %v", action)
	}
}
