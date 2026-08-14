package resourceguard

import (
	"math"
	"sort"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

const maximumBaselineSamples = 4096

type baselineTracker struct {
	layout          state.Layout
	scope           guardstate.AuthorityScope
	service         string
	identity        string
	learningWindow  time.Duration
	healthyDuration time.Duration
	healthySamples  int64
	persistedP99    guardstate.Metrics
	samples         []guardstate.Metrics
	lastCheckpoint  time.Time
}

func loadBaselineTracker(
	layout state.Layout,
	scope guardstate.AuthorityScope,
	service, identity string,
	learningWindow time.Duration,
) (*baselineTracker, error) {
	tracker := &baselineTracker{
		layout: layout, scope: scope, service: service, identity: identity,
		learningWindow: learningWindow, lastCheckpoint: time.Now(),
	}
	baseline, exists, err := guardstate.ReadBaseline(layout, scope, service, identity)
	if err != nil {
		return nil, err
	}
	if exists {
		tracker.healthySamples = baseline.HealthySamples
		tracker.healthyDuration, _ = time.ParseDuration(baseline.HealthyDuration)
		tracker.persistedP99 = baseline.P99
	}
	return tracker, nil
}

func (b *baselineTracker) AddHealthy(metrics guardstate.Metrics, interval time.Duration) {
	b.healthySamples++
	b.healthyDuration += interval
	if len(b.samples) == maximumBaselineSamples {
		copy(b.samples, b.samples[1:])
		b.samples[len(b.samples)-1] = metrics
	} else {
		b.samples = append(b.samples, metrics)
	}
}

func (b *baselineTracker) Mature() bool { return b.healthyDuration >= b.learningWindow }

func (b *baselineTracker) Snapshot(now time.Time) guardstate.Baseline {
	p99 := percentile99(b.samples)
	if len(b.samples) < maximumBaselineSamples {
		p99 = maxMetrics(p99, b.persistedP99)
	}
	return guardstate.Baseline{
		Scope: b.scope, Service: b.service, ServiceIdentity: b.identity,
		HealthySamples: b.healthySamples, HealthyDuration: b.healthyDuration.String(),
		P99: p99, Mature: b.Mature(), UpdatedAt: now.UTC().Format(time.RFC3339Nano),
	}
}

func (b *baselineTracker) Checkpoint(now time.Time) error {
	if now.Sub(b.lastCheckpoint) < time.Minute {
		return nil
	}
	if err := guardstate.WriteBaseline(b.layout, b.Snapshot(now)); err != nil {
		return err
	}
	b.persistedP99 = b.Snapshot(now).P99
	b.lastCheckpoint = now
	return nil
}

func percentile99(samples []guardstate.Metrics) guardstate.Metrics {
	if len(samples) == 0 {
		return guardstate.Metrics{}
	}
	values := func(value func(guardstate.Metrics) float64) float64 {
		items := make([]float64, len(samples))
		for index, sample := range samples {
			items[index] = value(sample)
		}
		sort.Float64s(items)
		position := int(math.Ceil(float64(len(items))*0.99)) - 1
		if position < 0 {
			position = 0
		}
		return items[position]
	}
	return guardstate.Metrics{
		CPUPercent:    values(func(item guardstate.Metrics) float64 { return item.CPUPercent }),
		MemoryPercent: values(func(item guardstate.Metrics) float64 { return item.MemoryPercent }),
		RSSBytes:      uint64(values(func(item guardstate.Metrics) float64 { return float64(item.RSSBytes) })),
		Processes:     int(values(func(item guardstate.Metrics) float64 { return float64(item.Processes) })),
		Threads:       int(values(func(item guardstate.Metrics) float64 { return float64(item.Threads) })),
		ThreadGrowth:  int(values(func(item guardstate.Metrics) float64 { return float64(item.ThreadGrowth) })),
	}
}

func effectiveLimits(policy manifest.ResourceGuard, baseline guardstate.Baseline) guardstate.Limits {
	return guardstate.Limits{
		CPUPercent:    adaptiveThreshold(baseline.P99.CPUPercent, policy.Sustained.CPU, policy.Emergency.CPUPercent),
		MemoryPercent: adaptiveThreshold(baseline.P99.MemoryPercent, policy.Sustained.Memory, policy.Emergency.MemoryPercent),
		Processes:     int(math.Ceil(adaptiveThreshold(float64(baseline.P99.Processes), policy.Sustained.Processes, float64(policy.Emergency.Processes)))),
		Threads:       int(math.Ceil(adaptiveThreshold(float64(baseline.P99.Threads), policy.Sustained.Threads, float64(policy.Emergency.Threads)))),
		ThreadGrowth:  min(policy.Sustained.ThreadGrowth, policy.Emergency.ThreadGrowth),
	}
}

func emergencyLimits(policy manifest.ResourceGuard) guardstate.Limits {
	return guardstate.Limits{
		CPUPercent: policy.Emergency.CPUPercent, MemoryPercent: policy.Emergency.MemoryPercent,
		Processes: policy.Emergency.Processes, Threads: policy.Emergency.Threads,
		ThreadGrowth: policy.Emergency.ThreadGrowth,
	}
}

func adaptiveThreshold(p99 float64, policy manifest.AdaptiveLimit, ceiling float64) float64 {
	return math.Min(ceiling, math.Max(policy.Floor, math.Max(p99*policy.Multiplier, p99+policy.Headroom)))
}

func breach(metrics guardstate.Metrics, limits guardstate.Limits) string {
	switch {
	case metrics.CPUPercent > limits.CPUPercent:
		return "cpu"
	case metrics.MemoryPercent > limits.MemoryPercent:
		return "memory"
	case metrics.Processes > limits.Processes:
		return "process_count"
	case metrics.Threads > limits.Threads:
		return "thread_count"
	case metrics.ThreadGrowth > limits.ThreadGrowth:
		return "thread_growth"
	default:
		return ""
	}
}

func maxMetrics(left, right guardstate.Metrics) guardstate.Metrics {
	return guardstate.Metrics{
		CPUPercent:    max(left.CPUPercent, right.CPUPercent),
		MemoryPercent: max(left.MemoryPercent, right.MemoryPercent),
		RSSBytes:      max(left.RSSBytes, right.RSSBytes),
		Processes:     max(left.Processes, right.Processes),
		Threads:       max(left.Threads, right.Threads),
		ThreadGrowth:  max(left.ThreadGrowth, right.ThreadGrowth),
	}
}
