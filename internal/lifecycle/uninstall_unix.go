//go:build darwin || linux

package lifecycle

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/warp"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

func Uninstall(ctx context.Context, layout state.Layout, keepLogs, keepConfig bool) error {
	if err := layout.VerifyMarker(); err != nil {
		return err
	}
	lock, err := workspace.Acquire(ctx, layout)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	if err := downProjectLocked(ctx, layout); err != nil {
		return err
	}
	if journal, exists, err := workspace.ReadJournalIfPresent(layout); err != nil {
		return err
	} else if exists && (journal.TeardownRequired || journal.State == workspace.StateCleanup) {
		return errs.New(errs.ExitPartial, "RG1127", "refusing to uninstall while workspace cleanup remains required")
	}
	if err := warp.Uninstall(layout); err != nil {
		return err
	}
	expectedParent := filepath.Join(layout.StateRoot, "rungrid", "projects")
	if filepath.Dir(layout.ProjectDir) != expectedParent || filepath.Base(layout.ProjectDir) != layout.ProjectID {
		return errs.New(errs.ExitConflict, "RG1117", "refusing to uninstall outside the exact project state directory")
	}
	if keepLogs || keepConfig {
		return uninstallPreserving(layout, keepLogs, keepConfig)
	}
	if err := os.RemoveAll(layout.ProjectDir); err != nil {
		return errs.Wrap(errs.ExitPartial, "RG1118", "remove owned project state", err)
	}
	return nil
}

func uninstallPreserving(layout state.Layout, keepLogs, keepConfig bool) error {
	if keepLogs && !keepConfig {
		generationsDirectory := filepath.Join(layout.ProjectDir, "generations")
		entries, err := os.ReadDir(generationsDirectory)
		if err != nil && !os.IsNotExist(err) {
			return errs.Wrap(errs.ExitPartial, "RG1120", "inspect generation logs for preservation", err)
		}
		preservedLogs := filepath.Join(layout.ProjectDir, "preserved-logs")
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			source := filepath.Join(generationsDirectory, entry.Name(), "logs")
			if info, statErr := os.Lstat(source); statErr == nil {
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return errs.New(errs.ExitConflict, "RG1121", "generation log path is not a real directory")
				}
				if err := os.MkdirAll(preservedLogs, 0o700); err != nil {
					return errs.Wrap(errs.ExitPartial, "RG1122", "create preserved log directory", err)
				}
				if err := os.Rename(source, filepath.Join(preservedLogs, entry.Name())); err != nil {
					return errs.Wrap(errs.ExitPartial, "RG1123", "preserve generation logs", err)
				}
			} else if !os.IsNotExist(statErr) {
				return errs.Wrap(errs.ExitPartial, "RG1124", "inspect generation log path", statErr)
			}
		}
	}
	remove := []string{"sessions", "tabs", "locks", "terminal-install.json", "runtime.json", "runtime.sock"}
	if keepLogs {
		remove = append(remove, filepath.Join("resource-guard", "baselines"), filepath.Join("resource-guard", "clients"), filepath.Join("resource-guard", "resets"), filepath.Join("resource-guard", "status.json"))
	} else {
		remove = append(remove, "resource-guard")
	}
	if !keepConfig {
		remove = append(remove, "generations", "current", "lifecycle.json")
	}
	if !keepLogs {
		remove = append(remove, "process-compose.log", "client.log", "preserved-logs", "lifecycle-logs")
	}
	for _, name := range remove {
		target := filepath.Join(layout.ProjectDir, name)
		if err := os.RemoveAll(target); err != nil {
			return errs.Wrap(errs.ExitPartial, "RG1125", "remove owned project state component", err)
		}
	}
	return nil
}
