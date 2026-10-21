package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/update"
)

// processControl is the Process Compose surface a restart needs.
type processControl interface {
	Get(ctx context.Context, service string) (processcompose.ProcessState, error)
	Start(ctx context.Context, service string) error
	Stop(ctx context.Context, service string) error
}

// serviceRestart restarts one service for update. When a session owns the
// service, sessionPause asks it to pause cooperatively; if that pause or the
// matching resume fails, for example because the session is stale, its socket
// broke, or the start raced a terminating process, the restart falls back to
// Process Compose. Every path ends with the service running again or with an
// error that says it could not be started.
type serviceRestart struct {
	name         string
	sessionPause func(context.Context) (func(context.Context) error, error)
	process      processControl
	ready        func(processcompose.ProcessState) bool
	stopTimeout  time.Duration
	startTimeout time.Duration
}

func (r serviceRestart) pause(ctx context.Context) (maintenance.ResumeFunc, error) {
	if r.sessionPause == nil {
		if err := r.stop(ctx); err != nil {
			return nil, errors.Join(err, r.restoreAfterFailedPause())
		}
		return r.ensureRunning, nil
	}
	resume, pauseErr := r.sessionPause(ctx)
	if pauseErr == nil {
		return func(resumeContext context.Context) error {
			resumeErr := resume(resumeContext)
			if resumeErr == nil {
				return nil
			}
			if err := r.ensureRunning(resumeContext); err != nil {
				return errors.Join(resumeErr, err)
			}
			return update.Recovered{Detail: "session resume failed (" + resumeErr.Error() + "); restarted through Process Compose"}
		}, nil
	}
	if err := r.stop(ctx); err != nil {
		return nil, errors.Join(pauseErr, err, r.restoreAfterFailedPause())
	}
	return func(resumeContext context.Context) error {
		if err := r.ensureRunning(resumeContext); err != nil {
			return err
		}
		return update.Recovered{Detail: "session pause failed (" + pauseErr.Error() + "); restarted through Process Compose"}
	}, nil
}

// stop stops the process through Process Compose and waits until it exits.
func (r serviceRestart) stop(ctx context.Context) error {
	if err := r.process.Stop(ctx, r.name); err != nil && !isAlreadyStopped(err) {
		return err
	}
	return processcompose.WaitSettled(ctx, r.process, r.name, r.stopTimeout)
}

// restoreAfterFailedPause makes sure a failed pause never leaves the service
// stopped, using a fresh deadline independent of the caller's context.
func (r serviceRestart) restoreAfterFailedPause() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.stopTimeout+r.startTimeout)
	defer cancel()
	return r.ensureRunning(ctx)
}

// ensureRunning brings the process back to ready from whatever state it is in:
// it adopts a process that is already running, waits out a terminating one,
// and retries a start that Process Compose rejected while the process settled.
func (r serviceRestart) ensureRunning(ctx context.Context) error {
	deadline, cancel := context.WithTimeout(ctx, r.stopTimeout+r.startTimeout)
	defer cancel()
	var startErr error
	for {
		current, err := r.process.Get(deadline, r.name)
		switch {
		case err != nil:
		case r.ready(current):
			return nil
		case processcompose.Settled(current.Status):
			startErr = r.process.Start(deadline, r.name)
		}
		select {
		case <-deadline.Done():
			detail := "did not become ready"
			if startErr != nil {
				detail = "could not be started: " + startErr.Error()
			}
			return fmt.Errorf("%s %s (last status %q)", r.name, detail, strings.TrimSpace(current.Status))
		case <-time.After(250 * time.Millisecond):
		}
	}
}
