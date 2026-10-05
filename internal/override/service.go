package override

import (
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/environment"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

// checkService proves that one affected service can run from the override and
// returns every argument that will be re-anchored.
func checkService(loaded *manifest.Loaded, service *manifest.Service, view mapping) ([]Reanchor, error) {
	declared, err := declaredWorkingDirectory(loaded, service)
	if err != nil {
		return nil, err
	}
	working := view.mapPath(declared)
	if info, statErr := os.Stat(working); statErr != nil || !info.IsDir() {
		return nil, errs.New(errs.ExitConflict, "RG1813", "service "+service.Name+" working directory does not exist in the override checkout: "+working)
	}
	if service.Compose != nil && !within(view.override, filepath.Join(working, service.Compose.File)) {
		return nil, errs.New(errs.ExitConflict, "RG1814", "service "+service.Name+" compose file "+service.Compose.File+" leaves the override checkout")
	}
	root := executionRoot(loaded, service, view)
	for _, provider := range service.Environment.Providers {
		if err := checkProvider(service.Name, provider, working, root, declared, view); err != nil {
			return nil, err
		}
	}
	return serviceReanchors(service, declared, view), nil
}

// checkProvider keeps environment providers inside the override checkout, as
// the Constitution requires of every selected checkout and as checkService
// requires of Compose files. Rungrid never rewrites or synthesizes environment
// material.
func checkProvider(serviceName string, provider manifest.EnvironmentProvider, working, root, declared string, view mapping) error {
	field := "service " + serviceName + " environment provider " + provider.Type
	switch provider.Type {
	case "dotenv":
		if err := environment.CheckProviderPath(root, filepath.Join(working, provider.Path), provider.Optional); err != nil {
			return errs.Wrap(errs.ExitConflict, "RG1814", field+" path "+provider.Path+" is missing from or leaves the override checkout", err)
		}
	case "direnv":
		if err := environment.CheckProviderPath(root, filepath.Join(working, provider.Directory), false); err != nil {
			return errs.Wrap(errs.ExitConflict, "RG1814", field+" directory "+provider.Directory+" is missing from or leaves the override checkout", err)
		}
	case "command":
		if _, changes := view.rewriteArgv(provider.Argv, declared); len(changes) > 0 {
			return errs.New(errs.ExitConflict, "RG1814", field+" argument "+changes[0].Original+" leaves the original checkout; providers are not re-anchored")
		}
	}
	return nil
}

func serviceReanchors(service *manifest.Service, declared string, view mapping) []Reanchor {
	var result []Reanchor
	add := func(field string, argv []string) {
		_, changes := view.rewriteArgv(argv, declared)
		for _, change := range changes {
			change.Service, change.Field = service.Name, field
			result = append(result, change)
		}
	}
	if service.Run != nil {
		add("run.argv", service.Run.Argv)
	}
	if service.Health != nil && service.Health.Command != nil {
		add("health.command.argv", service.Health.Command.Argv)
	}
	if service.Compose != nil {
		add("compose.up_argv", service.Compose.UpArgv)
		add("compose.down_argv", service.Compose.DownArgv)
	}
	return result
}

// executionRoot is the containment root a service runs with under an override:
// the mapped declared repository root when it lies inside the original
// checkout, and otherwise, as for the implicit workspace, the override checkout.
func executionRoot(loaded *manifest.Loaded, service *manifest.Service, view mapping) string {
	if declaredRoot, err := manifest.ServiceRepositoryRoot(&loaded.Manifest, loaded.WorkspaceRoot, service); err == nil {
		if physical, physicalErr := physicalPath(declaredRoot); physicalErr == nil && within(view.original, physical) {
			return view.mapPath(physical)
		}
	}
	return view.override
}

func declaredWorkingDirectory(loaded *manifest.Loaded, service *manifest.Service) (string, error) {
	declared, err := manifest.ServiceWorkingDirectory(&loaded.Manifest, loaded.WorkspaceRoot, service)
	if err != nil {
		return "", err
	}
	return physicalPath(declared)
}
