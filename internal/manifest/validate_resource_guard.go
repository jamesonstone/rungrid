package manifest

import (
	"fmt"
	"time"
)

type validationAdder func(path, message string)

func validateResourceGuard(policy ResourceGuard, prefix string, add validationAdder) {
	betweenDuration(policy.SampleInterval.Duration, 500*time.Millisecond, 10*time.Second, prefix+".sample_interval", add)
	betweenDuration(policy.LearningWindow.Duration, time.Minute, 24*time.Hour, prefix+".learning_window", add)
	betweenDuration(policy.EmergencyWindow.Duration, 3*policy.SampleInterval.Duration, 30*time.Second, prefix+".emergency_window", add)
	betweenDuration(policy.SustainedWindow.Duration, 30*time.Second, time.Hour, prefix+".sustained_window", add)
	if policy.SustainedWindow.Duration <= policy.EmergencyWindow.Duration {
		add(prefix+".sustained_window", "must be longer than emergency_window")
	}
	if policy.RestartLimit < 1 || policy.RestartLimit > 10 {
		add(prefix+".restart_limit", "must be between 1 and 10")
	}
	betweenDuration(policy.RestartWindow.Duration, time.Minute, 24*time.Hour, prefix+".restart_window", add)
	betweenDuration(policy.BackoffInitial.Duration, 500*time.Millisecond, 10*time.Minute, prefix+".backoff_initial", add)
	betweenDuration(policy.BackoffMaximum.Duration, 500*time.Millisecond, 10*time.Minute, prefix+".backoff_maximum", add)
	if policy.BackoffInitial.Duration > policy.BackoffMaximum.Duration {
		add(prefix+".backoff_initial", "must not exceed backoff_maximum")
	}
	validateEmergencyLimits(policy, prefix+".emergency", add)
	validateAdaptiveLimits(policy, prefix+".sustained", add)
}

func validateEmergencyLimits(policy ResourceGuard, prefix string, add validationAdder) {
	limits := policy.Emergency
	betweenFloat(limits.CPUPercent, 0, 100, prefix+".cpu_percent", add)
	betweenFloat(limits.MemoryPercent, 0, 100, prefix+".memory_percent", add)
	betweenInt(limits.Processes, 1, 65536, prefix+".processes", add)
	betweenInt(limits.Threads, 1, 262144, prefix+".threads", add)
	betweenInt(limits.ThreadGrowth, 1, 262144, prefix+".thread_growth", add)
	betweenDuration(limits.ThreadGrowthWindow.Duration, 3*policy.SampleInterval.Duration, 10*time.Minute, prefix+".thread_growth_window", add)
}

func validateAdaptiveLimits(policy ResourceGuard, prefix string, add validationAdder) {
	limits := policy.Sustained
	validateAdaptiveLimit(limits.CPU, policy.Emergency.CPUPercent, prefix+".cpu", add)
	validateAdaptiveLimit(limits.Memory, policy.Emergency.MemoryPercent, prefix+".memory", add)
	validateAdaptiveLimit(limits.Processes, float64(policy.Emergency.Processes), prefix+".processes", add)
	validateAdaptiveLimit(limits.Threads, float64(policy.Emergency.Threads), prefix+".threads", add)
	betweenInt(limits.ThreadGrowth, 1, policy.Emergency.ThreadGrowth, prefix+".thread_growth", add)
	betweenDuration(limits.ThreadGrowthWindow.Duration, 10*time.Second, time.Hour, prefix+".thread_growth_window", add)
}

func validateAdaptiveLimit(limit AdaptiveLimit, ceiling float64, prefix string, add validationAdder) {
	if limit.Floor <= 0 || limit.Floor > ceiling {
		add(prefix+".floor", fmt.Sprintf("must be positive and not exceed emergency ceiling %.2f", ceiling))
	}
	if limit.Multiplier < 1 || limit.Multiplier > 10 {
		add(prefix+".multiplier", "must be between 1 and 10")
	}
	if limit.Headroom <= 0 || limit.Headroom > ceiling {
		add(prefix+".headroom", fmt.Sprintf("must be positive and not exceed emergency ceiling %.2f", ceiling))
	}
}

func betweenDuration(value, minimum, maximum time.Duration, path string, add validationAdder) {
	if value < minimum || value > maximum {
		add(path, fmt.Sprintf("must be between %s and %s", minimum, maximum))
	}
}

func betweenFloat(value, minimum, maximum float64, path string, add validationAdder) {
	if value <= minimum || value > maximum {
		add(path, fmt.Sprintf("must be greater than %.0f and at most %.0f", minimum, maximum))
	}
}

func betweenInt(value, minimum, maximum int, path string, add validationAdder) {
	if value < minimum || value > maximum {
		add(path, fmt.Sprintf("must be between %d and %d", minimum, maximum))
	}
}
