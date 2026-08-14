//go:build darwin || linux

package terminalshell

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/environment"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/session"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type TabRegistration struct {
	APIVersion      string `json:"api_version"`
	ProjectID       string `json:"project_id"`
	GenerationID    string `json:"generation_id"`
	Service         string `json:"service"`
	PID             int    `json:"pid"`
	ProcessIdentity string `json:"process_identity"`
	PaneID          string `json:"pane_id,omitempty"`
	StartedAt       string `json:"started_at"`
}

type ShellOptions struct {
	Layout   state.Layout
	Runtime  supervisor.Runtime
	Manifest *manifest.Manifest
	Service  string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

func RunShell(ctx context.Context, options ShellOptions) error {
	service, exists := manifest.FindService(options.Manifest, options.Service)
	if !exists || service.Activation != "tab" || service.Source == "external" {
		return errs.New(errs.ExitUsage, "RG1001", "managed shell requires a tab-owned service")
	}
	if err := supervisor.Verify(ctx, options.Layout, options.Runtime); err != nil {
		return err
	}
	tabLock, err := acquireTabLock(options.Layout, options.Runtime.GenerationID, service.Name)
	if err != nil {
		return err
	}
	defer releaseTabLock(tabLock)
	processIdentity, err := procidentity.Current()
	if err != nil {
		return errs.Wrap(errs.ExitConflict, "RG1013", "record managed tab process identity", err)
	}
	registration := TabRegistration{
		APIVersion:      "rungrid/output/v1",
		ProjectID:       options.Layout.ProjectID,
		GenerationID:    options.Runtime.GenerationID,
		Service:         service.Name,
		PID:             os.Getpid(),
		ProcessIdentity: processIdentity,
		PaneID:          os.Getenv("WARP_PANE_ID"),
		StartedAt:       state.RuntimeTimestamp(),
	}
	registrationPath := filepath.Join(options.Layout.ProjectDir, "tabs", options.Runtime.GenerationID+"-"+service.Name+".json")
	if err := writeTabRegistration(registrationPath, registration); err != nil {
		return err
	}
	defer removeTabRegistration(registrationPath, registration)

	rungridExecutable, err := os.Executable()
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1002", "resolve Rungrid executable", err)
	}
	rungridExecutable, err = filepath.EvalSymlinks(rungridExecutable)
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1003", "resolve Rungrid executable symlinks", err)
	}
	zDotDir := filepath.Join(options.Layout.ProjectDir, "generations", options.Runtime.GenerationID, "terminal", "shell", service.Name)
	userZDotDir := os.Getenv("ZDOTDIR")
	if userZDotDir == "" {
		userZDotDir, err = os.UserHomeDir()
		if err != nil {
			return errs.Wrap(errs.ExitFailure, "RG1004", "resolve user zsh configuration directory", err)
		}
	}
	workingDirectory, err := manifest.ServiceWorkingDirectory(options.Manifest, options.Runtime.WorkspaceRoot, service)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "zsh", "-i")
	command.Dir = workingDirectory
	command.Stdin = options.Stdin
	command.Stdout = options.Stdout
	command.Stderr = options.Stderr
	command.Env = replaceEnvironment(os.Environ(), map[string]string{
		"ZDOTDIR":               zDotDir,
		"RUNGRID_USER_ZDOTDIR":  userZDotDir,
		"RUNGRID_SHIM_DIR":      filepath.Join(zDotDir, "bin"),
		"RUNGRID_EXECUTABLE":    rungridExecutable,
		"RUNGRID_STATE_DIR":     options.Layout.StateRoot,
		"RUNGRID_PROJECT_ID":    options.Layout.ProjectID,
		"RUNGRID_GENERATION_ID": options.Runtime.GenerationID,
		"RUNGRID_SERVICE":       service.Name,
	})
	if err := command.Start(); err != nil {
		return errs.Wrap(errs.ExitDependency, "RG1005", "start managed zsh", err)
	}
	watchContext, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	shutdown := make(chan string, 1)
	go watchRuntime(watchContext, options, shutdown)
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(signals)
	result := make(chan error, 1)
	go func() { result <- command.Wait() }()
	for {
		select {
		case err := <-result:
			if err != nil {
				return errs.Wrap(errs.ExitFailure, "RG1006", "managed zsh exited", err)
			}
			return nil
		case received := <-signals:
			if command.Process != nil {
				_ = command.Process.Signal(received)
			}
			if received == os.Interrupt {
				continue
			}
			select {
			case err := <-result:
				if err != nil {
					return errs.Wrap(errs.ExitInterrupted, "RG1007", "managed zsh terminated", err)
				}
				return nil
			case <-time.After(3 * time.Second):
				if command.Process != nil {
					_ = command.Process.Kill()
				}
				return errs.New(errs.ExitInterrupted, "RG1008", "managed zsh did not exit after signal")
			}
		case reason := <-shutdown:
			return stopManagedShell(command, result, reason)
		}
	}
}

func acquireTabLock(layout state.Layout, generationID, service string) (*os.File, error) {
	if err := layout.Ensure(); err != nil {
		return nil, err
	}
	filename := filepath.Join(layout.ProjectDir, "locks", generationID+"-"+service+".tab.lock")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errs.Wrap(errs.ExitConflict, "RG1014", "open managed tab lock", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, errs.Wrap(errs.ExitConflict, "RG1015", "secure managed tab lock", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errs.New(errs.ExitConflict, "RG1016", "service already has a live managed tab")
	}
	return file, nil
}

func releaseTabLock(file *os.File) {
	if file == nil {
		return
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func RunTrigger(ctx context.Context, layout state.Layout, runtimeState supervisor.Runtime, m *manifest.Manifest, serviceName string, arguments []string, stdin io.Reader, stdout, stderr io.Writer) error {
	service, exists := manifest.FindService(m, serviceName)
	if !exists || service.Activation != "tab" || len(service.Terminal.TriggerArgv) == 0 {
		return errs.New(errs.ExitUsage, "RG1009", "trigger requires a configured tab-owned service")
	}
	expected := service.Terminal.TriggerArgv[1:]
	if equalArguments(expected, arguments) {
		return session.Run(ctx, session.Options{
			Layout: layout, Runtime: runtimeState, Manifest: m, Service: serviceName,
			TabID: os.Getenv("WARP_PANE_ID"), Stdin: stdin, Stdout: stdout, Stderr: stderr,
		})
	}
	originalPath := os.Getenv("RUNGRID_ORIGINAL_PATH")
	if originalPath == "" {
		return errs.New(errs.ExitConflict, "RG1010", "managed shell original PATH is missing")
	}
	environmentMap := map[string]string{"PATH": originalPath}
	workingDirectory, err := manifest.ServiceWorkingDirectory(m, runtimeState.WorkspaceRoot, service)
	if err != nil {
		return err
	}
	executable, err := environment.LookPath(service.Terminal.TriggerArgv[0], workingDirectory, environmentMap)
	if err != nil {
		return errs.Wrap(errs.ExitDependency, "RG1011", "resolve original trigger executable", err)
	}
	return syscall.Exec(executable, append([]string{service.Terminal.TriggerArgv[0]}, arguments...), replaceEnvironment(os.Environ(), map[string]string{"PATH": originalPath}))
}

func ActiveTab(layout state.Layout, generationID, service string) (TabRegistration, bool) {
	filename := filepath.Join(layout.ProjectDir, "tabs", generationID+"-"+service+".json")
	content, err := os.ReadFile(filename)
	if err != nil {
		return TabRegistration{}, false
	}
	var registration TabRegistration
	if json.Unmarshal(content, &registration) != nil || registration.ProjectID != layout.ProjectID || registration.GenerationID != generationID || registration.Service != service {
		return TabRegistration{}, false
	}
	if !procidentity.Matches(registration.PID, registration.ProcessIdentity) {
		return TabRegistration{}, false
	}
	return registration, true
}

func writeTabRegistration(filename string, registration TabRegistration) error {
	content, err := json.MarshalIndent(registration, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1012", "encode tab registration", err)
	}
	return state.WriteFileAtomic(filepath.Dir(filename), filepath.Base(filename), append(content, '\n'), 0o600)
}

func removeTabRegistration(filename string, expected TabRegistration) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	var actual TabRegistration
	if json.Unmarshal(content, &actual) == nil && actual == expected {
		_ = os.Remove(filename)
	}
}

func replaceEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, value := range base {
		name := value
		if index := strings.IndexByte(name, '='); index >= 0 {
			name = name[:index]
		}
		if _, replaced := replacements[name]; !replaced {
			result = append(result, value)
		}
	}
	for name, value := range replacements {
		result = append(result, name+"="+value)
	}
	return result
}

func equalArguments(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
