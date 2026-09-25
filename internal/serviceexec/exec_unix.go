//go:build darwin || linux

package serviceexec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/environment"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

type Context struct {
	Layout        state.Layout
	GenerationID  string
	WorkspaceRoot string
	Manifest      *manifest.Manifest
}

func LoadContext(projectID, generationID, stateOverride, workspaceRoot string) (Context, error) {
	if stateOverride == "" {
		stateOverride = os.Getenv("RUNGRID_STATE_DIR")
	}
	if workspaceRoot == "" {
		workspaceRoot = os.Getenv("RUNGRID_WORKSPACE_ROOT")
	}
	if generationID == "" {
		generationID = os.Getenv("RUNGRID_GENERATION_ID")
	}
	if workspaceRoot == "" || !filepath.IsAbs(workspaceRoot) {
		return Context{}, errs.New(errs.ExitConflict, "RG701", "runtime workspace root is missing or invalid")
	}
	layout, err := state.NewLayout(projectID, stateOverride)
	if err != nil {
		return Context{}, err
	}
	current, err := state.CurrentGeneration(layout)
	if err != nil {
		return Context{}, err
	}
	if generationID == "" || current != generationID {
		return Context{}, errs.New(errs.ExitConflict, "RG702", "service wrapper generation is no longer current")
	}
	manifestPath := filepath.Join(layout.ProjectDir, "generations", generationID, "manifest.yaml")
	loadedManifest, err := manifest.LoadGenerated(manifestPath, workspaceRoot)
	if err != nil {
		return Context{}, err
	}
	if loadedManifest.Project.ID != projectID {
		return Context{}, errs.New(errs.ExitConflict, "RG703", "generated manifest belongs to another project")
	}
	return Context{Layout: layout, GenerationID: generationID, WorkspaceRoot: workspaceRoot, Manifest: loadedManifest}, nil
}

func Exec(ctx context.Context, runtimeContext Context, serviceName string) error {
	service, exists := manifest.FindService(runtimeContext.Manifest, serviceName)
	if !exists {
		return errs.New(errs.ExitUsage, "RG704", "service is not present in the generated manifest")
	}
	if service.Source == "external" {
		return errs.New(errs.ExitUsage, "RG705", "external services do not have supervised processes")
	}
	repositoryRoot, workingDirectory, err := serviceRoots(ctx, runtimeContext.Layout, runtimeContext.Manifest, runtimeContext.WorkspaceRoot, service)
	if err != nil {
		return err
	}
	envList, envMap, err := environment.Resolve(ctx, service, repositoryRoot)
	if err != nil {
		return err
	}
	argv := serviceArgv(service)
	if len(argv) == 0 {
		return errs.New(errs.ExitUsage, "RG706", "service has no executable argument vector")
	}
	executable, err := environment.LookPath(argv[0], workingDirectory, envMap)
	if err != nil {
		return errs.Wrap(errs.ExitDependency, "RG707", "resolve service executable", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		return errs.Wrap(errs.ExitDependency, "RG718", "enter service working directory", err)
	}
	return syscall.Exec(executable, append([]string{argv[0]}, argv[1:]...), envList)
}

func CheckHealth(ctx context.Context, runtimeContext Context, serviceName string) error {
	service, exists := manifest.FindService(runtimeContext.Manifest, serviceName)
	if !exists {
		return errs.New(errs.ExitUsage, "RG708", "service is not present in the generated manifest")
	}
	if service.Source == "external" {
		return CheckExternal(ctx, runtimeContext.Manifest, runtimeContext.WorkspaceRoot, service)
	}
	if service.Health == nil {
		return nil
	}
	repositoryRoot, workingDirectory, err := serviceRoots(ctx, runtimeContext.Layout, runtimeContext.Manifest, runtimeContext.WorkspaceRoot, service)
	if err != nil {
		return err
	}
	envList, envMap, err := environment.Resolve(ctx, service, repositoryRoot)
	if err != nil {
		return err
	}
	if service.Health.Command != nil {
		return runHealthCommand(ctx, service.Health.Command.Argv, workingDirectory, envList, envMap)
	}
	return requestHealth(ctx, service.Health.URL, service.Health.Timeout.Duration)
}

func CheckExternal(ctx context.Context, m *manifest.Manifest, root string, service *manifest.Service) error {
	if service.External == nil {
		return errs.New(errs.ExitUsage, "RG709", "external service configuration is missing")
	}
	repositoryRoot, err := manifest.ServiceRepositoryRoot(m, root, service)
	if err != nil {
		return err
	}
	workingDirectory, err := manifest.ServiceWorkingDirectory(m, root, service)
	if err != nil {
		return err
	}
	envList, envMap, err := environment.Resolve(ctx, service, repositoryRoot)
	if err != nil {
		return err
	}
	if service.External.Command != nil {
		return runHealthCommand(ctx, service.External.Command.Argv, workingDirectory, envList, envMap)
	}
	return requestHealth(ctx, service.External.URL, 3*time.Second)
}

func WaitExternal(ctx context.Context, m *manifest.Manifest, root string, service *manifest.Service) error {
	interval := 500 * time.Millisecond
	if service.Health != nil && service.Health.Interval.Duration > 0 {
		interval = service.Health.Interval.Duration
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := CheckExternal(ctx, m, root, service); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(errs.ExitNotReady, "RG710", fmt.Sprintf("external service %s was not ready", service.Name), ctx.Err())
		case <-ticker.C:
		}
	}
}

func ComposeShutdown(ctx context.Context, layout state.Layout, m *manifest.Manifest, service *manifest.Service, root string) error {
	if service.Compose == nil {
		return nil
	}
	repositoryRoot, workingDirectory, err := serviceRoots(ctx, layout, m, root, service)
	if err != nil {
		return err
	}
	_, envMap, err := environment.Resolve(ctx, service, repositoryRoot)
	if err != nil {
		return err
	}
	argv := composeBase(service.Compose.DownArgv, service.Compose)
	argv = append(argv, "stop", service.Compose.Service)
	executable, err := environment.LookPath(argv[0], workingDirectory, envMap)
	if err != nil {
		return errs.Wrap(errs.ExitDependency, "RG711", "resolve Compose shutdown executable", err)
	}
	command := exec.CommandContext(ctx, executable, argv[1:]...)
	command.Dir = workingDirectory
	command.Env = flattenMap(envMap)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return errs.Wrap(errs.ExitPartial, "RG712", fmt.Sprintf("Compose shutdown failed for %s (output redacted)", service.Name), err)
	}
	return nil
}

func serviceArgv(service *manifest.Service) []string {
	if service.Source == "native" && service.Run != nil {
		return append([]string(nil), service.Run.Argv...)
	}
	if service.Source == "compose" && service.Compose != nil {
		argv := composeBase(service.Compose.UpArgv, service.Compose)
		return append(argv, "up", "--no-color", service.Compose.Service)
	}
	return nil
}

func composeBase(base []string, compose *manifest.Compose) []string {
	result := append([]string(nil), base...)
	result = append(result, "--file", compose.File)
	if compose.ProjectName != "" {
		result = append(result, "--project-name", compose.ProjectName)
	}
	for _, profile := range compose.Profiles {
		result = append(result, "--profile", profile)
	}
	return result
}

func runHealthCommand(ctx context.Context, argv []string, directory string, envList []string, envMap map[string]string) error {
	executable, err := environment.LookPath(argv[0], directory, envMap)
	if err != nil {
		return errs.Wrap(errs.ExitDependency, "RG713", "resolve health executable", err)
	}
	command := exec.CommandContext(ctx, executable, argv[1:]...)
	command.Dir = directory
	command.Env = envList
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return errs.Wrap(errs.ExitNotReady, "RG714", "health command failed (output redacted)", err)
	}
	return nil
}

func requestHealth(ctx context.Context, rawURL string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, rawURL, nil)
	if err != nil {
		return errs.Wrap(errs.ExitUsage, "RG715", "create health request", err)
	}
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return errs.Wrap(errs.ExitNotReady, "RG716", "health request failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return errs.New(errs.ExitNotReady, "RG717", fmt.Sprintf("health request returned HTTP %d", response.StatusCode))
	}
	return nil
}

func flattenMap(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
