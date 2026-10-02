//go:build darwin || linux

package lifecycle

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

// Resume runtime outcomes.
const (
	ResumeRuntimeReused    = "reused"
	ResumeRuntimeRecovered = "recovered"
)

type ResumeOptions struct {
	Open      bool
	ForceOpen bool
}

type ResumeService struct {
	Name       string `json:"name"`
	Activation string `json:"activation"`
	Status     string `json:"status"`
	Action     string `json:"action"`
	Detail     string `json:"detail,omitempty"`
}

type ResumeResult struct {
	Runtime         string          `json:"runtime"`
	Generation      string          `json:"generation"`
	RuntimePID      int             `json:"runtime_pid"`
	WindowsLive     bool            `json:"windows_live"`
	OpenedWorkspace bool            `json:"opened_workspace"`
	OpenedTabs      []string        `json:"opened_tabs"`
	Services        []ResumeService `json:"services"`
}

// Resume reuses a verified live runtime after its terminal presentation was
// lost. It never reruns lifecycle hooks or restarts a running service: it
// restarts workspace services that stopped without operator intent, then
// reopens only the Warp windows that did not survive.
func Resume(ctx context.Context, active Active, options ResumeOptions) (ResumeResult, error) {
	statuses, _, err := Status(ctx, active)
	if err != nil {
		return ResumeResult{}, err
	}
	result := ResumeResult{
		Runtime: ResumeRuntimeReused, Generation: active.Runtime.GenerationID, RuntimePID: active.Runtime.PID,
		OpenedTabs: []string{}, Services: make([]ResumeService, 0, len(statuses)),
	}
	var failed, absentTabs []string
	anyTabLive := false
	for _, observed := range statuses {
		action := planServiceResume(resumeServiceInput{
			Source: observed.Source, Activation: observed.Activation, Status: observed.Status,
			StopIntended: stopIntended(active.Layout, active.Runtime.GenerationID, observed.Name),
			TabLive:      observed.TabRegistered,
		})
		item := ResumeService{Name: observed.Name, Activation: observed.Activation, Status: observed.Status, Action: action}
		switch action {
		case ResumeRestart:
			if _, startErr := Start(ctx, active, observed.Name, false); startErr != nil {
				item.Action, item.Detail = ResumeFailed, startErr.Error()
				failed = append(failed, observed.Name)
			} else {
				item.Action, item.Status = ResumeRestarted, "Running"
			}
		case ResumeStopped:
			item.Detail = "stopped with rungrid stop; start it with rungrid start " + observed.Name
		case ResumeTabLive:
			anyTabLive = true
		case ResumeTabAbsent:
			absentTabs = append(absentTabs, observed.Name)
		}
		result.Services = append(result.Services, item)
	}
	overviewLive, err := overviewAttached(active)
	if err != nil {
		return result, err
	}
	result.WindowsLive = overviewLive || anyTabLive
	plan := planWindowOpen(resumeWindowInput{
		TerminalMode: active.Manifest.Terminal.Mode, Open: options.Open, ForceOpen: options.ForceOpen,
		OverviewLive: overviewLive, AnyTabLive: anyTabLive, AbsentTabs: absentTabs,
	})
	if plan.Workspace {
		if err := Open(ctx, active, ""); err != nil {
			return result, err
		}
		result.OpenedWorkspace = true
		markTabsOpened(active.Manifest, result.Services, absentTabs)
	}
	for _, tab := range plan.Tabs {
		if err := Open(ctx, active, tab); err != nil {
			return result, err
		}
		result.OpenedTabs = append(result.OpenedTabs, tab)
		markTabsOpened(active.Manifest, result.Services, []string{tab})
	}
	if len(failed) > 0 {
		return result, errs.New(errs.ExitPartial, "RG1149", resumeFailureSummary(failed))
	}
	return result, nil
}

// overviewAttached reports whether a live Process Compose attach client, the
// process behind the Overview tab, still serves this exact runtime.
func overviewAttached(active Active) (bool, error) {
	clients, err := guardstate.ListControlClients(active.Layout, supervisor.AuthorityScope(active.Layout, active.Runtime))
	if err != nil {
		return false, err
	}
	for _, client := range clients {
		if client.Operation == "attach" && procidentity.Matches(client.PID, client.ProcessIdentity) {
			return true, nil
		}
	}
	return false, nil
}

func markTabsOpened(configuration *manifest.Manifest, services []ResumeService, opened []string) {
	for _, name := range opened {
		service, exists := manifest.FindService(configuration, name)
		if !exists {
			continue
		}
		for index := range services {
			if services[index].Name == name && services[index].Action == ResumeTabAbsent {
				services[index].Action = ResumeTabOpened
				services[index].Detail = "tab reopened; run " + formatArgv(service.Terminal.TriggerArgv) + " there"
			}
		}
	}
}
