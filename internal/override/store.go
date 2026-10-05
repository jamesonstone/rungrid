package override

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
)

// Load returns the overrides recorded for generationID. Entries recorded for
// any other generation are stale and ignored, so a changed manifest never
// inherits a previous runtime's overrides.
func Load(layout state.Layout, generationID string) (map[string]Entry, error) {
	store, err := readStore(layout)
	if err != nil {
		return nil, err
	}
	if generationID == "" || store.GenerationID != generationID {
		return map[string]Entry{}, nil
	}
	return store.Repositories, nil
}

// Save replaces the overrides recorded for generationID.
func Save(layout state.Layout, generationID string, entries map[string]Entry) error {
	if layout.ProjectDir == "" {
		return errs.New(errs.ExitUsage, "RG1804", "project state directory is required to record overrides")
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	if entries == nil {
		entries = map[string]Entry{}
	}
	store := storeFile{APIVersion: storeAPIVersion, ProjectID: layout.ProjectID, GenerationID: generationID, Repositories: entries}
	content, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1804", "encode repository overrides", err)
	}
	return state.WriteFileAtomic(layout.ProjectDir, storeFileName, append(content, '\n'), 0o600)
}

// Remove deletes every recorded override. A missing store is already clear.
func Remove(layout state.Layout) error {
	if layout.ProjectDir == "" {
		return nil
	}
	err := os.Remove(filepath.Join(layout.ProjectDir, storeFileName))
	if err != nil && !os.IsNotExist(err) {
		return errs.Wrap(errs.ExitConflict, "RG1801", "remove repository overrides", err)
	}
	return nil
}

// Sorted returns entries in repository-name order.
func Sorted(entries map[string]Entry) []Entry {
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Repository < result[j].Repository })
	return result
}

func readStore(layout state.Layout) (storeFile, error) {
	store := storeFile{APIVersion: storeAPIVersion, ProjectID: layout.ProjectID, Repositories: map[string]Entry{}}
	if layout.ProjectDir == "" {
		return store, nil
	}
	filename := filepath.Join(layout.ProjectDir, storeFileName)
	info, err := os.Lstat(filename)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return storeFile{}, errs.Wrap(errs.ExitConflict, "RG1801", "inspect repository overrides", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return storeFile{}, errs.New(errs.ExitConflict, "RG1802", "repository overrides are not a private regular file")
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return storeFile{}, errs.Wrap(errs.ExitConflict, "RG1801", "read repository overrides", err)
	}
	if json.Unmarshal(content, &store) != nil || store.APIVersion != storeAPIVersion {
		return storeFile{}, errs.New(errs.ExitConflict, "RG1802", "repository overrides are not valid")
	}
	if store.ProjectID != layout.ProjectID {
		return storeFile{}, errs.New(errs.ExitConflict, "RG1803", "repository overrides belong to another project")
	}
	if store.Repositories == nil {
		store.Repositories = map[string]Entry{}
	}
	return store, nil
}
