//go:build darwin || linux

package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func resourceRestartWaiting(options Options) (bool, *guardstate.IncidentSummary) {
	status, exists, err := guardstate.ReadStatus(options.Layout)
	if err != nil || !exists || status.Scope != supervisor.AuthorityScope(options.Layout, options.Runtime) {
		return false, nil
	}
	for index := range status.Services {
		service := &status.Services[index]
		if service.Name != options.Service {
			continue
		}
		switch service.State {
		case "restart_pending", "circuit_open", "restart_failed", "restart_refused", "stop_failed", "enforcement_refused":
			return true, service.LatestIncident
		default:
			return false, service.LatestIncident
		}
	}
	return false, nil
}

func printResourceIncident(stdout io.Writer, incident *guardstate.IncidentSummary, previous string) string {
	if incident == nil || incident.ID == previous {
		return previous
	}
	_, _ = fmt.Fprintf(stdout, "\n[rungrid] resource incident %s: %s breach (%s), action=%s\n", incident.OccurredAt, incident.Tier, incident.Trigger, incident.Action)
	return incident.ID
}

func quiesceReason(options Options) string {
	marker := filepath.Join(options.Layout.ProjectDir, "locks", "down-"+options.Runtime.GenerationID+".json")
	if _, err := os.Lstat(marker); err == nil {
		return "workspace shutdown began"
	} else if !os.IsNotExist(err) {
		return "workspace shutdown state became unverifiable"
	}
	if !supervisor.StaticScopeMatches(options.Layout, options.Runtime) {
		return "runtime identity disappeared or changed"
	}
	return ""
}
