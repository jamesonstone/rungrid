package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/versions"
)

// Command output gates color exactly as help output does, but never gates
// emoji: the same command must produce the same document wherever it is
// written. These cases pin both halves of that rule.
func TestPresentStyleGatesColorButNeverEmoji(t *testing.T) {
	for _, test := range []struct {
		name       string
		terminal   bool
		noColor    bool
		noColorEnv string
		wantColor  bool
	}{
		{name: "interactive", terminal: true, wantColor: true},
		{name: "interactive with --no-color", terminal: true, noColor: true},
		{name: "interactive with NO_COLOR", terminal: true, noColorEnv: "1"},
		{name: "redirected", terminal: false, wantColor: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			setTerminalHelp(t, test.terminal)
			t.Setenv("NO_COLOR", test.noColorEnv)

			var buffer bytes.Buffer
			style := presentStyle(&buffer, test.noColor)
			if style.Color != test.wantColor {
				t.Fatalf("color = %t, want %t", style.Color, test.wantColor)
			}

			snapshot := versions.Snapshot{
				Runtime: "running", Generation: "g1",
				Services: []versions.ServiceVersion{{
					Name: "api", Repository: "api", State: "Running", Health: "healthy",
					PID: 4310, Ports: []int{8080}, Branch: "main", Commit: "abc1234", GitState: "clean",
				}},
			}
			versions.WriteHuman(&buffer, style, snapshot)
			output := buffer.String()
			if hasANSI := strings.Contains(output, "\033["); hasANSI != test.wantColor {
				t.Errorf("ANSI present = %t, want %t: %q", hasANSI, test.wantColor, output)
			}
			// Emoji and box borders are content and survive every gate.
			for _, expected := range []string{present.EmojiVersions, "┌", "│"} {
				if !strings.Contains(output, expected) {
					t.Errorf("output dropped %q under %s:\n%s", expected, test.name, output)
				}
			}
		})
	}
}
