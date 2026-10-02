package versions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/present"
)

func TestVersionsMarksOverriddenServices(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	WriteHuman(&output, present.New(false), Snapshot{Runtime: "running", Services: []ServiceVersion{
		{Name: "alpha", Repository: "workspace", State: "Running", Branch: "GH-1", Commit: "0123456", Override: "/lanes/GH-1"},
		{Name: "gamma", Repository: "workspace", State: "Running", Branch: "main", Commit: "0123456"},
	}})
	text := output.String()
	if !strings.Contains(text, "workspace "+present.EmojiOverride+" override") || !strings.Contains(text, "alpha runs from override /lanes/GH-1 (GH-1)") || strings.Contains(text, "gamma runs from override") {
		t.Fatalf("versions output:\n%s", text)
	}
}
