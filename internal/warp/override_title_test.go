package warp

import (
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/state"
)

func TestOverrideTitlesMarkOnlyOverriddenServiceTabs(t *testing.T) {
	t.Parallel()
	configuration := &manifest.Manifest{
		Project: manifest.Project{ID: "example-k7m4q2"},
		Services: []manifest.Service{
			{Name: "api", Source: "native", Activation: "tab", Terminal: manifest.ServiceTerminal{Title: "API"}},
			{Name: "web", Source: "native", Activation: "tab", Terminal: manifest.ServiceTerminal{Title: "Web"}},
		},
	}
	layout, err := state.NewLayout("example-k7m4q2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := override.Save(layout, "generation-a", map[string]override.Entry{"api": {Repository: "api", Path: "/lanes/GH-1", Branch: "GH-1", Services: []string{"api"}}}); err != nil {
		t.Fatal(err)
	}
	base := Templates(configuration, "generation-a")
	titled := overrideTitles(layout, configuration, "generation-a", base)
	if !strings.Contains(string(titled[2].Content), `title = "API (override: GH-1)"`) || titled[2].Filename != base[2].Filename {
		t.Fatalf("api tab = %s", titled[2].Content)
	}
	if string(titled[3].Content) != string(base[3].Content) || string(titled[0].Content) != string(base[0].Content) {
		t.Fatal("a tab without an override changed")
	}
	if stale := overrideTitles(layout, configuration, "generation-b", base); string(stale[2].Content) != string(base[2].Content) {
		t.Fatal("another generation's override decorated a tab")
	}
}
