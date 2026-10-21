// Package update brings a workspace's checked-out default branches current in
// place: it fast-forwards them with the sync contract, reinstalls dependencies
// and runs declared refresh steps only when their inputs changed, and restarts
// only the running services those changes affect.
package update

import (
	"context"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

const Operation = "update"

// Report statuses shared by repositories, steps, and services.
const (
	StatusUpdated = "UPDATED"
	StatusCurrent = "CURRENT"
	StatusSkipped = "SKIPPED"
	StatusFailed  = "FAILED"
)

type Options struct {
	Repositories []string
	DryRun       bool
	// Baseline records the current inputs as up to date without fetching,
	// fast-forwarding, running steps, or restarting services.
	Baseline bool
}

// Service is one Rungrid-owned service whose effective checkout lies inside
// an inspected default-branch worktree.
type Service struct {
	Config           *manifest.Service
	WorkingDirectory string
	Running          bool
	// RestartPath says how a running service restarts: RestartSession for a
	// tab service with a live owning session, otherwise RestartProcess.
	RestartPath string
}

const (
	// RestartSession pauses and resumes through the owning session, which
	// keeps the tab, its ownership lock, and its log view.
	RestartSession = "session"
	// RestartProcess stops and starts the process through Process Compose.
	RestartProcess = "process-compose"
)

// Coordinator maps services to worktrees and pauses one exact running service
// through the Process Compose lifecycle. The returned function restarts what
// Pause stopped and waits until the service is ready again.
type Coordinator interface {
	Services(ctx context.Context, worktreePath string) ([]Service, error)
	Pause(ctx context.Context, name string) (maintenance.ResumeFunc, error)
}

// Recovered is returned by a ResumeFunc whose service is running again only
// through a fallback, for example Process Compose after a failed session
// restart. The engine reports it as a successful restart with this detail.
type Recovered struct {
	Detail string
}

func (r Recovered) Error() string { return r.Detail }

// Command is one dependency install or refresh step.
type Command struct {
	Name             string
	Root             string
	WorkingDirectory string
	Argv             []string
	Timeout          time.Duration
	Environment      manifest.Environment
}

// Executor runs a command and returns the state-relative log path.
type Executor interface {
	Execute(ctx context.Context, index int, command Command) (string, error)
}

type Report struct {
	Operation    string                `json:"operation"`
	DryRun       bool                  `json:"dry_run"`
	Baseline     bool                  `json:"baseline"`
	StartedAt    string                `json:"started_at"`
	FinishedAt   string                `json:"finished_at"`
	Repositories []RepositoryResult    `json:"repositories"`
	Failures     []maintenance.Failure `json:"failures"`
}

type RepositoryResult struct {
	Name          string          `json:"name"`
	Path          string          `json:"path,omitempty"`
	DefaultBranch string          `json:"default_branch,omitempty"`
	FromOID       string          `json:"from_oid,omitempty"`
	ToOID         string          `json:"to_oid,omitempty"`
	Status        string          `json:"status"`
	Git           string          `json:"git"`
	Reason        string          `json:"reason,omitempty"`
	Steps         []StepResult    `json:"steps"`
	Services      []ServiceResult `json:"services"`
}

type StepResult struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Services  []string `json:"services"`
	Directory string   `json:"directory"`
	Argv      []string `json:"argv"`
	Status    string   `json:"status"`
	Reason    string   `json:"reason"`
	Log       string   `json:"log,omitempty"`
}

type ServiceResult struct {
	Name        string `json:"name"`
	Restart     string `json:"restart"`
	RestartPath string `json:"restart_path,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
}

const (
	KindDependencies = "dependencies"
	KindStep         = "step"
)

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
