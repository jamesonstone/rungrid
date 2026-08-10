//go:build darwin || linux

package supervisor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/state"
)

func Read(layout state.Layout) (Runtime, error) {
	filename := filepath.Join(layout.ProjectDir, "runtime.json")
	info, err := os.Lstat(filename)
	if err != nil {
		return Runtime{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return Runtime{}, errs.New(errs.ExitConflict, "RG609", "runtime record is not a private regular file")
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return Runtime{}, errs.Wrap(errs.ExitConflict, "RG610", "read runtime record", err)
	}
	var result Runtime
	if err := json.Unmarshal(content, &result); err != nil {
		return Runtime{}, errs.Wrap(errs.ExitConflict, "RG611", "decode runtime record", err)
	}
	return result, nil
}

func Write(layout state.Layout, runtimeState Runtime) error {
	content, err := json.MarshalIndent(runtimeState, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG612", "encode runtime record", err)
	}
	return state.WriteFileAtomic(layout.ProjectDir, "runtime.json", append(content, '\n'), 0o600)
}

func Verify(ctx context.Context, layout state.Layout, runtimeState Runtime) error {
	if err := verifyRecordScope(layout, runtimeState); err != nil {
		return err
	}
	identity, command, err := inspectProcess(ctx, runtimeState.PID)
	if err != nil {
		return errs.Wrap(errs.ExitConflict, "RG615", "runtime PID is stale", err)
	}
	if identity != runtimeState.ProcessIdentity || command != runtimeState.ProcessCommand {
		return errs.New(errs.ExitConflict, "RG616", "runtime PID identity no longer matches")
	}
	device, inode, uid, err := inspectSocket(runtimeState.Socket)
	if err != nil {
		return err
	}
	if uid != uint32(os.Getuid()) || device != runtimeState.SocketDevice || inode != runtimeState.SocketInode {
		return errs.New(errs.ExitConflict, "RG617", "runtime Unix socket identity no longer matches")
	}
	configuration, err := os.ReadFile(runtimeState.Configuration)
	if err != nil || state.Hash(configuration) != runtimeState.ConfigurationHash {
		return errs.New(errs.ExitConflict, "RG618", "active Process Compose configuration was modified")
	}
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := Client(layout, runtimeState).Ping(pingContext); err != nil {
		return errs.Wrap(errs.ExitConflict, "RG619", "runtime socket does not answer as the recorded Process Compose server", err)
	}
	return nil
}

func verifyRecordScope(layout state.Layout, runtimeState Runtime) error {
	if runtimeState.APIVersion != "rungrid/output/v1" || runtimeState.ProjectID != layout.ProjectID {
		return errs.New(errs.ExitConflict, "RG613", "runtime record belongs to another project or API")
	}
	expectedSocket := filepath.Join(layout.ProjectDir, "runtime.sock")
	if filepath.Clean(runtimeState.Socket) != expectedSocket {
		return errs.New(errs.ExitConflict, "RG614", "runtime socket path is outside the selected project state")
	}
	return nil
}

func Client(layout state.Layout, runtimeState Runtime) processcompose.Client {
	return processcompose.Client{
		Executable: runtimeState.ProcessCompose,
		Socket:     "runtime.sock",
		LogFile:    processcompose.ClientLog(layout.ProjectDir),
		WorkDir:    layout.ProjectDir,
	}
}

func Stop(ctx context.Context, layout state.Layout, runtimeState Runtime) error {
	if err := Verify(ctx, layout, runtimeState); err != nil {
		return err
	}
	client := Client(layout, runtimeState)
	if err := client.Down(ctx); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for processExists(runtimeState.PID) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if processExists(runtimeState.PID) {
		return errs.New(errs.ExitPartial, "RG620", "Process Compose did not exit after ordered shutdown")
	}
	if info, err := os.Lstat(runtimeState.Socket); err == nil {
		device, inode, uid, statErr := inspectSocket(runtimeState.Socket)
		if statErr != nil || uid != uint32(os.Getuid()) || device != runtimeState.SocketDevice || inode != runtimeState.SocketInode || info.Mode()&os.ModeSocket == 0 {
			return errs.New(errs.ExitConflict, "RG621", "refusing to remove a socket whose identity changed")
		}
		if err := os.Remove(runtimeState.Socket); err != nil {
			return errs.Wrap(errs.ExitPartial, "RG622", "remove stopped runtime socket", err)
		}
	} else if !os.IsNotExist(err) {
		return errs.Wrap(errs.ExitPartial, "RG623", "inspect stopped runtime socket", err)
	}
	if err := os.Remove(filepath.Join(layout.ProjectDir, "runtime.json")); err != nil && !os.IsNotExist(err) {
		return errs.Wrap(errs.ExitPartial, "RG624", "remove stopped runtime record", err)
	}
	return nil
}
