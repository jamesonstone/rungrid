package planner

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

type Plan struct {
	APIVersion        string           `json:"api_version"`
	ProjectID         string           `json:"project_id"`
	GenerationID      string           `json:"generation_id"`
	ManifestSHA256    string           `json:"manifest_sha256"`
	LifecycleSHA256   string           `json:"lifecycle_sha256"`
	ManifestDirectory string           `json:"manifest_directory"`
	WorkspaceRoot     string           `json:"workspace_root"`
	Repositories      []RepositoryPlan `json:"repositories"`
	Lifecycle         LifecyclePlan    `json:"lifecycle"`
	Services          []ServicePlan    `json:"services"`
	Artifacts         []string         `json:"artifacts"`
	Executables       []string         `json:"executables"`
	TerminalMode      string           `json:"terminal_mode"`
	OpenTerminal      bool             `json:"open_terminal"`
	Recovery          *RecoveryPlan    `json:"recovery,omitempty"`
}

type ServicePlan struct {
	Name         string            `json:"name"`
	Repository   string            `json:"repository"`
	Source       string            `json:"source"`
	Activation   string            `json:"activation"`
	Process      bool              `json:"process_compose_process"`
	Disabled     bool              `json:"disabled"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Actions      []string          `json:"actions"`
}

type RepositoryPlan struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Remote        string `json:"remote"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

func Build(loaded *manifest.Loaded, generatorVersion string) Plan {
	manifestHash := state.Hash(loaded.MergedYAML)
	generationHash := state.Hash(loaded.MergedYAML, []byte(generatorVersion))
	lifecycle, lifecycleHash := buildLifecyclePlan(&loaded.Manifest)
	plan := Plan{
		APIVersion:        "rungrid/output/v1",
		ProjectID:         loaded.Manifest.Project.ID,
		GenerationID:      generationHash[:20],
		ManifestSHA256:    manifestHash,
		LifecycleSHA256:   lifecycleHash,
		ManifestDirectory: ".",
		WorkspaceRoot:     loaded.Manifest.Workspace.Root,
		Repositories:      repositoryPlans(&loaded.Manifest),
		Lifecycle:         lifecycle,
		Artifacts: []string{
			"manifest.yaml",
			"plan.json",
			"process-compose.yaml",
			"wrappers/rungrid-resource-guard",
			"wrappers/rungrid-maintenance-sync",
			"wrappers/rungrid-maintenance-worktrees-prune",
		},
		TerminalMode: loaded.Manifest.Terminal.Mode,
		OpenTerminal: loaded.Manifest.Terminal.Open != nil && *loaded.Manifest.Terminal.Open,
	}
	if loaded.Manifest.Terminal.Mode == "warp" {
		plan.Artifacts = append(plan.Artifacts,
			"terminal/warp/00_overview.toml.tmpl",
			"terminal/warp/01_versions.toml.tmpl",
		)
	}
	executables := map[string]bool{loaded.Manifest.Runtime.ProcessCompose.Executable: true}
	addLifecycleExecutables(executables, &loaded.Manifest)
	if loaded.Manifest.Terminal.Mode == "warp" {
		executables["zsh"] = true
	}
	tabIndex := 2
	for _, service := range loaded.Manifest.Services {
		item := ServicePlan{
			Name:         service.Name,
			Repository:   service.Repository,
			Source:       service.Source,
			Activation:   service.Activation,
			Process:      service.Source != "external",
			Disabled:     service.Activation == "tab",
			Dependencies: service.DependsOn,
		}
		switch service.Source {
		case "native":
			item.Actions = []string{"resolve environment", "start supervised native process"}
			if service.Run != nil {
				for _, executable := range manifest.CommandExecutables(service.Run.Argv) {
					executables[executable] = true
				}
			}
		case "compose":
			item.Actions = []string{"resolve environment", "start exact Compose service", "record exact Compose shutdown"}
			if service.Compose != nil {
				for _, executable := range manifest.CommandExecutables(service.Compose.UpArgv) {
					executables[executable] = true
				}
			}
		case "external":
			item.Actions = []string{"observe external readiness"}
			if service.External != nil && service.External.Command != nil {
				for _, executable := range manifest.CommandExecutables(service.External.Command.Argv) {
					executables[executable] = true
				}
			}
		}
		if service.Activation == "tab" {
			item.Actions = append(item.Actions, "wait for exclusive service session")
			if len(service.Terminal.TriggerArgv) > 0 {
				executables[service.Terminal.TriggerArgv[0]] = true
			}
			if loaded.Manifest.Terminal.Mode == "warp" {
				plan.Artifacts = append(plan.Artifacts, fmt.Sprintf("terminal/warp/%02d_%s.toml.tmpl", tabIndex, service.Name))
				tabIndex++
			}
			plan.Artifacts = append(plan.Artifacts, "wrappers/"+service.Name)
		} else if service.Source != "external" {
			plan.Artifacts = append(plan.Artifacts, "wrappers/"+service.Name)
		}
		for _, provider := range service.Environment.Providers {
			switch provider.Type {
			case "command":
				for _, executable := range manifest.CommandExecutables(provider.Argv) {
					executables[executable] = true
				}
			case "direnv":
				executables["direnv"] = true
			}
		}
		if service.Health != nil && service.Health.Command != nil {
			for _, executable := range manifest.CommandExecutables(service.Health.Command.Argv) {
				executables[executable] = true
			}
		}
		plan.Services = append(plan.Services, item)
	}
	for executable := range executables {
		plan.Executables = append(plan.Executables, executable)
	}
	sort.Strings(plan.Executables)
	return plan
}

func (p Plan) JSON() ([]byte, error) {
	content, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func (p Plan) WriteHuman(w io.Writer) {
	_, _ = fmt.Fprintf(
		w,
		"Project: %s\nGeneration: %s\nManifest directory: %s\nWorkspace root: %s\nTerminal: %s\n\n",
		p.ProjectID,
		p.GenerationID,
		p.ManifestDirectory,
		p.WorkspaceRoot,
		p.TerminalMode,
	)
	p.Lifecycle.writeHuman(w)
	_, _ = fmt.Fprintln(w, "Repositories:")
	for _, repository := range p.Repositories {
		defaultBranch := repository.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = "<remote HEAD>"
		}
		_, _ = fmt.Fprintf(w, "  %-20s %-20s remote=%s default=%s\n", repository.Name, repository.Path, repository.Remote, defaultBranch)
	}
	_, _ = fmt.Fprintln(w)
	if p.Recovery != nil {
		if p.Recovery.Generation == "" {
			_, _ = fmt.Fprintln(w, "Recovery: start; no recorded lifecycle generation")
			_, _ = fmt.Fprintln(w)
		} else {
			_, _ = fmt.Fprintf(
				w,
				"Recovery: %s; recorded generation %s is %s (teardown-required=%t)\n\n",
				p.Recovery.Action,
				p.Recovery.Generation,
				p.Recovery.State,
				p.Recovery.TeardownRequired,
			)
		}
	}
	_, _ = fmt.Fprintln(w, "Services:")
	for _, service := range p.Services {
		stateText := "enabled"
		if service.Disabled {
			stateText = "disabled until session ownership"
		} else if !service.Process {
			stateText = "observed only"
		}
		_, _ = fmt.Fprintf(w, "  %-20s %-12s %-9s %-9s %s\n", service.Name, service.Repository, service.Source, service.Activation, stateText)
	}
	_, _ = fmt.Fprintln(w, "\nArtifacts:")
	for _, artifact := range p.Artifacts {
		_, _ = fmt.Fprintf(w, "  %s\n", artifact)
	}
	_, _ = fmt.Fprintf(w, "\nRequired executables: %s\n", strings.Join(p.Executables, ", "))
}

func repositoryPlans(m *manifest.Manifest) []RepositoryPlan {
	result := []RepositoryPlan{{Name: manifest.WorkspaceRepository, Path: ".", Remote: "origin"}}
	for _, name := range manifest.DeclaredRepositoryNames(m) {
		repository := m.Repositories[name]
		result = append(result, RepositoryPlan{Name: name, Path: repository.Path, Remote: repository.Remote, DefaultBranch: repository.DefaultBranch})
	}
	return result
}
