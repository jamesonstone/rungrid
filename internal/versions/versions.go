package versions

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type Snapshot struct {
	CapturedAt string           `json:"captured_at"`
	Runtime    string           `json:"runtime"`
	Generation string           `json:"generation"`
	Services   []ServiceVersion `json:"services"`
}

type ServiceVersion struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	State      string `json:"state"`
	Health     string `json:"health,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Ports      []int  `json:"ports"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
	GitState   string `json:"git_state"`
	Worktree   string `json:"worktree,omitempty"`
}

func Capture(ctx context.Context, m *manifest.Manifest, runtimeState supervisor.Runtime, client processcompose.Client) Snapshot {
	return NewCollector().Capture(ctx, m, runtimeState, client)
}

func MateriallyEqual(left, right Snapshot) bool {
	return left.Runtime == right.Runtime &&
		left.Generation == right.Generation &&
		reflect.DeepEqual(left.Services, right.Services)
}

func WriteHuman(w io.Writer, snapshot Snapshot) {
	_, _ = fmt.Fprintf(w, "Rungrid Versions  %s  generation %s\n\n", snapshot.CapturedAt, snapshot.Generation)
	_, _ = fmt.Fprintf(w, "%-18s %-14s %-18s %-9s %-7s %-12s %-18s %-10s\n", "SERVICE", "REPOSITORY", "STATE", "HEALTH", "PID", "PORTS", "BRANCH@COMMIT", "GIT")
	for _, service := range snapshot.Services {
		ports := "-"
		if len(service.Ports) > 0 {
			parts := make([]string, len(service.Ports))
			for i, port := range service.Ports {
				parts[i] = strconv.Itoa(port)
			}
			ports = strings.Join(parts, ",")
		}
		pid := "-"
		if service.PID > 0 {
			pid = strconv.Itoa(service.PID)
		}
		version := "-"
		if service.Branch != "" || service.Commit != "" {
			version = service.Branch + "@" + service.Commit
		}
		health := service.Health
		if health == "" {
			health = "-"
		}
		_, _ = fmt.Fprintf(w, "%-18s %-14s %-18s %-9s %-7s %-12s %-18s %-10s\n", service.Name, service.Repository, service.State, health, pid, ports, version, service.GitState)
	}
}
