package manifest

import (
	"fmt"
	"path"
	"strings"
)

// Refresh declares how `rungrid update` keeps one service current after its
// checked-out default branch advances. Every field is optional; the update
// engine supplies detected defaults.
type Refresh struct {
	// Restart is changed, dependencies, or never. Empty selects changed, or
	// dependencies for a detected hot-reloading development server.
	Restart string `yaml:"restart,omitempty" json:"restart,omitempty"`
	// Watch lists service-relative paths or globs whose change restarts the
	// service under the changed policy. Empty watches the working directory,
	// or only the compose file for a compose service.
	Watch []string `yaml:"watch,omitempty" json:"watch,omitempty"`
	// Dependencies is auto (detect go, npm, pnpm, or yarn installs) or none.
	Dependencies string        `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Steps        []RefreshStep `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// RefreshStep is one post-update command, such as a migration or a generated
// document, that runs only when one of its watched paths changes.
type RefreshStep struct {
	Name    string   `yaml:"name" json:"name"`
	Run     Command  `yaml:"run" json:"run"`
	Watch   []string `yaml:"watch" json:"watch"`
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

const (
	RefreshRestartChanged      = "changed"
	RefreshRestartDependencies = "dependencies"
	RefreshRestartNever        = "never"
)

func validateRefresh(refresh *Refresh, prefix string, add func(string, string)) {
	switch refresh.Restart {
	case "", RefreshRestartChanged, RefreshRestartDependencies, RefreshRestartNever:
	default:
		add(prefix+".restart", "must be changed, dependencies, or never")
	}
	switch refresh.Dependencies {
	case "", "auto", "none":
	default:
		add(prefix+".dependencies", "must be auto or none")
	}
	validateWatch(refresh.Watch, prefix+".watch", add)
	names := map[string]bool{}
	for index, step := range refresh.Steps {
		stepPrefix := fmt.Sprintf("%s.steps[%d]", prefix, index)
		if !serviceNamePattern.MatchString(step.Name) {
			add(stepPrefix+".name", "must match [a-z][a-z0-9-]*")
		} else if names[step.Name] {
			add(stepPrefix+".name", "must be unique within the service")
		}
		names[step.Name] = true
		validateArgv(step.Run.Argv, stepPrefix+".run.argv", add)
		if len(step.Watch) == 0 {
			add(stepPrefix+".watch", "must list at least one path")
		}
		validateWatch(step.Watch, stepPrefix+".watch", add)
		if step.Timeout.Duration < 0 {
			add(stepPrefix+".timeout", "must not be negative")
		}
	}
}

func validateWatch(patterns []string, field string, add func(string, string)) {
	for index, pattern := range patterns {
		entry := fmt.Sprintf("%s[%d]", field, index)
		if strings.TrimSpace(pattern) == "" || strings.ContainsRune(pattern, '\x00') {
			add(entry, "must be a non-empty path")
			continue
		}
		if strings.HasPrefix(pattern, "/") || strings.Contains(pattern, `\`) {
			add(entry, "must be a service-relative slash path")
			continue
		}
		clean := path.Clean(pattern)
		if clean == ".." || strings.HasPrefix(clean, "../") {
			add(entry, "must remain within the service working directory")
			continue
		}
		if _, err := path.Match(strings.ReplaceAll(clean, "**", "*"), ""); err != nil {
			add(entry, "is not a valid glob")
		}
	}
}
