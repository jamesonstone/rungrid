package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/versions"
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
