package manifest

import "time"

func (m *Manifest) ApplyDefaults() {
	if m.Project.Slug == "" {
		m.Project.Slug = Slug(m.Project.Name)
	}
	if m.Workspace.Root == "" {
		m.Workspace.Root = "."
	}
	if m.Runtime.StartupTimeout.Duration == 0 {
		m.Runtime.StartupTimeout.Duration = 45 * time.Second
	}
	if m.Runtime.ShutdownTimeout.Duration == 0 {
		m.Runtime.ShutdownTimeout.Duration = 20 * time.Second
	}
	if m.Runtime.LogRetention.Duration == 0 {
		m.Runtime.LogRetention.Duration = 168 * time.Hour
	}
	if m.Runtime.ProcessCompose.Executable == "" {
		m.Runtime.ProcessCompose.Executable = "process-compose"
	}
	if m.Runtime.ProcessCompose.LogLevel == "" {
		m.Runtime.ProcessCompose.LogLevel = "info"
	}
	if m.Runtime.ProcessCompose.LogRotation.MaxSizeMB == 0 {
		m.Runtime.ProcessCompose.LogRotation.MaxSizeMB = 10
	}
	if m.Runtime.ProcessCompose.LogRotation.MaxBackups == 0 {
		m.Runtime.ProcessCompose.LogRotation.MaxBackups = 1
	}
	m.Runtime.ResourceGuard = EffectiveResourceGuard(DefaultResourceGuard(), m.Runtime.ResourceGuard)
	if m.Terminal.Mode == "" {
		m.Terminal.Mode = "warp"
	}
	if m.Terminal.Open == nil {
		m.Terminal.Open = Bool(m.Terminal.Mode == "warp")
	}
	for name, repository := range m.Repositories {
		if repository.Remote == "" {
			repository.Remote = "origin"
		}
		m.Repositories[name] = repository
	}
	applyLifecycleDefaults(m.Lifecycle.BeforeUp, m.Runtime.StartupTimeout.Duration)
	applyLifecycleDefaults(m.Lifecycle.AfterDown, m.Runtime.ShutdownTimeout.Duration)
	for i := range m.Services {
		applyServiceDefaults(&m.Services[i])
	}
}

func applyServiceDefaults(service *Service) {
	if service.Repository == "" {
		service.Repository = WorkspaceRepository
	}
	if service.Activation == "" {
		if service.Source == "external" {
			service.Activation = "workspace"
		} else {
			service.Activation = "tab"
		}
	}
	if service.WorkingDirectory == "" {
		service.WorkingDirectory = "."
	}
	applyComposeDefaults(service)
	applyServiceRestartDefaults(service)
	if service.Terminal.Title == "" {
		service.Terminal.Title = service.Name
	}
	if service.Terminal.IncludeInVersions == nil {
		service.Terminal.IncludeInVersions = Bool(true)
	}
	if service.Activation == "tab" && len(service.Terminal.TriggerArgv) == 0 && service.Run != nil {
		service.Terminal.TriggerArgv = append([]string(nil), service.Run.Argv...)
	}
	if service.Health != nil {
		if service.Health.Interval.Duration == 0 {
			service.Health.Interval.Duration = 2 * time.Second
		}
		if service.Health.Timeout.Duration == 0 {
			service.Health.Timeout.Duration = 3 * time.Second
		}
		if service.Health.Retries == 0 {
			service.Health.Retries = 30
		}
	}
	applyProviderDefaults(&service.Environment)
}

func applyComposeDefaults(service *Service) {
	if service.Compose == nil {
		return
	}
	if len(service.Compose.UpArgv) == 0 {
		service.Compose.UpArgv = []string{"docker", "compose"}
	}
	if len(service.Compose.DownArgv) == 0 {
		service.Compose.DownArgv = []string{"docker", "compose"}
	}
}

func applyServiceRestartDefaults(service *Service) {
	if service.Restart.Policy == "" {
		if service.Activation == "workspace" && service.Source == "native" {
			service.Restart.Policy = "on-failure"
			service.Restart.MaxRestarts = 5
		} else {
			service.Restart.Policy = "no"
		}
	}
	if service.Restart.Backoff.Duration == 0 {
		service.Restart.Backoff.Duration = time.Second
	}
}

func applyLifecycleDefaults(commands []LifecycleCommand, timeout time.Duration) {
	for i := range commands {
		if commands[i].WorkingDirectory == "" {
			commands[i].WorkingDirectory = "."
		}
		if commands[i].Timeout.Duration == 0 {
			commands[i].Timeout.Duration = timeout
		}
		applyProviderDefaults(&commands[i].Environment)
	}
}

func applyProviderDefaults(environment *Environment) {
	for i := range environment.Providers {
		if environment.Providers[i].Timeout.Duration == 0 {
			environment.Providers[i].Timeout.Duration = 10 * time.Second
		}
	}
}
