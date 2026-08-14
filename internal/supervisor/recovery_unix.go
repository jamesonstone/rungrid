//go:build darwin || linux

package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/state"
)

// RetireStaleRuntime removes only a verified runtime record whose process and
// socket are both absent. Callers must hold the project lifecycle lock.
func RetireStaleRuntime(layout state.Layout, runtimeState Runtime) (bool, error) {
	if err := layout.VerifyMarker(); err != nil {
		return false, err
	}
	if err := verifyRecordScope(layout, runtimeState); err != nil {
		return false, err
	}
	if !completeRecordedIdentity(runtimeState) {
		return false, errs.New(errs.ExitConflict, "RG635", "stale runtime record identity is incomplete")
	}
	if processExists(runtimeState.PID) {
		return false, nil
	}
	absent, err := socketAbsent(runtimeState.Socket)
	if err != nil || !absent {
		return false, err
	}

	current, err := Read(layout)
	if err != nil {
		return false, errs.Wrap(errs.ExitConflict, "RG637", "re-read stale runtime record", err)
	}
	if current != runtimeState {
		return false, errs.New(errs.ExitConflict, "RG638", "runtime record changed during stale recovery")
	}
	if processExists(current.PID) {
		return false, nil
	}
	absent, err = socketAbsent(current.Socket)
	if err != nil || !absent {
		return false, err
	}
	if err := os.Remove(filepath.Join(layout.ProjectDir, "runtime.json")); err != nil {
		return false, errs.Wrap(errs.ExitPartial, "RG639", "remove stale runtime record", err)
	}
	_ = processcompose.RemoveSocketAlias(runtimeState.Socket)
	return true, nil
}

func completeRecordedIdentity(runtimeState Runtime) bool {
	return runtimeState.GenerationID != "" && runtimeState.PID > 1 &&
		runtimeState.EffectiveManifestSHA256 != "" && runtimeState.ConfigurationHash != "" &&
		strings.TrimSpace(runtimeState.ProcessIdentity) != "" &&
		strings.Contains(strings.ToLower(runtimeState.ProcessCommand), "process-compose") &&
		runtimeState.SocketDevice != 0 && runtimeState.SocketInode != 0 && runtimeState.SocketOwnerUID == uint32(os.Getuid())
}

func socketAbsent(filename string) (bool, error) {
	_, err := os.Lstat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, errs.Wrap(errs.ExitConflict, "RG636", "inspect stale runtime socket", err)
	}
	return false, nil
}
