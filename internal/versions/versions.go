package versions

import (
	"context"
	"reflect"

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
