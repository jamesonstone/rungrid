package reconcile

import (
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

const inactivityThreshold = 72 * time.Hour

type Options struct {
	Target            string
	DryRun            bool
	IncludeSubmodules bool
	Runner            maintenance.Runner
	Coordinator       maintenance.Coordinator
	RecoveryTimeout   time.Duration
	Now               func() time.Time
}

type Report struct {
	Operation    string                `json:"operation"`
	Target       string                `json:"target"`
	DryRun       bool                  `json:"dry_run"`
	StartedAt    string                `json:"started_at"`
	FinishedAt   string                `json:"finished_at"`
	Repositories []RepositoryResult    `json:"repositories"`
	Failures     []maintenance.Failure `json:"failures"`
}

type RepositoryResult struct {
	Name          string                         `json:"name"`
	Path          string                         `json:"path"`
	CommonDir     string                         `json:"common_dir"`
	DefaultBranch string                         `json:"default_branch,omitempty"`
	Sync          maintenance.SyncRepository     `json:"sync"`
	Root          RootResult                     `json:"root"`
	Worktrees     []maintenance.WorktreeDecision `json:"worktrees"`
}

type RootResult struct {
	Path                   string   `json:"path"`
	Branch                 string   `json:"branch,omitempty"`
	HeadOID                string   `json:"head_oid,omitempty"`
	Dirty                  bool     `json:"dirty"`
	DirtyPaths             []string `json:"dirty_paths,omitempty"`
	ActivityAt             string   `json:"activity_at,omitempty"`
	HeadCommitAt           string   `json:"head_commit_at,omitempty"`
	HeadReflogAt           string   `json:"head_reflog_at,omitempty"`
	DirtyPathAt            string   `json:"dirty_path_at,omitempty"`
	CWDProcessIDs          []string `json:"cwd_process_ids,omitempty"`
	OpenPathProcessIDs     []string `json:"open_path_process_ids,omitempty"`
	OwnedProcessIDs        []string `json:"owned_process_ids,omitempty"`
	ProcessInspectionError string   `json:"process_inspection_error,omitempty"`
	Services               []string `json:"services,omitempty"`
	Action                 string   `json:"action"`
	Reason                 string   `json:"reason"`
	Detail                 string   `json:"detail,omitempty"`
	WIPCommitOID           string   `json:"wip_commit_oid,omitempty"`
	StashOID               string   `json:"stash_oid,omitempty"`
}

type rootState struct {
	path       string
	branch     string
	headOID    string
	dirtyPaths []string
	staged     bool
	conflicted bool
	ignored    []string
	activityAt time.Time
	commitAt   time.Time
	reflogAt   time.Time
	dirtyAt    time.Time
	cwdPIDs    []string
	openPIDs   []string
	processes  []string
	processErr error
}

func (state rootState) dirty() bool { return len(state.dirtyPaths) != 0 }

func (state rootState) result() RootResult {
	result := RootResult{
		Path: state.path, Branch: state.branch, HeadOID: state.headOID,
		Dirty: state.dirty(), DirtyPaths: append([]string(nil), state.dirtyPaths...),
		CWDProcessIDs: append([]string(nil), state.cwdPIDs...), OpenPathProcessIDs: append([]string(nil), state.openPIDs...),
		Action: "preserved", Reason: "not-evaluated",
	}
	if !state.activityAt.IsZero() {
		result.ActivityAt = state.activityAt.UTC().Format(time.RFC3339Nano)
	}
	result.HeadCommitAt = reportTime(state.commitAt)
	result.HeadReflogAt = reportTime(state.reflogAt)
	result.DirtyPathAt = reportTime(state.dirtyAt)
	if state.processErr != nil {
		result.ProcessInspectionError = state.processErr.Error()
	}
	return result
}

func reportTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
