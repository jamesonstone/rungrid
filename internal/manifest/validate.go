package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
)

var (
	serviceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	slugPattern        = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	projectIDPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*-[a-z2-7]{6}$`)
	triggerNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	secretKeyPattern   = regexp.MustCompile(`(?i)(secret|token|password|passwd|private[_-]?key|api[_-]?key|credential)`)
)

func SecretLikeKey(key string) bool { return secretKeyPattern.MatchString(key) }

func Validate(m *Manifest, root string) error {
	var problems []string
	add := func(path, message string) { problems = append(problems, path+": "+message) }

	if m.APIVersion != APIVersion {
		add("api_version", fmt.Sprintf("must be %q", APIVersion))
	}
	if m.Kind != Kind {
		add("kind", fmt.Sprintf("must be %q", Kind))
	}
	if strings.TrimSpace(m.Project.Name) == "" {
		add("project.name", "is required")
	}
	if !slugPattern.MatchString(m.Project.Slug) {
		add("project.slug", "must contain lowercase letters, digits, and single hyphens")
	}
	if m.Project.ID == "" {
		add("project.id", "is required for stable project-scoped state; run rungrid init")
	} else if !projectIDPattern.MatchString(m.Project.ID) || !strings.HasPrefix(m.Project.ID, m.Project.Slug+"-") {
		add("project.id", "must be the project slug plus a six-character lowercase base32 suffix")
	}
	if filepath.IsAbs(m.Workspace.Root) {
		add("workspace.root", "must be relative to the manifest directory")
	}
	if m.Runtime.StartupTimeout.Duration <= 0 {
		add("runtime.startup_timeout", "must be positive")
	}
	if m.Runtime.ShutdownTimeout.Duration <= 0 {
		add("runtime.shutdown_timeout", "must be positive")
	}
	if m.Runtime.LogRetention.Duration <= 0 {
		add("runtime.log_retention", "must be positive")
	}
	if strings.TrimSpace(m.Runtime.ProcessCompose.Executable) == "" {
		add("runtime.process_compose.executable", "is required")
	}
	if !map[string]bool{
		"trace": true, "debug": true, "info": true, "warn": true,
		"error": true, "fatal": true, "panic": true, "disabled": true,
	}[m.Runtime.ProcessCompose.LogLevel] {
		add("runtime.process_compose.log_level", "must be trace, debug, info, warn, error, fatal, panic, or disabled")
	}
	if m.Runtime.ProcessCompose.LogRotation.MaxSizeMB <= 0 {
		add("runtime.process_compose.log_rotation.max_size_mb", "must be positive")
	}
	if m.Runtime.ProcessCompose.LogRotation.MaxBackups <= 0 {
		add("runtime.process_compose.log_rotation.max_backups", "must be positive")
	}
	validateResourceGuard(m.Runtime.ResourceGuard, "runtime.resource_guard", add)
	if m.Terminal.Mode != "warp" && m.Terminal.Mode != "headless" {
		add("terminal.mode", "must be warp or headless")
	}
	validateLifecycle(root, m.Lifecycle, add)
	repositoryRoots := validateRepositories(root, m.Repositories, add)

	names := make(map[string]int, len(m.Services))
	for i := range m.Services {
		service := &m.Services[i]
		prefix := fmt.Sprintf("services[%d]", i)
		if !serviceNamePattern.MatchString(service.Name) {
			add(prefix+".name", "must match [a-z][a-z0-9-]*")
		}
		if strings.HasPrefix(service.Name, "rungrid-maintenance-") {
			add(prefix+".name", "uses a reserved internal process prefix")
		}
		if previous, exists := names[service.Name]; exists {
			add(prefix+".name", fmt.Sprintf("duplicates services[%d]", previous))
		} else {
			names[service.Name] = i
		}
		if service.Source != "native" && service.Source != "compose" && service.Source != "external" {
			add(prefix+".source", "must be native, compose, or external")
		}
		if service.Activation != "workspace" && service.Activation != "tab" {
			add(prefix+".activation", "must be workspace or tab")
		}
		if service.Source == "external" && service.Activation != "workspace" {
			add(prefix+".activation", "external services must use workspace activation")
		}
		repositoryRoot, repositoryOK := repositoryRoots[service.Repository]
		if !serviceNamePattern.MatchString(service.Repository) {
			add(prefix+".repository", "must match [a-z][a-z0-9-]*")
			repositoryOK = false
		} else if _, declared := m.Repositories[service.Repository]; service.Repository != WorkspaceRepository && !declared {
			add(prefix+".repository", "references an unknown repository")
			repositoryOK = false
		}
		if repositoryOK {
			validateWorkingDirectory(repositoryRoot, service.WorkingDirectory, prefix+".working_directory", "repository", add)
		}
		blocks := 0
		if service.Run != nil {
			blocks++
		}
		if service.Compose != nil {
			blocks++
		}
		if service.External != nil {
			blocks++
		}
		if blocks != 1 {
			add(prefix, "must define exactly one of run, compose, or external")
		}
		switch service.Source {
		case "native":
			if service.Run == nil {
				add(prefix+".run", "is required for a native service")
			} else {
				validateArgv(service.Run.Argv, prefix+".run.argv", add)
			}
		case "compose":
			if service.Compose == nil {
				add(prefix+".compose", "is required for a compose service")
			} else if repositoryOK {
				validateCompose(repositoryRoot, service, prefix, add)
			}
		case "external":
			if service.External == nil {
				add(prefix+".external", "is required for an external service")
			} else {
				validateExternal(service.External, prefix+".external", add)
			}
		}
		if service.Activation == "tab" {
			validateArgv(service.Terminal.TriggerArgv, prefix+".terminal.trigger_argv", add)
			if len(service.Terminal.TriggerArgv) > 0 && !triggerNamePattern.MatchString(service.Terminal.TriggerArgv[0]) {
				add(prefix+".terminal.trigger_argv[0]", "must be a simple executable name that can be wrapped by zsh")
			}
		}
		if repositoryOK {
			validateEnvironment(repositoryRoot, service.Environment, service.WorkingDirectory, prefix+".environment", "repository", add)
		}
		validateHealth(service.Health, prefix+".health", add)
		if service.Restart.Policy != "no" && service.Restart.Policy != "always" && service.Restart.Policy != "on-failure" {
			add(prefix+".restart.policy", "must be no, always, or on-failure")
		}
		if service.Restart.MaxRestarts < 0 {
			add(prefix+".restart.max_restarts", "must not be negative")
		}
		if service.Restart.Backoff.Duration < 0 {
			add(prefix+".restart.backoff", "must not be negative")
		}
		if service.ResourceGuard != nil {
			if service.ResourceGuard.SampleInterval.Duration != 0 {
				add(prefix+".resource_guard.sample_interval", "is a workspace-wide runtime setting and may not be overridden per service")
			}
			validateResourceGuard(EffectiveResourceGuard(m.Runtime.ResourceGuard, *service.ResourceGuard), prefix+".resource_guard", add)
		}
		for portIndex, port := range service.Ports {
			if port < 1 || port > 65535 {
				add(fmt.Sprintf("%s.ports[%d]", prefix, portIndex), "must be between 1 and 65535")
			}
		}
	}

	allowedConditions := map[string]bool{"running": true, "healthy": true, "completed_successfully": true}
	for i, service := range m.Services {
		for dependency, condition := range service.DependsOn {
			path := fmt.Sprintf("services[%d].depends_on.%s", i, dependency)
			if _, exists := names[dependency]; !exists {
				add(path, "references an unknown service")
			}
			if !allowedConditions[condition] {
				add(path, "must be running, healthy, or completed_successfully")
			}
			if dependency == service.Name {
				add(path, "may not depend on itself")
			}
		}
	}
	if cycle := dependencyCycle(m.Services); len(cycle) > 0 {
		add("services", "dependency cycle: "+strings.Join(cycle, " -> "))
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errs.New(errs.ExitUsage, "RG120", "invalid manifest:\n  - "+strings.Join(problems, "\n  - "))
	}
	return nil
}
