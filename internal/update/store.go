package update

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/output"
	"github.com/jamesonstone/rungrid/internal/state"
)

const storePath = "update/inputs.json"

// store records, per default-branch worktree, the fingerprint of every input
// whose dependency install, refresh step, or restart last succeeded.
type store struct {
	APIVersion   string                     `json:"api_version"`
	ProjectID    string                     `json:"project_id"`
	Repositories map[string]repositoryInput `json:"repositories"`
}

type repositoryInput struct {
	OID       string            `json:"oid"`
	Inputs    map[string]string `json:"inputs"`
	UpdatedAt string            `json:"updated_at"`
}

func loadStore(layout state.Layout) (store, error) {
	result := store{APIVersion: output.APIVersion, ProjectID: layout.ProjectID, Repositories: map[string]repositoryInput{}}
	content, err := os.ReadFile(filepath.Join(layout.ProjectDir, storePath))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, errs.Wrap(errs.ExitConflict, "RG1901", "read update input state", err)
	}
	var loaded store
	if json.Unmarshal(content, &loaded) != nil || loaded.APIVersion != output.APIVersion || loaded.ProjectID != layout.ProjectID {
		return result, errs.New(errs.ExitConflict, "RG1902", "update input state is invalid; remove "+filepath.Join(layout.ProjectDir, storePath)+" to rebuild it")
	}
	if loaded.Repositories == nil {
		loaded.Repositories = map[string]repositoryInput{}
	}
	return loaded, nil
}

func saveStore(layout state.Layout, value store) error {
	if err := layout.Ensure(); err != nil {
		return err
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(layout.ProjectDir, storePath, append(content, '\n'), 0o600)
}
