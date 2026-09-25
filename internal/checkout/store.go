package checkout

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
)

func loadStore(layout state.Layout) (storeFile, error) {
	store := storeFile{APIVersion: storeAPIVersion, ProjectID: layout.ProjectID, Services: map[string]Selection{}}
	if layout.ProjectDir == "" {
		return store, nil
	}
	content, err := os.ReadFile(filepath.Join(layout.ProjectDir, storeFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return storeFile{}, errs.Wrap(errs.ExitConflict, "RG1701", "read service checkout selections", err)
	}
	if json.Unmarshal(content, &store) != nil || store.APIVersion != storeAPIVersion {
		return storeFile{}, errs.New(errs.ExitConflict, "RG1702", "service checkout selections are not valid")
	}
	if store.ProjectID != "" && store.ProjectID != layout.ProjectID {
		return storeFile{}, errs.New(errs.ExitConflict, "RG1703", "service checkout selections belong to another project")
	}
	if store.Services == nil {
		store.Services = map[string]Selection{}
	}
	store.ProjectID = layout.ProjectID
	return store, nil
}

func saveStore(layout state.Layout, store storeFile) error {
	if layout.ProjectDir == "" {
		return errs.New(errs.ExitUsage, "RG1704", "project state directory is required to persist checkout selections")
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	store.APIVersion = storeAPIVersion
	store.ProjectID = layout.ProjectID
	if store.Services == nil {
		store.Services = map[string]Selection{}
	}
	content, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1705", "encode service checkout selections", err)
	}
	return state.WriteFileAtomic(layout.ProjectDir, storeFileName, append(content, '\n'), 0o600)
}

func storedPath(layout state.Layout, service string) (string, bool, error) {
	store, err := loadStore(layout)
	if err != nil {
		return "", false, err
	}
	selection, exists := store.Services[service]
	if !exists || selection.Path == "" {
		return "", false, nil
	}
	return selection.Path, true, nil
}
