package guardstate

type AuthorityScope struct {
	ProjectID                string `json:"project_id"`
	GenerationID             string `json:"generation_id"`
	EffectiveManifestSHA256  string `json:"effective_manifest_sha256"`
	RuntimePID               int    `json:"process_compose_pid"`
	RuntimeProcessIdentity   string `json:"process_compose_start_identity"`
	RuntimeCommandSHA256     string `json:"runtime_command_sha256"`
	SocketPath               string `json:"socket_path"`
	SocketOwnerUID           uint32 `json:"socket_owner_uid"`
	SocketDevice             uint64 `json:"socket_device"`
	SocketInode              uint64 `json:"socket_inode"`
	ProcessComposeConfigHash string `json:"process_compose_config_sha256"`
}

type Metrics struct {
	CPUPercent      float64 `json:"cpu_percent"`
	CPUTotalSeconds float64 `json:"cpu_total_seconds"`
	MemoryPercent   float64 `json:"memory_percent"`
	RSSBytes        uint64  `json:"rss_bytes"`
	Processes       int     `json:"processes"`
	Threads         int     `json:"threads"`
	ThreadGrowth    int     `json:"thread_growth"`
}

type Limits struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	Processes     int     `json:"processes"`
	Threads       int     `json:"threads"`
	ThreadGrowth  int     `json:"thread_growth"`
}

type Baseline struct {
	APIVersion      string         `json:"api_version"`
	Scope           AuthorityScope `json:"scope"`
	Service         string         `json:"service"`
	ServiceIdentity string         `json:"service_identity_sha256"`
	HealthySamples  int64          `json:"healthy_samples"`
	HealthyDuration string         `json:"healthy_duration"`
	P99             Metrics        `json:"p99"`
	Mature          bool           `json:"mature"`
	UpdatedAt       string         `json:"updated_at"`
}

type IncidentSummary struct {
	ID         string  `json:"id"`
	OccurredAt string  `json:"occurred_at"`
	Subject    string  `json:"subject"`
	Tier       string  `json:"tier"`
	Trigger    string  `json:"trigger"`
	Action     string  `json:"action"`
	Metrics    Metrics `json:"metrics"`
}

type Incident struct {
	APIVersion string `json:"api_version"`
	IncidentSummary
	Scope          AuthorityScope `json:"scope"`
	RootPID        int            `json:"root_pid,omitempty"`
	RootIdentity   string         `json:"root_start_identity,omitempty"`
	Limits         Limits         `json:"effective_limits"`
	RestartCount   int            `json:"restart_count"`
	CircuitState   string         `json:"circuit_state"`
	AuthorityValid bool           `json:"authority_valid"`
}

type ServiceStatus struct {
	Name            string           `json:"name"`
	Source          string           `json:"source"`
	Enforcement     string           `json:"enforcement"`
	State           string           `json:"state"`
	AuthorityValid  bool             `json:"authority_valid"`
	DegradedReason  string           `json:"degraded_reason,omitempty"`
	RootPID         int              `json:"root_pid,omitempty"`
	RootIdentity    string           `json:"root_start_identity,omitempty"`
	ServiceIdentity string           `json:"service_identity_sha256"`
	Metrics         Metrics          `json:"metrics"`
	Baseline        Baseline         `json:"baseline"`
	EffectiveLimits Limits           `json:"effective_limits"`
	RestartHistory  []string         `json:"resource_restart_history,omitempty"`
	RestartCount    int              `json:"restart_count"`
	CircuitState    string           `json:"circuit_state"`
	LatestIncident  *IncidentSummary `json:"latest_incident,omitempty"`
}

type Status struct {
	APIVersion            string           `json:"api_version"`
	ProjectID             string           `json:"project_id"`
	GenerationID          string           `json:"generation_id"`
	Scope                 AuthorityScope   `json:"authority_scope"`
	AuthorityValid        bool             `json:"authority_valid"`
	Health                string           `json:"health"`
	HeartbeatAt           string           `json:"heartbeat_at"`
	DegradedReason        string           `json:"degraded_reason,omitempty"`
	Shutdown              bool             `json:"shutdown"`
	GuardPID              int              `json:"guard_pid,omitempty"`
	GuardCPUPercent       float64          `json:"guard_cpu_percent"`
	GuardRSSBytes         uint64           `json:"guard_rss_bytes"`
	GuardThreads          int              `json:"guard_threads"`
	SamplerDurationMS     float64          `json:"sampler_duration_ms"`
	Services              []ServiceStatus  `json:"services"`
	LatestControlIncident *IncidentSummary `json:"latest_control_plane_incident,omitempty"`
}

type ControlClient struct {
	APIVersion      string         `json:"api_version"`
	Scope           AuthorityScope `json:"scope"`
	PID             int            `json:"pid"`
	ProcessIdentity string         `json:"process_identity"`
	PGID            int            `json:"pgid"`
	ParentPID       int            `json:"parent_pid"`
	ParentIdentity  string         `json:"parent_identity"`
	Operation       string         `json:"operation"`
	Service         string         `json:"service,omitempty"`
	StartedAt       string         `json:"started_at"`
	DeadlineAt      string         `json:"deadline_at,omitempty"`
}

type CircuitReset struct {
	APIVersion  string         `json:"api_version"`
	Scope       AuthorityScope `json:"scope"`
	Service     string         `json:"service"`
	RequestedAt string         `json:"requested_at"`
}
