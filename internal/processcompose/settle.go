package processcompose

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Settled reports a status in which Process Compose accepts a start request:
// the process is neither running nor in a start or stop transition. A stop
// request returns while the process is still "Terminating", and a start sent
// then is rejected with HTTP 400 "already running".
func Settled(status string) bool {
	normalized := strings.ToLower(status)
	for _, busy := range []string{"running", "launch", "pending", "terminat", "restarting", "waiting"} {
		if strings.Contains(normalized, busy) {
			return false
		}
	}
	return true
}

// StateReader reads one process state.
type StateReader interface {
	Get(ctx context.Context, service string) (ProcessState, error)
}

// WaitSettled polls until the process has fully stopped or the timeout ends.
func WaitSettled(ctx context.Context, reader StateReader, service string, timeout time.Duration) error {
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	last := ""
	for {
		current, err := reader.Get(waitContext, service)
		if err == nil {
			if Settled(current.Status) {
				return nil
			}
			last = current.Status
		}
		select {
		case <-waitContext.Done():
			return fmt.Errorf("%s did not finish stopping (last status %q): %w", service, last, waitContext.Err())
		case <-ticker.C:
		}
	}
}
