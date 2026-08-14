//go:build darwin || linux

package terminalshell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func watchRuntime(ctx context.Context, options ShellOptions, shutdown chan<- string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		marker := filepath.Join(options.Layout.ProjectDir, "locks", "down-"+options.Runtime.GenerationID+".json")
		if _, err := os.Lstat(marker); err == nil {
			shutdown <- "workspace shutdown began"
			return
		} else if !os.IsNotExist(err) {
			shutdown <- "workspace shutdown state became unverifiable"
			return
		}
		if !supervisor.StaticScopeMatches(options.Layout, options.Runtime) {
			shutdown <- "runtime identity disappeared or changed"
			return
		}
	}
}

func stopManagedShell(command *exec.Cmd, result <-chan error, reason string) error {
	if command.Process != nil {
		_ = command.Process.Signal(syscall.SIGHUP)
	}
	select {
	case <-result:
		return nil
	case <-time.After(3 * time.Second):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-result
		return errs.New(errs.ExitInterrupted, "RG1008", "managed zsh did not exit after "+reason)
	}
}

func WaitGenerationReleased(ctx context.Context, layout state.Layout, generationID string, services []string) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		owned := false
		for _, service := range services {
			if _, live := ActiveTab(layout, generationID, service); live {
				owned = true
				break
			}
		}
		if !owned {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
