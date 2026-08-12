//go:build darwin || linux

package lifecycle

import (
	"context"
	"testing"

	"github.com/jamesonstone/rungrid/internal/processcompose"
)

func TestWaitForServiceStoppedWaitsPastStoppingState(t *testing.T) {
	calls := 0
	get := func(context.Context, string) (processcompose.ProcessState, error) {
		calls++
		if calls == 1 {
			return processcompose.ProcessState{Status: "Stopping"}, nil
		}
		return processcompose.ProcessState{Status: "Completed"}, nil
	}
	if err := waitForServiceStopped(context.Background(), get, "api"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("service state queried %d times, want 2", calls)
	}
}
