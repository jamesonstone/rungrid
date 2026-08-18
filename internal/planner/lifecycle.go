package planner

import (
	"strings"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

type RecoveryPlan struct {
	Generation          string `json:"generation"`
	State               string `json:"state"`
	TeardownRequired    bool   `json:"teardown_required"`
	Action              string `json:"action"`
	ManifestCompatible  bool   `json:"manifest_compatible"`
	LifecycleCompatible bool   `json:"lifecycle_compatible"`
}

type LifecyclePlan struct {
	BeforeUp          []LifecycleCommandPlan `json:"before_up"`
	AfterDown         []LifecycleCommandPlan `json:"after_down"`
	RollbackOnFailure bool                   `json:"rollback_on_failure"`
}

type LifecycleCommandPlan struct {
	Name             string   `json:"name"`
	WorkingDirectory string   `json:"working_directory"`
	Argv             []string `json:"argv"`
	Timeout          string   `json:"timeout"`
	Providers        []string `json:"providers,omitempty"`
}

func buildLifecyclePlan(configuration *manifest.Manifest) (LifecyclePlan, string) {
	plan := LifecyclePlan{
		BeforeUp:          commandPlans(configuration.Lifecycle.BeforeUp),
		AfterDown:         commandPlans(configuration.Lifecycle.AfterDown),
		RollbackOnFailure: len(configuration.Lifecycle.AfterDown) > 0,
	}
	return plan, workspace.LifecycleDigest(configuration.Lifecycle)
}

func commandPlans(commands []manifest.LifecycleCommand) []LifecycleCommandPlan {
	result := make([]LifecycleCommandPlan, 0, len(commands))
	for _, command := range commands {
		providers := make([]string, 0, len(command.Environment.Providers))
		for _, provider := range command.Environment.Providers {
			providers = append(providers, provider.Type)
		}
		result = append(result, LifecycleCommandPlan{
			Name:             command.Name,
			WorkingDirectory: command.WorkingDirectory,
			Argv:             redactArgv(command.Run.Argv),
			Timeout:          command.Timeout.String(),
			Providers:        providers,
		})
	}
	return result
}

func redactArgv(argv []string) []string {
	result := append([]string(nil), argv...)
	redactNext := false
	for index, value := range result {
		if redactNext {
			result[index] = "<redacted>"
			redactNext = false
			continue
		}
		if key, _, found := strings.Cut(value, "="); found && manifest.SecretLikeKey(strings.TrimLeft(key, "-")) {
			result[index] = key + "=<redacted>"
			continue
		}
		redactNext = manifest.SecretLikeKey(strings.TrimLeft(value, "-"))
	}
	return result
}

func addLifecycleExecutables(executables map[string]bool, configuration *manifest.Manifest) {
	commands := append(
		append([]manifest.LifecycleCommand(nil), configuration.Lifecycle.BeforeUp...),
		configuration.Lifecycle.AfterDown...,
	)
	for _, command := range commands {
		for _, executable := range manifest.CommandExecutables(command.Run.Argv) {
			executables[executable] = true
		}
		for _, provider := range command.Environment.Providers {
			if provider.Type == "command" {
				for _, executable := range manifest.CommandExecutables(provider.Argv) {
					executables[executable] = true
				}
			}
			if provider.Type == "direnv" {
				executables["direnv"] = true
			}
		}
	}
}

func InspectRecovery(layout state.Layout, requested Plan) (*RecoveryPlan, error) {
	journal, exists, err := workspace.ReadJournalIfPresent(layout)
	if err != nil {
		return nil, err
	}
	if !exists {
		return &RecoveryPlan{Action: "start", ManifestCompatible: true, LifecycleCompatible: true}, nil
	}
	result := &RecoveryPlan{
		Generation: journal.GenerationID, State: journal.State,
		TeardownRequired:    journal.TeardownRequired,
		ManifestCompatible:  journal.ManifestSHA256 == requested.ManifestSHA256,
		LifecycleCompatible: journal.LifecycleSHA256 == requested.LifecycleSHA256,
	}
	switch {
	case journal.TeardownRequired || journal.State == workspace.StateCleanup || journal.State == workspace.StateStarting || journal.State == workspace.StateStopping:
		result.Action = "recover"
		if journal.GenerationID != requested.GenerationID {
			result.Action = "recover-and-replace"
		}
	case journal.State == workspace.StateActive && journal.GenerationID == requested.GenerationID:
		result.Action = "reuse"
	case journal.GenerationID != requested.GenerationID:
		result.Action = "replace"
	default:
		result.Action = "start"
	}
	return result, nil
}
