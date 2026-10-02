//go:build darwin || linux

package lifecycle

import (
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
)

// Stop intent records that an operator deliberately stopped a managed service
// with rungrid stop. Resume consults it so a service the operator stopped is
// preserved rather than restarted. Records are generation-scoped, cleared by
// rungrid start, and removed wholesale when a fresh runtime starts.
const stopIntentDirectory = "stopped"

func stopIntentRelative(generationID, service string) string {
	return filepath.Join(stopIntentDirectory, generationID+"-"+service)
}

func recordStopIntent(layout state.Layout, generationID, service string) error {
	content := []byte(state.RuntimeTimestamp() + "\n")
	if err := state.WriteFileAtomic(layout.ProjectDir, stopIntentRelative(generationID, service), content, 0o600); err != nil {
		return errs.Wrap(errs.ExitPartial, "RG1146", "service stopped but its stop intent was not recorded", err)
	}
	return nil
}

func clearStopIntent(layout state.Layout, generationID, service string) error {
	err := os.Remove(filepath.Join(layout.ProjectDir, stopIntentRelative(generationID, service)))
	if err != nil && !os.IsNotExist(err) {
		return errs.Wrap(errs.ExitConflict, "RG1147", "clear service stop intent", err)
	}
	return nil
}

func clearAllStopIntents(layout state.Layout) error {
	if err := os.RemoveAll(filepath.Join(layout.ProjectDir, stopIntentDirectory)); err != nil {
		return errs.Wrap(errs.ExitConflict, "RG1148", "clear stale service stop intents", err)
	}
	return nil
}

func stopIntended(layout state.Layout, generationID, service string) bool {
	info, err := os.Lstat(filepath.Join(layout.ProjectDir, stopIntentRelative(generationID, service)))
	return err == nil && info.Mode().IsRegular()
}
