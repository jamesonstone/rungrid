//go:build darwin || linux

package session

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type logFollower struct {
	command      *exec.Cmd
	cancel       context.CancelFunc
	done         chan struct{}
	mu           sync.Mutex
	result       error
	registration *guardstate.ClientRegistration
}

func startLogFollower(layout state.Layout, runtimeState supervisor.Runtime, client processcompose.Client, service string, stdin io.Reader, stdout, stderr io.Writer) (*logFollower, error) {
	ctx, cancel := context.WithCancel(context.Background())
	command := client.LogsCommand(ctx, []string{service}, true, -1, true, stdin, stdout, stderr)
	if err := command.Start(); err != nil {
		cancel()
		return nil, errs.Wrap(errs.ExitFailure, "RG811", "start service log foreground", err)
	}
	registration, err := guardstate.RegisterControlClient(layout, supervisor.AuthorityScope(layout, runtimeState), command, "logs", service, time.Time{})
	if err != nil {
		cancel()
		_ = command.Wait()
		return nil, err
	}
	follower := &logFollower{command: command, cancel: cancel, done: make(chan struct{}), registration: registration}
	go func() {
		err := command.Wait()
		_ = registration.Release()
		follower.mu.Lock()
		follower.result = err
		follower.mu.Unlock()
		close(follower.done)
	}()
	return follower, nil
}

func (f *logFollower) channel() <-chan struct{} {
	if f == nil {
		return nil
	}
	return f.done
}

func (f *logFollower) err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.result
}

func (f *logFollower) stop() error {
	if f == nil {
		return nil
	}
	f.cancel()
	defer func() { _ = f.registration.Release() }()
	select {
	case <-f.done:
		err := f.err()
		if err != nil && !errors.Is(err, context.Canceled) {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				return err
			}
		}
		return nil
	case <-time.After(2 * time.Second):
		if f.command.Process != nil {
			_ = f.command.Process.Signal(os.Interrupt)
		}
		return nil
	}
}
