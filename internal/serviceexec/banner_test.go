package serviceexec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/override"
)

func TestOverrideBannerMarksOnlyOverriddenStarts(t *testing.T) {
	t.Parallel()
	var plain bytes.Buffer
	writeOverrideBanner(&plain, "api", override.Execution{})
	if plain.Len() != 0 {
		t.Fatalf("banner written without an override: %q", plain.String())
	}
	entry := &override.Entry{
		Repository: "api", OriginalPath: "/ws/api", Path: "/lanes/GH-1", Branch: "GH-1",
		Reanchored: []override.Reanchor{
			{Service: "api", Field: "run.argv", Original: "../tools/run.sh", Resolved: "/ws/tools/run.sh"},
			{Service: "worker", Field: "run.argv", Original: "../x", Resolved: "/ws/x"},
		},
	}
	var banner bytes.Buffer
	writeOverrideBanner(&banner, "api", override.Execution{Roots: checkout.Roots{WorkingDirectory: "/lanes/GH-1"}, Override: entry})
	text := banner.String()
	if !strings.Contains(text, "api runs from override /lanes/GH-1 (GH-1) for repository api instead of /ws/api") ||
		!strings.Contains(text, "run.argv ../tools/run.sh re-anchored to /ws/tools/run.sh") || strings.Contains(text, "/ws/x") {
		t.Fatalf("banner = %q", text)
	}
}
