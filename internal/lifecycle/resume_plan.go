//go:build darwin || linux

package lifecycle

import "strings"

// Resume service actions. Each managed service resolves to exactly one.
const (
	ResumeRunning   = "running"
	ResumeRestart   = "restart"
	ResumeRestarted = "restarted"
	ResumeFailed    = "failed"
	ResumeStopped   = "stopped"
	ResumeExternal  = "external"
	ResumeTabLive   = "tab-live"
	ResumeTabAbsent = "tab-absent"
	ResumeTabOpened = "tab-opened"
)

// resumeServiceInput is the observed state resume needs for one service.
type resumeServiceInput struct {
	Source       string
	Activation   string
	Status       string
	StopIntended bool
	TabLive      bool
}

// planServiceResume decides what resume does for one service. Workspace
// services that are not running are restarted unless the operator stopped them
// on purpose; tab services are never started here because a tab owns their
// lifecycle, so resume only reports whether the tab survived.
func planServiceResume(input resumeServiceInput) string {
	switch {
	case input.Source == "external":
		return ResumeExternal
	case input.Activation == "tab":
		if input.TabLive {
			return ResumeTabLive
		}
		return ResumeTabAbsent
	case shouldStop(input.Status):
		return ResumeRunning
	case input.StopIntended:
		return ResumeStopped
	default:
		return ResumeRestart
	}
}

// resumeWindowInput describes the surviving Warp presentation.
type resumeWindowInput struct {
	TerminalMode string
	Open         bool
	ForceOpen    bool
	OverviewLive bool
	AnyTabLive   bool
	AbsentTabs   []string
}

type resumeWindowPlan struct {
	Workspace bool
	Tabs      []string
}

// planWindowOpen reopens the whole workspace only when no Rungrid window
// survived; otherwise it reopens just the missing service tabs so a partial
// restart never duplicates the Overview and Versions tabs.
func planWindowOpen(input resumeWindowInput) resumeWindowPlan {
	if input.TerminalMode != "warp" || !input.Open {
		return resumeWindowPlan{}
	}
	if input.ForceOpen || (!input.OverviewLive && !input.AnyTabLive) {
		return resumeWindowPlan{Workspace: true}
	}
	return resumeWindowPlan{Tabs: append([]string(nil), input.AbsentTabs...)}
}

func resumeFailureSummary(names []string) string {
	return "resume could not restart: " + strings.Join(names, ", ")
}
