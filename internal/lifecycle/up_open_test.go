//go:build darwin || linux

package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func TestPlanUpOpen(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		input upOpenInput
		want  resumeWindowPlan
	}{
		{"fresh runtime opens the workspace", upOpenInput{}, resumeWindowPlan{Workspace: true}},
		{"a stale live signal cannot block a fresh runtime", upOpenInput{WindowsLive: true}, resumeWindowPlan{Workspace: true}},
		{"reused without windows reopens them", upOpenInput{Reused: true}, resumeWindowPlan{Workspace: true}},
		{"reused with a live window opens nothing", upOpenInput{Reused: true, WindowsLive: true}, resumeWindowPlan{}},
		{"reused with a live window opens only an absent requested tab",
			upOpenInput{Reused: true, WindowsLive: true, AbsentTabs: []string{"worker"}},
			resumeWindowPlan{Tabs: []string{"worker"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := planUpOpen(test.input)
			if got.Workspace != test.want.Workspace || (len(got.Tabs)+len(test.want.Tabs) > 0 && !reflect.DeepEqual(got.Tabs, test.want.Tabs)) {
				t.Fatalf("planUpOpen(%+v) = %+v, want %+v", test.input, got, test.want)
			}
		})
	}
}

func TestAttachResultTreatsAStoppedRuntimeAsACleanEnd(t *testing.T) {
	t.Parallel()
	lost := errors.New("exit status 1")
	if err := attachResult(lost, false); err != nil {
		t.Fatalf("a client that outlived its runtime failed: %v", err)
	}
	if err := attachResult(nil, true); err != nil {
		t.Fatalf("a clean client exit failed: %v", err)
	}
	if err := attachResult(lost, true); err == nil {
		t.Fatal("a client that failed against a live runtime reported success")
	}
}

func TestWatchRuntimeEnd(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ended := watchRuntimeEnd(context.Background(), time.Millisecond, func() bool { return calls.Add(1) < 3 })
	if !ended || calls.Load() != 3 {
		t.Fatalf("ended = %v after %d checks", ended, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if watchRuntimeEnd(ctx, time.Hour, func() bool { return true }) {
		t.Fatal("a canceled watch reported the runtime ended")
	}
}

func TestWindowStartingCoversOnlyThisRuntimesRecentOpen(t *testing.T) {
	t.Parallel()
	layout := state.Layout{ProjectDir: t.TempDir()}
	now := time.Now()
	runtimeState := supervisor.Runtime{PID: 41}
	if windowStarting(layout, runtimeState, now) {
		t.Fatal("no open was recorded")
	}
	if err := recordWindowOpen(layout, runtimeState, now); err != nil {
		t.Fatal(err)
	}
	if !windowStarting(layout, runtimeState, now.Add(windowStartGrace-time.Second)) {
		t.Fatal("a window inside the grace period was not counted")
	}
	if windowStarting(layout, runtimeState, now.Add(windowStartGrace)) {
		t.Fatal("the grace period did not end")
	}
	if windowStarting(layout, supervisor.Runtime{PID: 42}, now) {
		t.Fatal("another runtime's open was counted")
	}
}
