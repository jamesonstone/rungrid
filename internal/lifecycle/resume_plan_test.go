//go:build darwin || linux

package lifecycle

import (
	"reflect"
	"testing"

	"github.com/jamesonstone/rungrid/internal/state"
)

func TestPlanServiceResume(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		input resumeServiceInput
		want  string
	}{
		{"external is never managed", resumeServiceInput{Source: "external", Activation: "workspace", Status: "external"}, ResumeExternal},
		{"running is left alone", resumeServiceInput{Activation: "workspace", Status: "Running"}, ResumeRunning},
		{"launching is left alone", resumeServiceInput{Activation: "workspace", Status: "Launching"}, ResumeRunning},
		{"crashed restarts", resumeServiceInput{Activation: "workspace", Status: "Error"}, ResumeRestart},
		{"completed restarts", resumeServiceInput{Activation: "workspace", Status: "Completed"}, ResumeRestart},
		{"operator stop is preserved", resumeServiceInput{Activation: "workspace", Status: "Completed", StopIntended: true}, ResumeStopped},
		{"running ignores stale stop intent", resumeServiceInput{Activation: "workspace", Status: "Running", StopIntended: true}, ResumeRunning},
		{"live tab is reported", resumeServiceInput{Activation: "tab", Status: "Disabled", TabLive: true}, ResumeTabLive},
		{"closed tab is never started", resumeServiceInput{Activation: "tab", Status: "Error"}, ResumeTabAbsent},
	} {
		if got := planServiceResume(test.input); got != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}

func TestPlanWindowOpen(t *testing.T) {
	t.Parallel()
	tabs := []string{"api"}
	for _, test := range []struct {
		name  string
		input resumeWindowInput
		want  resumeWindowPlan
	}{
		{"headless never opens", resumeWindowInput{TerminalMode: "headless", Open: true, ForceOpen: true}, resumeWindowPlan{}},
		{"no-open never opens", resumeWindowInput{TerminalMode: "warp", AbsentTabs: tabs}, resumeWindowPlan{}},
		{"lost windows reopen the workspace", resumeWindowInput{TerminalMode: "warp", Open: true, AbsentTabs: tabs}, resumeWindowPlan{Workspace: true}},
		{"live overview reopens only absent tabs", resumeWindowInput{TerminalMode: "warp", Open: true, OverviewLive: true, AbsentTabs: tabs}, resumeWindowPlan{Tabs: tabs}},
		{"live tab reopens only absent tabs", resumeWindowInput{TerminalMode: "warp", Open: true, AnyTabLive: true}, resumeWindowPlan{}},
		{"force reopens the workspace", resumeWindowInput{TerminalMode: "warp", Open: true, ForceOpen: true, OverviewLive: true}, resumeWindowPlan{Workspace: true}},
	} {
		got := planWindowOpen(test.input)
		if got.Workspace != test.want.Workspace || len(got.Tabs) != len(test.want.Tabs) || (len(got.Tabs) > 0 && !reflect.DeepEqual(got.Tabs, test.want.Tabs)) {
			t.Errorf("%s: got %+v, want %+v", test.name, got, test.want)
		}
	}
}

func TestStopIntentLifecycle(t *testing.T) {
	t.Parallel()
	layout, err := state.NewLayout("example-k7m4q2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if stopIntended(layout, "gen-a", "api") {
		t.Fatal("stop intent exists before any stop")
	}
	if err := recordStopIntent(layout, "gen-a", "api"); err != nil {
		t.Fatal(err)
	}
	if !stopIntended(layout, "gen-a", "api") {
		t.Fatal("recorded stop intent was not observed")
	}
	if stopIntended(layout, "gen-b", "api") || stopIntended(layout, "gen-a", "worker") {
		t.Fatal("stop intent leaked across generation or service")
	}
	if err := clearStopIntent(layout, "gen-a", "api"); err != nil {
		t.Fatal(err)
	}
	if stopIntended(layout, "gen-a", "api") {
		t.Fatal("cleared stop intent is still observed")
	}
	if err := clearStopIntent(layout, "gen-a", "api"); err != nil {
		t.Fatalf("clearing an absent stop intent must succeed: %v", err)
	}
	if err := recordStopIntent(layout, "gen-a", "worker"); err != nil {
		t.Fatal(err)
	}
	if err := clearAllStopIntents(layout); err != nil {
		t.Fatal(err)
	}
	if stopIntended(layout, "gen-a", "worker") {
		t.Fatal("fresh runtime start must clear every stop intent")
	}
}
