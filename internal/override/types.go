// Package override points one repository's managed services at another
// checkout of that repository for the life of a runtime generation.
package override

import "time"

const (
	storeAPIVersion = "rungrid/output/v1"
	storeFileName   = "overrides.json"
)

// Override sources record why an entry exists.
const (
	SourceCLI      = "cli"
	SourceManifest = "manifest"
	SourceUpFlag   = "up-flag"
)

// Reanchor reports one argument whose relative path was rewritten so it keeps
// naming the same file, or the mapped file, after the base directory moves.
type Reanchor struct {
	Service  string `json:"service"`
	Field    string `json:"field"`
	Original string `json:"original"`
	Resolved string `json:"resolved"`
}

// Entry is one active repository override.
type Entry struct {
	Repository   string     `json:"repository"`
	OriginalPath string     `json:"original_path"`
	Path         string     `json:"path"`
	Branch       string     `json:"branch,omitempty"`
	HeadOID      string     `json:"head_oid,omitempty"`
	Dirty        bool       `json:"dirty"`
	Source       string     `json:"source"`
	SetAt        string     `json:"set_at"`
	Services     []string   `json:"services"`
	Reanchored   []Reanchor `json:"reanchored"`
}

type storeFile struct {
	APIVersion   string           `json:"api_version"`
	ProjectID    string           `json:"project_id"`
	GenerationID string           `json:"generation_id"`
	Repositories map[string]Entry `json:"repositories"`
}

// Repository is one Git checkout that contains at least one service's declared
// working directory.
type Repository struct {
	Name      string
	TopLevel  string
	CommonDir string
	Origin    string
	Managed   []string
	External  []string
}

// ServiceAction reports what applying an override change did to one service.
type ServiceAction struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Activation string `json:"activation"`
	Action     string `json:"action"`
	Detail     string `json:"detail,omitempty"`
}

// Service actions emitted by an override change.
const (
	ActionRestarted     = "restarted"
	ActionRestartFailed = "restart-failed"
	ActionStopped       = "stopped"
	ActionNotRunning    = "not-running"
	ActionTabStopped    = "tab-stopped"
	ActionTabIdle       = "tab-idle"
)

// Report is the result of set, clear, sync, and up seeding.
type Report struct {
	Operation string          `json:"operation"`
	Set       []Entry         `json:"set"`
	Cleared   []Entry         `json:"cleared"`
	Services  []ServiceAction `json:"services"`
	Warnings  []string        `json:"warnings"`
}

// ListReport is the result of override list.
type ListReport struct {
	Generation string  `json:"generation"`
	Overrides  []Entry `json:"overrides"`
}

// Candidate is one checkout offered by the worktree picker.
type Candidate struct {
	Path      string    `json:"path"`
	Branch    string    `json:"branch,omitempty"`
	HeadOID   string    `json:"head_oid,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	Primary   bool      `json:"primary"`
	Dirty     bool      `json:"dirty"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
