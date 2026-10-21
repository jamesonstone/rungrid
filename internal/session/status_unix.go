//go:build darwin || linux

package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/processcompose"
)

func isRunning(status string) bool {
	normalized := strings.ToLower(status)
	return strings.Contains(normalized, "running") || strings.Contains(normalized, "launch") || strings.Contains(normalized, "pending")
}

// awaitingStart reports a start that Process Compose has accepted but not yet
// run: a disabled tab process whose dependencies are not ready keeps the
// status "Disabled" until they are, then starts. Before the process has run,
// that status means the session must keep waiting, not that it has ended.
func awaitingStart(status string, started bool) bool {
	return !started && strings.Contains(strings.ToLower(status), "disabled")
}

// stopRequestedSince reports a `rungrid stop` recorded after the session
// began, so an operator can still end a session whose start is queued. The
// path matches lifecycle's generation-scoped stop intent record.
func stopRequestedSince(options Options, since time.Time) bool {
	info, err := os.Lstat(filepath.Join(options.Layout.ProjectDir, "stopped", options.Runtime.GenerationID+"-"+options.Service))
	return err == nil && info.Mode().IsRegular() && info.ModTime().After(since)
}

func isTerminal(status string) bool {
	normalized := strings.ToLower(status)
	return strings.Contains(normalized, "complete") || strings.Contains(normalized, "stopped") || strings.Contains(normalized, "disabled") || strings.Contains(normalized, "error") || strings.Contains(normalized, "skipped")
}

type queueAction int

const (
	queueNone queueAction = iota
	queueWait
	queueStopLaunched
	queueEnd
)

// queuedStart tracks a session whose start request Process Compose accepted
// but has not run yet because dependencies are not ready. It acts only until
// the process first runs; after that the session's ordinary rules apply.
type queuedStart struct {
	since        time.Time
	settled      bool
	announced    bool
	stopDeadline time.Time
}

// decide keeps the session while its start is queued. If the operator runs
// `rungrid stop` meanwhile, Process Compose still keeps the queued start, so
// the session waits, within the startup timeout, to stop it when it launches.
func (q *queuedStart) decide(options Options, status string, started bool, now time.Time) queueAction {
	if started || q.settled {
		return queueNone
	}
	if stopRequestedSince(options, q.since) {
		if isRunning(status) {
			return queueStopLaunched
		}
		if q.stopDeadline.IsZero() {
			q.stopDeadline = now.Add(options.Manifest.Runtime.StartupTimeout.Duration)
			_, _ = fmt.Fprintf(options.Stdout, "[rungrid] %s was stopped before it started; stopping it if its queued start launches\n", options.Service)
		}
		if !awaitingStart(status, false) || now.After(q.stopDeadline) {
			return queueEnd
		}
		return queueWait
	}
	if awaitingStart(status, false) {
		if !q.announced {
			q.announced = true
			_, _ = fmt.Fprintf(options.Stdout, "[rungrid] %s is waiting for its dependencies before it starts\n", options.Service)
		}
		return queueWait
	}
	if isRunning(status) {
		q.settled = true
	}
	return queueNone
}

// finishQueuedStart ends a session whose queued start was stopped, stopping
// the process first when the queued start has launched.
func finishQueuedStart(action queueAction, options Options, client processcompose.Client) error {
	if action != queueStopLaunched {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), options.Manifest.Runtime.ShutdownTimeout.Duration)
	defer cancel()
	return client.Stop(ctx, options.Service)
}
