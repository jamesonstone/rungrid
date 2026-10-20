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
}

// Coordinator maps services to worktrees and pauses exact running services
// through the Process Compose lifecycle. Resume restarts what Pause stopped.
type Coordinator interface {
	Services(ctx context.Context, worktreePath string) ([]Service, error)
	Pause(ctx context.Context, names []string) (maintenance.ResumeFunc, error)
}

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
	Name    string `json:"name"`
	Restart string `json:"restart"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

const (
	KindDependencies = "dependencies"
	KindStep         = "step"
)

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
