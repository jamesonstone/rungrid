package processcompose

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"gopkg.in/yaml.v3"
)

type Artifacts struct {
	Configuration []byte
	Wrappers      map[string][]byte
}

func Compile(m *manifest.Manifest, generationID string) (Artifacts, error) {
	root := mappingNode()
	appendScalar(root, "version", "0.5")
	appendScalar(root, "name", m.Project.ID)
	appendScalar(root, "log_level", m.Runtime.ProcessCompose.LogLevel)
	appendBool(root, "is_strict", true)
	appendBool(root, "ordered_shutdown", true)
	processes := mappingNode()
	root.Content = append(root.Content, scalarNode("processes"), processes)

	result := Artifacts{Wrappers: map[string][]byte{}}
	for _, service := range m.Services {
		if service.Source == "external" {
			continue
		}
		process := mappingNode()
		wrapperName := safeFilename(service.Name)
		appendScalar(process, "command", "./wrappers/"+wrapperName)
		appendScalar(process, "working_dir", ".")
		appendScalar(process, "log_location", "./logs/"+wrapperName+".log")
		appendLogConfiguration(process, m)
		appendBool(process, "is_dotenv_disabled", true)
		if service.Activation == "tab" {
			appendBool(process, "disabled", true)
		}
		if service.Namespace != "" {
			appendScalar(process, "namespace", service.Namespace)
		}
		if len(service.DependsOn) > 0 {
			dependencies := mappingNode()
			for _, dependency := range orderedDependencies(m, service) {
				condition := mappingNode()
				appendScalar(condition, "condition", dependencyCondition(service.DependsOn[dependency]))
				dependencies.Content = append(dependencies.Content, scalarNode(dependency), condition)
			}
			process.Content = append(process.Content, scalarNode("depends_on"), dependencies)
		}
		availability := mappingNode()
		appendScalar(availability, "restart", restartPolicy(service.Restart.Policy))
		appendInt(availability, "backoff_seconds", secondsCeil(service.Restart.Backoff.Seconds()))
		appendInt(availability, "max_restarts", service.Restart.MaxRestarts)
		process.Content = append(process.Content, scalarNode("availability"), availability)
		if service.Health != nil {
			probe := mappingNode()
			execProbe := mappingNode()
			appendScalar(execProbe, "command", "./wrappers/"+wrapperName+"-health")
			appendScalar(execProbe, "working_dir", ".")
			probe.Content = append(probe.Content, scalarNode("exec"), execProbe)
			appendInt(probe, "initial_delay_seconds", secondsCeil(service.Health.StartPeriod.Seconds()))
			appendInt(probe, "period_seconds", secondsCeil(service.Health.Interval.Seconds()))
			appendInt(probe, "timeout_seconds", secondsCeil(service.Health.Timeout.Seconds()))
			appendInt(probe, "failure_threshold", service.Health.Retries)
			process.Content = append(process.Content, scalarNode("readiness_probe"), probe)
			result.Wrappers[wrapperName+"-health"] = wrapperScript(m.Project.ID, generationID, service.Name, true)
		}
		processes.Content = append(processes.Content, scalarNode(service.Name), process)
		result.Wrappers[wrapperName] = wrapperScript(m.Project.ID, generationID, service.Name, false)
	}
	for _, operation := range []struct {
		name      string
		operation string
	}{
		{name: "rungrid-maintenance-sync", operation: "sync"},
		{name: "rungrid-maintenance-worktrees-prune", operation: "worktrees-prune"},
	} {
		process := mappingNode()
		appendScalar(process, "command", "./wrappers/"+operation.name)
		appendScalar(process, "working_dir", ".")
		appendScalar(process, "log_location", "./logs/"+operation.name+".log")
		appendLogConfiguration(process, m)
		appendBool(process, "is_dotenv_disabled", true)
		appendBool(process, "disabled", true)
		appendScalar(process, "namespace", "maintenance")
		availability := mappingNode()
		appendScalar(availability, "restart", "no")
		appendInt(availability, "backoff_seconds", 0)
		appendInt(availability, "max_restarts", 0)
		process.Content = append(process.Content, scalarNode("availability"), availability)
		processes.Content = append(processes.Content, scalarNode(operation.name), process)
		result.Wrappers[operation.name] = maintenanceWrapperScript(m.Project.ID, generationID, operation.operation)
	}

	document := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return Artifacts{}, errs.Wrap(errs.ExitFailure, "RG301", "encode Process Compose configuration", err)
	}
	if err := encoder.Close(); err != nil {
		return Artifacts{}, errs.Wrap(errs.ExitFailure, "RG302", "finish Process Compose configuration", err)
	}
	result.Configuration = buffer.Bytes()
	return result, nil
}

func maintenanceWrapperScript(projectID, generationID, operation string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\nset -eu\n: \"${RUNGRID_EXECUTABLE:=rungrid}\"\nexec \"$RUNGRID_EXECUTABLE\" internal maintenance-worker --project-id %s --generation %s --operation %s\n", projectID, generationID, operation))
}

func wrapperScript(projectID, generationID, service string, health bool) []byte {
	command := "exec-service"
	if health {
		command = "health-service"
	}
	return []byte(fmt.Sprintf("#!/bin/sh\nset -eu\n: \"${RUNGRID_EXECUTABLE:=rungrid}\"\nexec \"$RUNGRID_EXECUTABLE\" internal %s --project-id %s --generation %s --service %s\n", command, projectID, generationID, service))
}

func orderedDependencies(m *manifest.Manifest, service manifest.Service) []string {
	result := make([]string, 0, len(service.DependsOn))
	for _, candidate := range m.Services {
		if _, exists := service.DependsOn[candidate.Name]; exists && candidate.Source != "external" {
			result = append(result, candidate.Name)
		}
	}
	return result
}

func dependencyCondition(condition string) string {
	switch condition {
	case "running":
		return "process_started"
	case "healthy":
		return "process_healthy"
	case "completed_successfully":
		return "process_completed_successfully"
	default:
		return condition
	}
}

func restartPolicy(policy string) string {
	if policy == "on-failure" {
		return "on_failure"
	}
	return policy
}

func safeFilename(name string) string {
	return strings.Trim(filepath.ToSlash(name), "/")
}

func secondsCeil(value float64) int {
	if value <= 0 {
		return 0
	}
	return int(math.Ceil(value))
}

func appendLogConfiguration(process *yaml.Node, m *manifest.Manifest) {
	rotation := mappingNode()
	appendInt(rotation, "max_size_mb", m.Runtime.ProcessCompose.LogRotation.MaxSizeMB)
	appendInt(rotation, "max_age_days", secondsCeil(m.Runtime.LogRetention.Hours()/24))
	appendInt(rotation, "max_backups", m.Runtime.ProcessCompose.LogRotation.MaxBackups)
	appendBool(rotation, "compress", true)
	configuration := mappingNode()
	configuration.Content = append(configuration.Content, scalarNode("rotation"), rotation)
	process.Content = append(process.Content, scalarNode("log_configuration"), configuration)
}

func mappingNode() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }
func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
func appendScalar(mapping *yaml.Node, key, value string) {
	mapping.Content = append(mapping.Content, scalarNode(key), scalarNode(value))
}
func appendBool(mapping *yaml.Node, key string, value bool) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprintf("%t", value)}
	mapping.Content = append(mapping.Content, scalarNode(key), node)
}
func appendInt(mapping *yaml.Node, key string, value int) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", value)}
	mapping.Content = append(mapping.Content, scalarNode(key), node)
}
