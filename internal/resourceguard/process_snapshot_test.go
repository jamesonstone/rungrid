//go:build darwin || linux

package resourceguard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
)

func TestBuildCandidateRequiresRuntimeAncestryAndOwnedGroups(t *testing.T) {
	snapshot := processSnapshot{
		10: {PID: 10, PPID: 1, PGID: 10, StartIdentity: "runtime"},
		20: {PID: 20, PPID: 10, PGID: 20, StartIdentity: "root", CPUPercent: 140, RSSBytes: 1000, Threads: 4},
		21: {PID: 21, PPID: 20, PGID: 20, StartIdentity: "child", CPUPercent: 20, RSSBytes: 500, Threads: 2},
	}
	candidate, err := buildCandidate(snapshot, 20, 10, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Root.StartIdentity != "root" || candidate.Metrics.Processes != 2 || candidate.Metrics.Threads != 6 {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	if candidate.Metrics.RSSBytes != 1500 || candidate.Metrics.MemoryPercent != 15 {
		t.Fatalf("unexpected candidate metrics: %#v", candidate.Metrics)
	}

	unowned := snapshot
	unowned[30] = processInfo{PID: 30, PPID: 1, PGID: 20, StartIdentity: "manual"}
	if _, err := buildCandidate(unowned, 20, 10, 10_000); err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("expected mixed process-group refusal, got %v", err)
	}
	delete(unowned, 30)
	unowned[20] = processInfo{PID: 20, PPID: 1, PGID: 20, StartIdentity: "root"}
	if _, err := buildCandidate(unowned, 20, 10, 10_000); err == nil || !strings.Contains(err.Error(), "descend") {
		t.Fatalf("expected ancestry refusal, got %v", err)
	}
}

func TestParseProcessSnapshot(t *testing.T) {
	content := []byte("20 10 20 Wed Aug 12 16:10:12 2026 1:02.50 1064.5 1024 77\n")
	snapshot, err := parseProcessSnapshot(content, 1<<30, false)
	if err != nil {
		t.Fatal(err)
	}
	process := snapshot[20]
	if process.PID != 20 || process.PPID != 10 || process.PGID != 20 || process.CPUTotalSeconds != 62.5 || process.CPUPercent != 1064.5 || process.RSSBytes != 1024*1024 || process.Threads != 77 {
		t.Fatalf("unexpected process: %#v", process)
	}
	if process.StartIdentity != "Wed Aug 12 16:10:12 2026" {
		t.Fatalf("unexpected start identity %q", process.StartIdentity)
	}
}

func TestParseDarwinThreadRows(t *testing.T) {
	content := []byte(`user 20 ?? 0.0 S 31T 0:00 0:00 command 20 10 20 Wed Aug 12 16:10:12 2026 1:02.50 12.5 1024
     20 0.5 S 20T 0:00 0:00 20 10 20 Wed Aug 12 16:10:12 2026 1:02.50 7.5 1024
`)
	snapshot, err := parseProcessSnapshot(content, 1<<30, true)
	if err != nil {
		t.Fatal(err)
	}
	process := snapshot[20]
	if process.Threads != 2 || process.CPUPercent != 20 || process.RSSBytes != 1024*1024 {
		t.Fatalf("unexpected aggregated Darwin process: %#v", process)
	}
}

func TestControlCandidateRejectsPIDReuseAndChangedParent(t *testing.T) {
	registration := guardstate.ControlClient{
		PID: 20, ProcessIdentity: "client", PGID: 20,
		ParentPID: 10, ParentIdentity: "rungrid",
	}
	base := processSnapshot{
		10: {PID: 10, PPID: 1, PGID: 10, StartIdentity: "rungrid"},
		20: {PID: 20, PPID: 10, PGID: 20, StartIdentity: "client"},
	}
	if _, err := buildControlCandidate(base, registration, 10_000); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(processSnapshot){
		"pid reuse":        func(value processSnapshot) { item := value[20]; item.StartIdentity = "reused"; value[20] = item },
		"changed parent":   func(value processSnapshot) { item := value[20]; item.PPID = 1; value[20] = item },
		"parent pid reuse": func(value processSnapshot) { item := value[10]; item.StartIdentity = "other"; value[10] = item },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := processSnapshot{10: base[10], 20: base[20]}
			mutate(candidate)
			if _, err := buildControlCandidate(candidate, registration, 10_000); err == nil {
				t.Fatal("changed control-client identity retained authority")
			}
		})
	}
}

func TestParseCPUTime(t *testing.T) {
	for input, expected := range map[string]float64{"0:00.25": 0.25, "1:02.50": 62.5, "02:03:04": 7384, "1-02:03:04": 93784} {
		actual, err := parseCPUTime(input)
		if err != nil || actual != expected {
			t.Errorf("parseCPUTime(%q)=%v,%v want %v", input, actual, err, expected)
		}
	}
}

func TestSnapshotContextUsesBoundedFullInterval(t *testing.T) {
	for name, test := range map[string]struct {
		interval time.Duration
		expected time.Duration
	}{
		"minimum":  {interval: 100 * time.Millisecond, expected: 500 * time.Millisecond},
		"interval": {interval: time.Second, expected: time.Second},
		"maximum":  {interval: 10 * time.Second, expected: 2 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := snapshotContext(context.Background(), test.interval)
			defer cancel()
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("snapshot context has no deadline")
			}
			remaining := time.Until(deadline)
			if remaining < test.expected-100*time.Millisecond || remaining > test.expected {
				t.Fatalf("snapshot deadline remaining %s, want %s", remaining, test.expected)
			}
		})
	}
}
