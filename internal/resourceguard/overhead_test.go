package resourceguard

import "testing"

func TestTreeOverheadIncludesSamplerChildOnly(t *testing.T) {
	snapshot := processSnapshot{
		10: {PID: 10, PPID: 1, CPUPercent: 0.2, RSSBytes: 100, Threads: 2},
		11: {PID: 11, PPID: 10, CPUPercent: 0.3, RSSBytes: 50, Threads: 1},
		12: {PID: 12, PPID: 1, CPUPercent: 99, RSSBytes: 999, Threads: 99},
	}
	cpu, rss, threads := treeOverhead(snapshot, 10)
	if cpu != 0.5 || rss != 150 || threads != 3 {
		t.Fatalf("unexpected guard tree overhead: %.1f %d %d", cpu, rss, threads)
	}
}
