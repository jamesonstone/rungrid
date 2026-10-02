//go:build darwin || linux

package lifecycle

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/state"
)

func TestStatusMarksOverriddenServicesAndListsOverrides(t *testing.T) {
	t.Parallel()
	layout := overrideTestLayout(t)
	if err := override.Save(layout, "generation-a", map[string]override.Entry{"svc": {Repository: "svc", Path: "/lanes/GH-1", Branch: "GH-1", Services: []string{"alpha"}}}); err != nil {
		t.Fatal(err)
	}
	status := WorkspaceStatus{ProjectID: layout.ProjectID, Runtime: "active", Generation: "generation-a", Services: []ServiceStatus{
		{Name: "alpha", Status: "Running", Source: "native", Activation: "workspace"},
		{Name: "gamma", Status: "Running", Source: "native", Activation: "workspace"},
	}}
	if err := attachOverrides(layout, &status); err != nil {
		t.Fatal(err)
	}
	if status.Services[0].Override == nil || status.Services[0].Override.Path != "/lanes/GH-1" || status.Services[1].Override != nil {
		t.Fatalf("services = %#v", status.Services)
	}
	var output bytes.Buffer
	if err := WriteStatusHuman(&output, present.New(false), status); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "alpha "+present.EmojiOverride) || strings.Contains(text, "gamma "+present.EmojiOverride) ||
		!strings.Contains(text, "Overrides (1)") || !strings.Contains(text, "/lanes/GH-1") {
		t.Fatalf("status output:\n%s", text)
	}
}

func overrideTestLayout(t *testing.T) state.Layout {
	t.Helper()
	layout, err := state.NewLayout("example-k7m4q2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return layout
}
