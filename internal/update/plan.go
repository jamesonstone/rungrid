package update

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/manifest"
)

const defaultCommandTimeout = 10 * time.Minute

// unit is one dependency install or refresh step with its input fingerprint.
type unit struct {
	result      StepResult
	key         string
	fingerprint string
	run         bool
	command     Command
}

type servicePlan struct {
	name         string
	policy       string
	running      bool
	watchKey     string
	watchPrint   string
	watchChanged bool
	restart      bool
	result       ServiceResult
}

type repositoryPlan struct {
	units    []*unit
	services []*servicePlan
}

// buildPlan compares the target commit's inputs with the recorded baseline.
// previous is the pre-fast-forward tree, used for restart detection when a
// service has no recorded baseline yet; it is nil when nothing advances.
func buildPlan(root string, services []Service, target, previous tree, baseline map[string]string) repositoryPlan {
	var plan repositoryPlan
	dependencies := map[string]*unit{}
	var steps []*unit
	for _, service := range services {
		base := serviceBase(root, service.WorkingDirectory)
		if dependencyMode(service.Config) == "auto" {
			for _, detected := range detectDependencies(target, base) {
				if existing, ok := dependencies[detected.ID]; ok {
					existing.result.Services = append(existing.result.Services, service.Config.Name)
					continue
				}
				dependencies[detected.ID] = dependencyUnit(root, detected, service.Config.Name, target, baseline)
			}
		}
		if service.Config.Refresh != nil {
			for _, step := range service.Config.Refresh.Steps {
				steps = append(steps, stepUnit(root, base, service.Config, step, target, baseline))
			}
		}
	}
	ids := make([]string, 0, len(dependencies))
	for id := range dependencies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		plan.units = append(plan.units, dependencies[id])
	}
	plan.units = append(plan.units, steps...)
	for _, service := range services {
		plan.services = append(plan.services, planService(root, service, plan.units, target, previous, baseline))
	}
	return plan
}

func dependencyUnit(root string, detected dependency, serviceName string, target tree, baseline map[string]string) *unit {
	item := &unit{
		key:         detected.ID,
		fingerprint: target.fingerprint(detected.Directory, detected.Inputs),
		result: StepResult{
			ID: detected.ID, Kind: KindDependencies, Name: detected.Name, Services: []string{serviceName},
			Directory: displayDirectory(detected.Directory), Argv: detected.Argv,
		},
		command: Command{
			Name: detected.Name, Root: root, WorkingDirectory: detected.Directory,
			Argv: detected.Argv, Timeout: defaultCommandTimeout,
		},
	}
	item.run, item.result.Reason = changed(baseline, item.key, item.fingerprint, strings.Join(detected.Inputs, " or ")+" changed")
	if !item.run && detected.NodeModules && nodeModulesMissing(filepath.Join(root, filepath.FromSlash(detected.Directory))) {
		item.run, item.result.Reason = true, "node_modules is missing"
	}
	return item
}

func stepUnit(root, base string, service *manifest.Service, step manifest.RefreshStep, target tree, baseline map[string]string) *unit {
	timeout := step.Timeout.Duration
	if timeout == 0 {
		timeout = defaultCommandTimeout
	}
	item := &unit{
		key:         "step/" + service.Name + "/" + step.Name,
		fingerprint: target.fingerprint(base, step.Watch),
		result: StepResult{
			ID: "step/" + service.Name + "/" + step.Name, Kind: KindStep, Name: step.Name, Services: []string{service.Name},
			Directory: displayDirectory(base), Argv: step.Run.Argv,
		},
		command: Command{
			Name: service.Name + "-" + step.Name, Root: root, WorkingDirectory: base,
			Argv: step.Run.Argv, Timeout: timeout, Environment: service.Environment,
		},
	}
	item.run, item.result.Reason = changed(baseline, item.key, item.fingerprint, "watched paths changed")
	return item
}

func changed(baseline map[string]string, key, fingerprint, reason string) (bool, string) {
	previous, ok := baseline[key]
	if !ok {
		return true, "no recorded baseline"
	}
	if previous != fingerprint {
		return true, reason
	}
	return false, "inputs unchanged"
}

func planService(root string, service Service, units []*unit, target, previous tree, baseline map[string]string) *servicePlan {
	policy, _ := restartPolicy(service)
	base := serviceBase(root, service.WorkingDirectory)
	patterns := watchPatterns(service.Config)
	item := &servicePlan{
		name: service.Config.Name, policy: policy, running: service.Running,
		watchKey: "watch/" + service.Config.Name, watchPrint: target.fingerprint(base, patterns),
	}
	if recorded, ok := baseline[item.watchKey]; ok {
		item.watchChanged = recorded != item.watchPrint
	} else if previous != nil {
		item.watchChanged = previous.fingerprint(base, patterns) != item.watchPrint
	}
	dependenciesRan, stepRan := false, false
	for _, candidate := range units {
		if !candidate.run || !contains(candidate.result.Services, item.name) {
			continue
		}
		if candidate.result.Kind == KindDependencies {
			dependenciesRan = true
		} else {
			stepRan = true
		}
	}
	item.result = ServiceResult{Name: item.name, Restart: policy, Status: StatusCurrent, Reason: "inputs unchanged"}
	reason := ""
	switch {
	case dependenciesRan:
		reason = "dependency install"
	case stepRan:
		reason = "refresh step"
	case item.watchChanged:
		reason = "watched paths changed"
	}
	switch {
	case reason == "":
	case policy == manifest.RefreshRestartNever:
		item.result.Status, item.result.Reason = StatusSkipped, reason+"; restart policy is never"
	case policy == manifest.RefreshRestartDependencies && !dependenciesRan:
		item.result.Status, item.result.Reason = StatusSkipped, reason+"; hot reload applies source changes"
	case !item.running:
		item.result.Status, item.result.Reason = StatusSkipped, reason+"; service is not running"
	default:
		item.restart = true
		item.result.RestartPath = service.RestartPath
		item.result.Status, item.result.Reason = StatusUpdated, reason+"; "+restartDescription(service)
	}
	return item
}

func restartDescription(service Service) string {
	if service.RestartPath == RestartSession {
		return "restart through its tab session"
	}
	if service.Config.Activation == "tab" {
		return "restart through Process Compose (no live tab session)"
	}
	return "restart through Process Compose"
}

func serviceBase(root, workingDirectory string) string {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if resolved, err := filepath.EvalSymlinks(workingDirectory); err == nil {
		workingDirectory = resolved
	}
	relative, err := filepath.Rel(root, workingDirectory)
	if err != nil || relative == "." {
		return ""
	}
	return filepath.ToSlash(relative)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (plan repositoryPlan) restartNames() []string {
	var names []string
	for _, service := range plan.services {
		if service.restart {
			names = append(names, service.name)
		}
	}
	return names
}
