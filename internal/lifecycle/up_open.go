//go:build darwin || linux

package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/terminalshell"
	"github.com/jamesonstone/rungrid/internal/warp"
)

const (
	attachPollInterval = 500 * time.Millisecond
	attachStopDelay    = 3 * time.Second
	// windowStartGrace covers a new Warp window before its shell has started
	// the Overview client; nothing can observe the window during that time.
	windowStartGrace = 90 * time.Second
	windowOpenRecord = "terminal-opened.json"
)

// RuntimeActive reports whether a runtime still serves its generation: no
// shutdown has begun and its recorded identity is intact.
func RuntimeActive(layout state.Layout, runtimeState supervisor.Runtime) bool {
	marker := filepath.Join(layout.ProjectDir, "locks", "down-"+runtimeState.GenerationID+".json")
	if _, err := os.Lstat(marker); err == nil || !os.IsNotExist(err) {
		return false
	}
	return supervisor.StaticScopeMatches(layout, runtimeState)
}

// attachResult maps the attach client's exit. A client stopped because its
// runtime ended has done its job; that is not a failure.
func attachResult(err error, runtimeActive bool) error {
	if err == nil || !runtimeActive {
		return nil
	}
	return errs.Wrap(errs.ExitFailure, "RG1116", "attach Process Compose TUI", err)
}

// watchRuntimeEnd reports true once active says the runtime this client
// attached to has ended, and false if ctx ends first. Without it, a Process
// Compose client retries its socket, reconnects to the next runtime when up
// follows down, and repaints in its tab indefinitely; each restart then leaves
// one more live client.
func watchRuntimeEnd(ctx context.Context, interval time.Duration, active func() bool) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if !active() {
				return true
			}
		}
	}
}

type windowOpen struct {
	RuntimePID int       `json:"runtime_pid"`
	OpenedAt   time.Time `json:"opened_at"`
}

// upOpenInput is what up needs to decide which Warp windows to open.
type upOpenInput struct {
	Reused      bool
	WindowsLive bool
	AbsentTabs  []string
}

// planUpOpen opens the whole workspace unless a reused runtime still has a
// live window; repeating up must never stack a second window over it. A live
// workspace gets only the requested service tabs it lacks.
func planUpOpen(input upOpenInput) resumeWindowPlan {
	if input.Reused && input.WindowsLive {
		return resumeWindowPlan{Tabs: append([]string(nil), input.AbsentTabs...)}
	}
	return resumeWindowPlan{Workspace: true}
}

// windowsLive reports whether any Warp presentation of the runtime survives,
// the same evidence resume uses, and which requested tab services lack a tab.
func windowsLive(
	layout state.Layout,
	configuration *manifest.Manifest,
	runtimeState supervisor.Runtime,
	requested []string,
) (bool, []string, error) {
	live, err := overviewAttached(Active{Layout: layout, Runtime: runtimeState, Manifest: configuration})
	if err != nil {
		return false, nil, err
	}
	live = live || windowStarting(layout, runtimeState, time.Now())
	for _, service := range configuration.Services {
		if service.Activation != "tab" {
			continue
		}
		if _, active := terminalshell.ActiveTab(layout, runtimeState.GenerationID, service.Name); active {
			live = true
		}
	}
	var absent []string
	for _, name := range requested {
		service, exists := manifest.FindService(configuration, name)
		if !exists || service.Activation != "tab" {
			continue
		}
		if _, active := terminalshell.ActiveTab(layout, runtimeState.GenerationID, name); !active {
			absent = append(absent, name)
		}
	}
	return live, absent, nil
}

// recordWindowOpen notes that this runtime's workspace window just opened.
func recordWindowOpen(layout state.Layout, runtimeState supervisor.Runtime, now time.Time) error {
	content, err := json.Marshal(windowOpen{RuntimePID: runtimeState.PID, OpenedAt: now.UTC()})
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(layout.ProjectDir, windowOpenRecord, append(content, '\n'), 0o600)
}

// windowStarting reports whether this runtime's window opened so recently
// that its Overview client may not have started yet.
func windowStarting(layout state.Layout, runtimeState supervisor.Runtime, now time.Time) bool {
	content, err := os.ReadFile(filepath.Join(layout.ProjectDir, windowOpenRecord))
	if err != nil {
		return false
	}
	var record windowOpen
	if json.Unmarshal(content, &record) != nil || record.RuntimePID != runtimeState.PID {
		return false
	}
	return now.Sub(record.OpenedAt) < windowStartGrace
}

// openUpWindows opens what planUpOpen decides and reports whether it opened
// anything.
func openUpWindows(ctx context.Context, record warp.InstallRecord, plan resumeWindowPlan) (bool, error) {
	if plan.Workspace {
		return true, warp.Open(ctx, record, "")
	}
	for _, tab := range plan.Tabs {
		if err := warp.Open(ctx, record, tab); err != nil {
			return true, err
		}
	}
	return len(plan.Tabs) > 0, nil
}
