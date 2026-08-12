package resourceguard

import (
	"math"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
)

type metricPoint struct {
	at      time.Time
	threads int
}

type observation struct {
	candidate processCandidate
	metrics   guardstate.Metrics
	limits    guardstate.Limits
	trigger   string
	tier      string
}

type serviceMonitor struct {
	service          *manifest.Service
	policy           manifest.ResourceGuard
	identity         string
	baseline         *baselineTracker
	current          guardstate.Metrics
	effective        guardstate.Limits
	authorityValid   bool
	degradedReason   string
	rootPID          int
	rootIdentity     string
	state            string
	emergencySamples int
	sustainedSince   time.Time
	threadHistory    []metricPoint
	restartHistory   []time.Time
	circuitState     string
	latestIncident   *guardstate.IncidentSummary
}

func (m *serviceMonitor) observe(
	now time.Time,
	interval time.Duration,
	snapshot processSnapshot,
	current processcompose.ProcessState,
	runtimePID int,
	hostMemory uint64,
) observation {
	m.authorityValid = false
	m.degradedReason = ""
	m.rootPID, m.rootIdentity = 0, ""
	if m.circuitState == "" {
		m.circuitState = "closed"
	}
	m.pruneRestarts(now)
	if m.service.Source == "external" {
		m.state = "observe_only"
		m.current = guardstate.Metrics{}
		m.effective = emergencyLimits(m.policy)
		return observation{}
	}
	if current.PID <= 1 || !isProcessRunning(current.Status) {
		m.state = "inactive"
		m.current = guardstate.Metrics{}
		m.effective = effectiveLimits(m.policy, m.baseline.Snapshot(now))
		m.emergencySamples = 0
		m.sustainedSince = time.Time{}
		return observation{}
	}
	candidate, err := buildCandidate(snapshot, current.PID, runtimePID, hostMemory)
	if err != nil {
		m.state = "degraded"
		m.degradedReason = err.Error()
		m.current = guardstate.Metrics{}
		m.effective = emergencyLimits(m.policy)
		return observation{}
	}
	m.authorityValid = true
	m.rootPID, m.rootIdentity = candidate.Root.PID, candidate.Root.StartIdentity
	m.threadHistory = append(m.threadHistory, metricPoint{at: now, threads: candidate.Metrics.Threads})
	m.pruneThreadHistory(now)
	emergencyMetrics := candidate.Metrics
	emergencyMetrics.ThreadGrowth = m.threadGrowth(now, m.policy.Emergency.ThreadGrowthWindow.Duration)
	sustainedMetrics := candidate.Metrics
	sustainedMetrics.ThreadGrowth = m.threadGrowth(now, m.policy.Sustained.ThreadGrowthWindow.Duration)
	m.current = sustainedMetrics
	emergency := emergencyLimits(m.policy)
	emergencyTrigger := breach(emergencyMetrics, emergency)
	if emergencyTrigger == "" {
		m.emergencySamples = 0
	} else {
		m.emergencySamples++
	}
	baseline := m.baseline.Snapshot(now)
	m.effective = effectiveLimits(m.policy, baseline)
	sustainedTrigger := ""
	if baseline.Mature {
		sustainedTrigger = breach(sustainedMetrics, m.effective)
	}
	// Once mature, do not teach an already abnormal sample back into the
	// baseline before its sustained window can elapse.
	if healthyState(m.service, current) && emergencyTrigger == "" && sustainedTrigger == "" {
		m.baseline.AddHealthy(sustainedMetrics, interval)
		baseline = m.baseline.Snapshot(now)
		m.effective = effectiveLimits(m.policy, baseline)
		if baseline.Mature {
			sustainedTrigger = breach(sustainedMetrics, m.effective)
		}
	}
	if m.circuitState == "open" {
		m.state = "circuit_open"
		return observation{}
	}
	needed := int(math.Ceil(float64(m.policy.EmergencyWindow.Duration) / float64(interval)))
	if emergencyTrigger != "" && m.emergencySamples >= needed {
		m.state = "breached"
		return observation{candidate: candidate, metrics: emergencyMetrics, limits: emergency, trigger: emergencyTrigger, tier: "emergency"}
	}
	if baseline.Mature {
		if sustainedTrigger == "" {
			m.sustainedSince = time.Time{}
		} else if m.sustainedSince.IsZero() {
			m.sustainedSince = now
		} else if now.Sub(m.sustainedSince)+interval >= m.policy.SustainedWindow.Duration {
			m.state = "breached"
			return observation{candidate: candidate, metrics: sustainedMetrics, limits: m.effective, trigger: sustainedTrigger, tier: "sustained"}
		}
	}
	if baseline.Mature {
		m.state = "monitoring"
	} else {
		m.state = "learning"
	}
	return observation{}
}

func (m *serviceMonitor) status(now time.Time) guardstate.ServiceStatus {
	enforcement := "enforce"
	if m.service.Source == "external" {
		enforcement = "observe_only"
	}
	history := make([]string, len(m.restartHistory))
	for index, item := range m.restartHistory {
		history[index] = item.UTC().Format(time.RFC3339Nano)
	}
	return guardstate.ServiceStatus{
		Name: m.service.Name, Source: m.service.Source, Enforcement: enforcement,
		State: m.state, AuthorityValid: m.authorityValid, DegradedReason: m.degradedReason,
		RootPID: m.rootPID, RootIdentity: m.rootIdentity, ServiceIdentity: m.identity,
		Metrics: m.current, Baseline: m.baseline.Snapshot(now), EffectiveLimits: m.effective,
		RestartHistory: history, RestartCount: len(history), CircuitState: m.circuitState,
		LatestIncident: m.latestIncident,
	}
}

func (m *serviceMonitor) restore(previous guardstate.Status) {
	for _, item := range previous.Services {
		if item.Name != m.service.Name || item.ServiceIdentity != m.identity {
			continue
		}
		for _, value := range item.RestartHistory {
			if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
				m.restartHistory = append(m.restartHistory, parsed)
			}
		}
		m.circuitState = item.CircuitState
		m.latestIncident = item.LatestIncident
		return
	}
}

func (m *serviceMonitor) threadGrowth(now time.Time, window time.Duration) int {
	current := m.current.Threads
	if len(m.threadHistory) > 0 {
		current = m.threadHistory[len(m.threadHistory)-1].threads
	}
	oldest := current
	for _, point := range m.threadHistory {
		if !point.at.Before(now.Add(-window)) {
			oldest = point.threads
			break
		}
	}
	return max(0, current-oldest)
}

func (m *serviceMonitor) pruneThreadHistory(now time.Time) {
	window := max(m.policy.Emergency.ThreadGrowthWindow.Duration, m.policy.Sustained.ThreadGrowthWindow.Duration)
	cutoff := now.Add(-window - m.policy.SampleInterval.Duration)
	first := 0
	for first < len(m.threadHistory) && m.threadHistory[first].at.Before(cutoff) {
		first++
	}
	m.threadHistory = append([]metricPoint(nil), m.threadHistory[first:]...)
}

func (m *serviceMonitor) pruneRestarts(now time.Time) {
	cutoff := now.Add(-m.policy.RestartWindow.Duration)
	first := 0
	for first < len(m.restartHistory) && m.restartHistory[first].Before(cutoff) {
		first++
	}
	m.restartHistory = append([]time.Time(nil), m.restartHistory[first:]...)
}

func (m *serviceMonitor) nextBackoff() time.Duration {
	backoff := m.policy.BackoffInitial.Duration
	for index := 0; index < len(m.restartHistory); index++ {
		backoff *= 2
		if backoff >= m.policy.BackoffMaximum.Duration {
			return m.policy.BackoffMaximum.Duration
		}
	}
	return backoff
}

func isProcessRunning(status string) bool {
	value := strings.ToLower(status)
	return strings.Contains(value, "running") || strings.Contains(value, "launch") || strings.Contains(value, "pending") || strings.Contains(value, "restart")
}
