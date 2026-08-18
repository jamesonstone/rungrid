package present

import (
	"bytes"
	"strings"
	"testing"
)

func TestColorlessStyleEmitsNoANSI(t *testing.T) {
	t.Parallel()
	style := New(false)
	var buffer bytes.Buffer
	if err := style.Header(&buffer, EmojiServices, "Services"); err != nil {
		t.Fatal(err)
	}
	if err := style.Field(&buffer, "generation", "8f3c1a2b"); err != nil {
		t.Fatal(err)
	}
	table := style.NewTable("SERVICE", "STATUS", "PID")
	table.Row(GlyphRunning+" api", "running", "41234")
	table.Row(GlyphIdle+" web", "inactive", "")
	if err := table.Render(&buffer, "no services"); err != nil {
		t.Fatal(err)
	}
	output := buffer.String()
	if strings.Contains(output, "\033[") {
		t.Fatalf("colorless output contained ANSI escapes: %q", output)
	}
	for _, expected := range []string{EmojiServices, GlyphRunning, "running", "┌", "│", "└", Dash} {
		if !strings.Contains(output, expected) {
			t.Errorf("colorless output missing %q\n%s", expected, output)
		}
	}
}

func TestColorStyleWrapsEmphasis(t *testing.T) {
	t.Parallel()
	style := New(true)
	if plain := New(false).Bold("Services"); plain != "Services" {
		t.Fatalf("colorless bold changed text: %q", plain)
	}
	if colored := style.Bold("Services"); !strings.Contains(colored, WhiteBold) || !strings.Contains(colored, Reset) {
		t.Fatalf("colored bold missing ANSI: %q", colored)
	}
	if empty := style.Paint(Dim, ""); empty != "" {
		t.Fatalf("empty text gained ANSI: %q", empty)
	}
}

func TestGlyphsCoverKnownStates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		status, health, want string
	}{
		{status: "Running", health: "healthy", want: GlyphRunning},
		{status: "Running", health: "unhealthy", want: GlyphFailed},
		{status: "Launching", want: GlyphPending},
		{status: "Error", want: GlyphFailed},
		{status: "Completed", want: GlyphOK},
		{status: "external", want: GlyphExtern},
		{status: "active", want: GlyphRunning},
		{status: "inactive", want: GlyphIdle},
	} {
		if actual := ServiceGlyph(test.status, test.health); actual != test.want {
			t.Errorf("ServiceGlyph(%q, %q) = %s, want %s", test.status, test.health, actual, test.want)
		}
	}
	for status, want := range map[string]string{"ok": GlyphOK, "warning": GlyphWarning, "error": GlyphError} {
		if actual := CheckGlyph(status); actual != want {
			t.Errorf("CheckGlyph(%q) = %s, want %s", status, actual, want)
		}
	}
	for action, want := range map[string]string{
		"fast-forwarded": GlyphOK, "preserved": GlyphIdle, "would-remove": GlyphStep, "failed": GlyphError,
	} {
		if actual := ActionGlyph(action); actual != want {
			t.Errorf("ActionGlyph(%q) = %s, want %s", action, actual, want)
		}
	}
}

func TestEmptyTableRendersNote(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	if err := New(false).NewTable("SERVICE").Render(&buffer, "no services are declared"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "no services are declared") {
		t.Fatalf("empty table omitted its note: %q", buffer.String())
	}
}
