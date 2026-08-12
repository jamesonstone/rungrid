package resourceguard

import (
	"testing"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

func TestAdaptiveThresholdNeverExceedsEmergencyCeiling(t *testing.T) {
	policy := manifest.AdaptiveLimit{Floor: 5, Multiplier: 3, Headroom: 2}
	for _, test := range []struct {
		p99, expected float64
	}{{0, 5}, {2, 6}, {20, 60}, {40, 75}} {
		if actual := adaptiveThreshold(test.p99, policy, 75); actual != test.expected {
			t.Errorf("adaptiveThreshold(%v)=%v, want %v", test.p99, actual, test.expected)
		}
	}
}

func TestPercentile99AndBreach(t *testing.T) {
	samples := make([]guardstate.Metrics, 100)
	for index := range samples {
		samples[index] = guardstate.Metrics{CPUPercent: float64(index + 1), Threads: index + 1}
	}
	p99 := percentile99(samples)
	if p99.CPUPercent != 99 || p99.Threads != 99 {
		t.Fatalf("unexpected P99: %#v", p99)
	}
	if trigger := breach(guardstate.Metrics{Threads: 101}, guardstate.Limits{CPUPercent: 75, MemoryPercent: 50, Processes: 32, Threads: 100, ThreadGrowth: 10}); trigger != "thread_count" {
		t.Fatalf("unexpected trigger %q", trigger)
	}
	if trigger := breach(guardstate.Metrics{ThreadGrowth: 11}, guardstate.Limits{CPUPercent: 75, MemoryPercent: 50, Processes: 32, Threads: 100, ThreadGrowth: 10}); trigger != "thread_growth" {
		t.Fatalf("unexpected growth trigger %q", trigger)
	}
}
