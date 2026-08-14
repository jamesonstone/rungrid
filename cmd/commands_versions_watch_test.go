package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/versions"
	"github.com/spf13/cobra"
)

func TestVersionsWatchDisplayUsesAlternateScreen(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	snapshot := versions.Snapshot{CapturedAt: "2026-08-12T16:00:00Z", Generation: "generation-1"}
	var human bytes.Buffer
	versions.WriteHuman(&human, snapshot)

	display := newVersionsWatchDisplay(&output, true)
	display.open()
	display.render(snapshot)
	display.render(snapshot)
	display.close()
	display.close()

	want := versionsWatchEnter + versionsWatchRedraw + human.String() +
		versionsWatchRedraw + human.String() + versionsWatchExit
	if output.String() != want {
		t.Fatalf("alternate-screen output mismatch:\nwant %q\n got %q", want, output.String())
	}
}

func TestVersionsWatchDisplayLeavesRedirectedOutputPlain(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	snapshot := versions.Snapshot{CapturedAt: "2026-08-12T16:00:00Z", Generation: "generation-1"}
	display := newVersionsWatchDisplay(&output, false)

	display.open()
	display.render(snapshot)
	display.close()

	if strings.Contains(output.String(), "\033[") {
		t.Fatalf("redirected watch output contained ANSI controls: %q", output.String())
	}
	if !strings.HasPrefix(output.String(), "Rungrid Versions") {
		t.Fatalf("redirected watch output omitted the table: %q", output.String())
	}
}

func TestVersionsWatchExitsWhenRuntimeBecomesInactive(t *testing.T) {
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetContext(context.Background())
	command.SetOut(&output)
	active := lifecycle.Active{}
	checked := false

	err := watchVersionsWhileRuntimeActive(command, active, func(state.Layout, supervisor.Runtime) bool {
		checked = true
		return false
	})
	if err != nil || !checked || output.Len() != 0 {
		t.Fatalf("inactive watch did not exit cleanly: checked=%t output=%q err=%v", checked, output.String(), err)
	}
}

func TestVersionsRuntimeActiveRejectsShutdownMarker(t *testing.T) {
	layout, err := state.NewLayout("versions-watch-r4k2m7", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	runtimeState := supervisor.Runtime{GenerationID: "generation"}
	marker := filepath.Join("locks", "down-generation.json")
	if err := state.WriteFileAtomic(layout.ProjectDir, marker, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if versionsRuntimeActive(layout, runtimeState) {
		t.Fatal("Versions watch accepted a generation whose shutdown had begun")
	}
}
