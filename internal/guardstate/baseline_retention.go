package guardstate

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/state"
)

type baselineFile struct {
	name      string
	updatedAt time.Time
}

func pruneBaselines(layout state.Layout, maximum int) error {
	directory := filepath.Join(layout.ProjectDir, "resource-guard", "baselines")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	baselines := make([]baselineFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var baseline Baseline
		exists, readErr := readPrivateJSON(filepath.Join(directory, entry.Name()), &baseline)
		if readErr != nil {
			return readErr
		}
		if !exists || baseline.APIVersion != apiVersion || baseline.Scope.ProjectID != layout.ProjectID {
			continue
		}
		updatedAt, parseErr := time.Parse(time.RFC3339Nano, baseline.UpdatedAt)
		if parseErr != nil {
			updatedAt = time.Time{}
		}
		baselines = append(baselines, baselineFile{name: entry.Name(), updatedAt: updatedAt})
	}
	if len(baselines) <= maximum {
		return nil
	}
	sort.Slice(baselines, func(i, j int) bool {
		if baselines[i].updatedAt.Equal(baselines[j].updatedAt) {
			return baselines[i].name < baselines[j].name
		}
		return baselines[i].updatedAt.Before(baselines[j].updatedAt)
	})
	for _, baseline := range baselines[:len(baselines)-maximum] {
		if err := os.Remove(filepath.Join(directory, baseline.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
