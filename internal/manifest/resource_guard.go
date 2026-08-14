package manifest

import "time"

type ResourceGuard struct {
	SampleInterval  Duration       `yaml:"sample_interval,omitempty" json:"sample_interval"`
	LearningWindow  Duration       `yaml:"learning_window,omitempty" json:"learning_window"`
	EmergencyWindow Duration       `yaml:"emergency_window,omitempty" json:"emergency_window"`
	SustainedWindow Duration       `yaml:"sustained_window,omitempty" json:"sustained_window"`
	RestartLimit    int            `yaml:"restart_limit,omitempty" json:"restart_limit"`
	RestartWindow   Duration       `yaml:"restart_window,omitempty" json:"restart_window"`
	BackoffInitial  Duration       `yaml:"backoff_initial,omitempty" json:"backoff_initial"`
	BackoffMaximum  Duration       `yaml:"backoff_maximum,omitempty" json:"backoff_maximum"`
	Emergency       ResourceLimits `yaml:"emergency,omitempty" json:"emergency"`
	Sustained       AdaptiveLimits `yaml:"sustained,omitempty" json:"sustained"`
}

type ResourceLimits struct {
	CPUPercent         float64  `yaml:"cpu_percent,omitempty" json:"cpu_percent"`
	MemoryPercent      float64  `yaml:"memory_percent,omitempty" json:"memory_percent"`
	Processes          int      `yaml:"processes,omitempty" json:"processes"`
	Threads            int      `yaml:"threads,omitempty" json:"threads"`
	ThreadGrowth       int      `yaml:"thread_growth,omitempty" json:"thread_growth"`
	ThreadGrowthWindow Duration `yaml:"thread_growth_window,omitempty" json:"thread_growth_window"`
}

type AdaptiveLimits struct {
	CPU                AdaptiveLimit `yaml:"cpu,omitempty" json:"cpu"`
	Memory             AdaptiveLimit `yaml:"memory,omitempty" json:"memory"`
	Processes          AdaptiveLimit `yaml:"processes,omitempty" json:"processes"`
	Threads            AdaptiveLimit `yaml:"threads,omitempty" json:"threads"`
	ThreadGrowth       int           `yaml:"thread_growth,omitempty" json:"thread_growth"`
	ThreadGrowthWindow Duration      `yaml:"thread_growth_window,omitempty" json:"thread_growth_window"`
}

type AdaptiveLimit struct {
	Floor      float64 `yaml:"floor,omitempty" json:"floor"`
	Multiplier float64 `yaml:"multiplier,omitempty" json:"multiplier"`
	Headroom   float64 `yaml:"headroom,omitempty" json:"headroom"`
}

func DefaultResourceGuard() ResourceGuard {
	return ResourceGuard{
		SampleInterval: Duration{Duration: time.Second}, LearningWindow: Duration{Duration: 15 * time.Minute},
		EmergencyWindow: Duration{Duration: 3 * time.Second}, SustainedWindow: Duration{Duration: time.Minute},
		RestartLimit: 3, RestartWindow: Duration{Duration: time.Hour},
		BackoffInitial: Duration{Duration: 2 * time.Second}, BackoffMaximum: Duration{Duration: 8 * time.Second},
		Emergency: ResourceLimits{
			CPUPercent: 75, MemoryPercent: 50, Processes: 512, Threads: 2048,
			ThreadGrowth: 512, ThreadGrowthWindow: Duration{Duration: 10 * time.Second},
		},
		Sustained: AdaptiveLimits{
			CPU:          AdaptiveLimit{Floor: 5, Multiplier: 3, Headroom: 2},
			Memory:       AdaptiveLimit{Floor: 5, Multiplier: 1.5, Headroom: 2},
			Processes:    AdaptiveLimit{Floor: 32, Multiplier: 2, Headroom: 16},
			Threads:      AdaptiveLimit{Floor: 128, Multiplier: 2, Headroom: 64},
			ThreadGrowth: 128, ThreadGrowthWindow: Duration{Duration: time.Minute},
		},
	}
}

func EffectiveServiceResourceGuard(runtime ResourceGuard, service *Service) ResourceGuard {
	if service.ResourceGuard == nil {
		return runtime
	}
	return EffectiveResourceGuard(runtime, *service.ResourceGuard)
}

func EffectiveResourceGuard(base, override ResourceGuard) ResourceGuard {
	result := base
	setDuration(&result.SampleInterval, override.SampleInterval)
	setDuration(&result.LearningWindow, override.LearningWindow)
	setDuration(&result.EmergencyWindow, override.EmergencyWindow)
	setDuration(&result.SustainedWindow, override.SustainedWindow)
	setInt(&result.RestartLimit, override.RestartLimit)
	setDuration(&result.RestartWindow, override.RestartWindow)
	setDuration(&result.BackoffInitial, override.BackoffInitial)
	setDuration(&result.BackoffMaximum, override.BackoffMaximum)
	mergeResourceLimits(&result.Emergency, override.Emergency)
	mergeAdaptiveLimits(&result.Sustained, override.Sustained)
	return result
}

func mergeResourceLimits(result *ResourceLimits, override ResourceLimits) {
	setFloat(&result.CPUPercent, override.CPUPercent)
	setFloat(&result.MemoryPercent, override.MemoryPercent)
	setInt(&result.Processes, override.Processes)
	setInt(&result.Threads, override.Threads)
	setInt(&result.ThreadGrowth, override.ThreadGrowth)
	setDuration(&result.ThreadGrowthWindow, override.ThreadGrowthWindow)
}

func mergeAdaptiveLimits(result *AdaptiveLimits, override AdaptiveLimits) {
	mergeAdaptiveLimit(&result.CPU, override.CPU)
	mergeAdaptiveLimit(&result.Memory, override.Memory)
	mergeAdaptiveLimit(&result.Processes, override.Processes)
	mergeAdaptiveLimit(&result.Threads, override.Threads)
	setInt(&result.ThreadGrowth, override.ThreadGrowth)
	setDuration(&result.ThreadGrowthWindow, override.ThreadGrowthWindow)
}

func mergeAdaptiveLimit(result *AdaptiveLimit, override AdaptiveLimit) {
	setFloat(&result.Floor, override.Floor)
	setFloat(&result.Multiplier, override.Multiplier)
	setFloat(&result.Headroom, override.Headroom)
}

func setDuration(result *Duration, value Duration) {
	if value.Duration != 0 {
		*result = value
	}
}

func setFloat(result *float64, value float64) {
	if value != 0 {
		*result = value
	}
}

func setInt(result *int, value int) {
	if value != 0 {
		*result = value
	}
}
