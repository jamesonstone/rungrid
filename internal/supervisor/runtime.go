//go:build darwin || linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

type Runtime struct {
	APIVersion              string `json:"api_version"`
	ProjectID               string `json:"project_id"`
	GenerationID            string `json:"generation_id"`
	EffectiveManifestSHA256 string `json:"effective_manifest_sha256"`
	PID                     int    `json:"pid"`
	ProcessIdentity         string `json:"process_identity"`
	ProcessCommand          string `json:"process_command"`
	Socket                  string `json:"socket"`
	SocketDevice            uint64 `json:"socket_device"`
	SocketInode             uint64 `json:"socket_inode"`
	SocketOwnerUID          uint32 `json:"socket_owner_uid"`
	ProcessCompose          string `json:"process_compose"`
	ProcessComposeVersion   string `json:"process_compose_version"`
	Configuration           string `json:"configuration"`
	ConfigurationHash       string `json:"configuration_hash"`
	WorkspaceRoot           string `json:"workspace_root"`
	StartedAt               string `json:"started_at"`
}

type StartOptions struct {
	Layout                  state.Layout
	GenerationID            string
	EffectiveManifestSHA256 string
	WorkspaceRoot           string
	ProcessCompose          string
	ProcessComposeVersion   string
	RungridExecutable       string
	StartupTimeout          time.Duration
}

func Start(ctx context.Context, options StartOptions) (result Runtime, reused bool, returnErr error) {
	if err := options.Layout.Ensure(); err != nil {
		return Runtime{}, false, err
	}
	if existing, err := Read(options.Layout); err == nil {
		if existing.GenerationID != options.GenerationID {
			return Runtime{}, false, errs.New(errs.ExitConflict, "RG601", "a different generated runtime is active; run rungrid down first")
		}
		if err := Verify(ctx, options.Layout, existing); err != nil {
			return Runtime{}, false, err
		}
		return existing, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Runtime{}, false, err
	}

	generationDirectory := filepath.Join(options.Layout.ProjectDir, "generations", options.GenerationID)
	configuration := filepath.Join(generationDirectory, "process-compose.yaml")
	configurationContent, err := os.ReadFile(configuration)
	if err != nil {
		return Runtime{}, false, errs.Wrap(errs.ExitConflict, "RG602", "read generated Process Compose configuration", err)
	}
	manifestContent, err := os.ReadFile(filepath.Join(generationDirectory, "manifest.yaml"))
	if err != nil {
		return Runtime{}, false, errs.Wrap(errs.ExitConflict, "RG602", "read generated effective manifest", err)
	}
	if options.EffectiveManifestSHA256 == "" || state.Hash(manifestContent) != options.EffectiveManifestSHA256 {
		return Runtime{}, false, errs.New(errs.ExitConflict, "RG602", "generated effective manifest hash does not match startup scope")
	}
	socket := filepath.Join(options.Layout.ProjectDir, "runtime.sock")
	if _, err := os.Lstat(socket); err == nil {
		return Runtime{}, false, errs.New(errs.ExitConflict, "RG604", "an unverified runtime socket already exists; run rungrid doctor")
	} else if !os.IsNotExist(err) {
		return Runtime{}, false, errs.Wrap(errs.ExitConflict, "RG605", "inspect runtime socket", err)
	}
	serverLog := processcompose.InternalLog()
	arguments := []string{
		"-D", "-t=false", "-U", "-u", filepath.Join("..", "..", "runtime.sock"),
		"-f", configuration, "--keep-project", "--ordered-shutdown", "--disable-dotenv", "-L", serverLog,
	}
	command := exec.CommandContext(ctx, options.ProcessCompose, arguments...)
	command.Dir = generationDirectory
	command.Env = processcompose.EnvironmentWithRuntime(os.Environ(), options.RungridExecutable, options.Layout.StateRoot, options.WorkspaceRoot, options.GenerationID)
	output, err := subprocess.Combined(command)
	if err != nil {
		message := "start detached Process Compose runtime"
		if len(strings.TrimSpace(string(output))) > 0 {
			message += " (subprocess output redacted)"
		}
		return Runtime{}, false, errs.Wrap(errs.ExitDependency, "RG606", message, err)
	}
	launched := true
	defer func() {
		if !launched || returnErr == nil {
			return
		}
		cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cleanupClient := processcompose.Client{Executable: options.ProcessCompose, Socket: "runtime.sock", LogFile: processcompose.InternalLog(), WorkDir: options.Layout.ProjectDir}
		if cleanupClient.Down(cleanupContext) == nil {
			if _, _, uid, socketErr := inspectSocket(socket); socketErr == nil && uid == uint32(os.Getuid()) {
				_ = os.Remove(socket)
			}
			_ = os.Remove(filepath.Join(options.Layout.ProjectDir, "runtime.json"))
		}
		_ = processcompose.RemoveSocketAlias(socket)
	}()
	waitContext, cancel := context.WithTimeout(ctx, options.StartupTimeout)
	defer cancel()
	if err := waitForSocket(waitContext, socket); err != nil {
		return Runtime{}, false, err
	}
	pid, err := socketPID(waitContext, socket, configuration)
	if err != nil {
		return Runtime{}, false, err
	}
	processIdentity, processCommand, err := inspectProcess(waitContext, pid)
	if err != nil {
		return Runtime{}, false, err
	}
	device, inode, uid, err := inspectSocket(socket)
	if err != nil {
		return Runtime{}, false, err
	}
	if uid != uint32(os.Getuid()) {
		return Runtime{}, false, errs.New(errs.ExitConflict, "RG607", "runtime socket is not owned by the current user")
	}
	runtimeState := Runtime{
		APIVersion: "rungrid/output/v1", ProjectID: options.Layout.ProjectID, GenerationID: options.GenerationID,
		EffectiveManifestSHA256: options.EffectiveManifestSHA256,
		PID:                     pid, ProcessIdentity: processIdentity, ProcessCommand: processCommand, Socket: socket,
		SocketDevice: device, SocketInode: inode, SocketOwnerUID: uid, ProcessCompose: options.ProcessCompose,
		ProcessComposeVersion: options.ProcessComposeVersion, Configuration: configuration,
		ConfigurationHash: state.Hash(configurationContent), WorkspaceRoot: options.WorkspaceRoot, StartedAt: state.RuntimeTimestamp(),
	}
	if err := Write(options.Layout, runtimeState); err != nil {
		return Runtime{}, false, err
	}
	client := Client(options.Layout, runtimeState)
	pingContext, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := client.Ping(pingContext); err != nil {
		return Runtime{}, false, errs.Wrap(errs.ExitConflict, "RG608", "detached Process Compose runtime did not answer on its recorded socket", err)
	}
	launched = false
	return runtimeState, false, nil
}
