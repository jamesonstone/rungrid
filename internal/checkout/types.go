package checkout

import "time"

const (
	storeAPIVersion = "rungrid/output/v1"
	storeFileName   = "checkouts.json"
)

type Roots struct {
	Service          string
	Repository       string
	RepositoryRoot   string
	WorkingDirectory string
	Branch           string
	HeadOID          string
	Primary          bool
	Selected         bool
	SelectedPath     string
}

type Selection struct {
	Path       string `json:"path"`
	SelectedAt string `json:"selected_at"`
}

type storeFile struct {
	APIVersion string               `json:"api_version"`
	ProjectID  string               `json:"project_id"`
	Services   map[string]Selection `json:"services"`
}

type Worktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Locked   bool
	Primary  bool
}

type ListedWorktree struct {
	Path       string   `json:"path"`
	Branch     string   `json:"branch,omitempty"`
	HeadOID    string   `json:"head_oid,omitempty"`
	Primary    bool     `json:"primary"`
	Detached   bool     `json:"detached,omitempty"`
	SelectedBy []string `json:"selected_by,omitempty"`
}

type RepositoryList struct {
	Name      string           `json:"name"`
	Remote    string           `json:"remote,omitempty"`
	Primary   string           `json:"primary,omitempty"`
	Worktrees []ListedWorktree `json:"worktrees"`
}

type ListReport struct {
	Operation    string           `json:"operation"`
	StartedAt    string           `json:"started_at"`
	FinishedAt   string           `json:"finished_at"`
	Repositories []RepositoryList `json:"repositories"`
	Failures     []Failure        `json:"failures"`
}

type UseReport struct {
	Operation string `json:"operation"`
	Service   string `json:"service"`
	Path      string `json:"path,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Action    string `json:"action"`
	Detail    string `json:"detail,omitempty"`
}

type UpdateTarget struct {
	Service   string `json:"service"`
	Path      string `json:"path"`
	Branch    string `json:"branch,omitempty"`
	LocalOID  string `json:"local_oid,omitempty"`
	RemoteOID string `json:"remote_oid,omitempty"`
	State     string `json:"state"`
	Action    string `json:"action"`
	Detail    string `json:"detail,omitempty"`
}

type UpdateReport struct {
	Operation  string         `json:"operation"`
	DryRun     bool           `json:"dry_run"`
	Sync       bool           `json:"sync"`
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at"`
	Targets    []UpdateTarget `json:"targets"`
	Failures   []Failure      `json:"failures"`
}

type Failure struct {
	Service    string `json:"service,omitempty"`
	Repository string `json:"repository,omitempty"`
	Operation  string `json:"operation"`
	Path       string `json:"path,omitempty"`
	Error      string `json:"error"`
}

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
