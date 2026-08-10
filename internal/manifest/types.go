package manifest

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "rungrid/v1"
	Kind       = "Workspace"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a string")
	}
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", d.String())), nil
}

type Manifest struct {
	APIVersion   string                `yaml:"api_version" json:"api_version"`
	Kind         string                `yaml:"kind" json:"kind"`
	Project      Project               `yaml:"project" json:"project"`
	Workspace    Workspace             `yaml:"workspace,omitempty" json:"workspace"`
	Repositories map[string]Repository `yaml:"repositories,omitempty" json:"repositories,omitempty"`
	Imports      []string              `yaml:"imports,omitempty" json:"imports,omitempty"`
	Runtime      Runtime               `yaml:"runtime,omitempty" json:"runtime"`
	Terminal     Terminal              `yaml:"terminal,omitempty" json:"terminal"`
	Lifecycle    Lifecycle             `yaml:"lifecycle,omitempty" json:"lifecycle"`
	Services     []Service             `yaml:"services" json:"services"`
}

type Project struct {
	Name string `yaml:"name" json:"name"`
	Slug string `yaml:"slug,omitempty" json:"slug"`
	ID   string `yaml:"id,omitempty" json:"id"`
}

type Workspace struct {
	Root string `yaml:"root,omitempty" json:"root"`
}

type Repository struct {
	Path          string `yaml:"path" json:"path"`
	Remote        string `yaml:"remote,omitempty" json:"remote"`
	DefaultBranch string `yaml:"default_branch,omitempty" json:"default_branch,omitempty"`
}

type Lifecycle struct {
	BeforeUp  []LifecycleCommand `yaml:"before_up,omitempty" json:"before_up,omitempty"`
	AfterDown []LifecycleCommand `yaml:"after_down,omitempty" json:"after_down,omitempty"`
}

type LifecycleCommand struct {
	Name             string      `yaml:"name" json:"name"`
	WorkingDirectory string      `yaml:"working_directory,omitempty" json:"working_directory"`
	Timeout          Duration    `yaml:"timeout,omitempty" json:"timeout"`
	Run              Command     `yaml:"run" json:"run"`
	Environment      Environment `yaml:"environment,omitempty" json:"environment,omitempty"`
}

type Runtime struct {
	StartupTimeout  Duration              `yaml:"startup_timeout,omitempty" json:"startup_timeout"`
	ShutdownTimeout Duration              `yaml:"shutdown_timeout,omitempty" json:"shutdown_timeout"`
	LogRetention    Duration              `yaml:"log_retention,omitempty" json:"log_retention"`
	ProcessCompose  ProcessComposeRuntime `yaml:"process_compose,omitempty" json:"process_compose"`
}

type ProcessComposeRuntime struct {
	Executable  string      `yaml:"executable,omitempty" json:"executable"`
	LogLevel    string      `yaml:"log_level,omitempty" json:"log_level"`
	LogRotation LogRotation `yaml:"log_rotation,omitempty" json:"log_rotation"`
}

type LogRotation struct {
	MaxSizeMB  int `yaml:"max_size_mb,omitempty" json:"max_size_mb"`
	MaxBackups int `yaml:"max_backups,omitempty" json:"max_backups"`
}

type Terminal struct {
	Mode  string `yaml:"mode,omitempty" json:"mode"`
	Open  *bool  `yaml:"open,omitempty" json:"open"`
	Theme string `yaml:"theme,omitempty" json:"theme,omitempty"`
}

type Service struct {
	Name             string            `yaml:"name" json:"name"`
	Repository       string            `yaml:"repository,omitempty" json:"repository"`
	Source           string            `yaml:"source" json:"source"`
	Activation       string            `yaml:"activation,omitempty" json:"activation"`
	WorkingDirectory string            `yaml:"working_directory,omitempty" json:"working_directory"`
	Run              *Run              `yaml:"run,omitempty" json:"run,omitempty"`
	Compose          *Compose          `yaml:"compose,omitempty" json:"compose,omitempty"`
	External         *External         `yaml:"external,omitempty" json:"external,omitempty"`
	Environment      Environment       `yaml:"environment,omitempty" json:"environment,omitempty"`
	DependsOn        map[string]string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Health           *Health           `yaml:"health,omitempty" json:"health,omitempty"`
	Restart          Restart           `yaml:"restart,omitempty" json:"restart"`
	Namespace        string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Terminal         ServiceTerminal   `yaml:"terminal,omitempty" json:"terminal"`
	Ports            []int             `yaml:"ports,omitempty" json:"ports,omitempty"`
}

type Command struct {
	Argv []string `yaml:"argv" json:"argv"`
}

type Run struct {
	Argv  []string `yaml:"argv" json:"argv"`
	Stdin bool     `yaml:"stdin,omitempty" json:"stdin"`
}

type Compose struct {
	File        string   `yaml:"file" json:"file"`
	ProjectName string   `yaml:"project_name,omitempty" json:"project_name,omitempty"`
	Service     string   `yaml:"service" json:"service"`
	Profiles    []string `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	UpArgv      []string `yaml:"up_argv,omitempty" json:"up_argv,omitempty"`
	DownArgv    []string `yaml:"down_argv,omitempty" json:"down_argv,omitempty"`
}

type External struct {
	URL     string   `yaml:"url,omitempty" json:"url,omitempty"`
	Command *Command `yaml:"command,omitempty" json:"command,omitempty"`
}

type Environment struct {
	Values    map[string]string     `yaml:"values,omitempty" json:"values,omitempty"`
	Providers []EnvironmentProvider `yaml:"providers,omitempty" json:"providers,omitempty"`
}

type EnvironmentProvider struct {
	Type      string   `yaml:"type" json:"type"`
	Path      string   `yaml:"path,omitempty" json:"path,omitempty"`
	Optional  bool     `yaml:"optional,omitempty" json:"optional,omitempty"`
	Argv      []string `yaml:"argv,omitempty" json:"argv,omitempty"`
	Timeout   Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Directory string   `yaml:"directory,omitempty" json:"directory,omitempty"`
}

type Health struct {
	Command     *Command `yaml:"command,omitempty" json:"command,omitempty"`
	URL         string   `yaml:"url,omitempty" json:"url,omitempty"`
	Interval    Duration `yaml:"interval,omitempty" json:"interval"`
	Timeout     Duration `yaml:"timeout,omitempty" json:"timeout"`
	Retries     int      `yaml:"retries,omitempty" json:"retries"`
	StartPeriod Duration `yaml:"start_period,omitempty" json:"start_period"`
}

type Restart struct {
	Policy      string   `yaml:"policy,omitempty" json:"policy"`
	MaxRestarts int      `yaml:"max_restarts,omitempty" json:"max_restarts"`
	Backoff     Duration `yaml:"backoff,omitempty" json:"backoff"`
}

type ServiceTerminal struct {
	Title             string   `yaml:"title,omitempty" json:"title,omitempty"`
	TriggerArgv       []string `yaml:"trigger_argv,omitempty" json:"trigger_argv,omitempty"`
	IncludeInVersions *bool    `yaml:"include_in_versions,omitempty" json:"include_in_versions"`
}

func Bool(value bool) *bool { return &value }

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
		s := &m.Services[i]
		if s.Repository == "" {
			s.Repository = WorkspaceRepository
		}
		if s.Activation == "" {
			if s.Source == "external" {
				s.Activation = "workspace"
			} else {
				s.Activation = "tab"
			}
		}
		if s.WorkingDirectory == "" {
			s.WorkingDirectory = "."
		}
		if s.Compose != nil {
			if len(s.Compose.UpArgv) == 0 {
				s.Compose.UpArgv = []string{"docker", "compose"}
			}
			if len(s.Compose.DownArgv) == 0 {
				s.Compose.DownArgv = []string{"docker", "compose"}
			}
		}
		if s.Restart.Policy == "" {
			if s.Activation == "workspace" && s.Source == "native" {
				s.Restart.Policy = "on-failure"
				s.Restart.MaxRestarts = 5
			} else {
				s.Restart.Policy = "no"
			}
		}
		if s.Restart.Backoff.Duration == 0 {
			s.Restart.Backoff.Duration = time.Second
		}
		if s.Terminal.Title == "" {
			s.Terminal.Title = s.Name
		}
		if s.Terminal.IncludeInVersions == nil {
			s.Terminal.IncludeInVersions = Bool(true)
		}
		if s.Activation == "tab" && len(s.Terminal.TriggerArgv) == 0 && s.Run != nil {
			s.Terminal.TriggerArgv = append([]string(nil), s.Run.Argv...)
		}
		if s.Health != nil {
			if s.Health.Interval.Duration == 0 {
				s.Health.Interval.Duration = 2 * time.Second
			}
			if s.Health.Timeout.Duration == 0 {
				s.Health.Timeout.Duration = 3 * time.Second
			}
			if s.Health.Retries == 0 {
				s.Health.Retries = 30
			}
		}
		for j := range s.Environment.Providers {
			if s.Environment.Providers[j].Timeout.Duration == 0 {
				s.Environment.Providers[j].Timeout.Duration = 10 * time.Second
			}
		}
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
