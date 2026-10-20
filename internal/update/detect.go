package update

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/manifest"
)

// dependency is one detected install keyed by ecosystem and directory, shared
// by every service whose working directory resolves to that module.
type dependency struct {
	ID        string
	Name      string
	Directory string
	Argv      []string
	Inputs    []string
	Services  []string
	Owner     *manifest.Service
	// NodeModules marks a JavaScript install whose absence of node_modules
	// forces a run even when the lockfile fingerprint is current.
	NodeModules bool
}

var javascriptLockfiles = []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock"}

// detectDependencies finds the nearest Go module and the nearest JavaScript
// lockfile at or above base, without leaving the repository top level.
func detectDependencies(t tree, base string) []dependency {
	var result []dependency
	if directory, ok := nearest(t, base, func(directory string) bool { return t.has(join(directory, "go.mod")) }); ok {
		result = append(result, dependency{
			ID: "dependencies/go/" + displayDirectory(directory), Name: "go-mod-download", Directory: directory,
			Argv: []string{"go", "mod", "download"}, Inputs: []string{"go.mod", "go.sum"},
		})
	}
	lockfile := ""
	directory, ok := nearest(t, base, func(directory string) bool {
		for _, candidate := range javascriptLockfiles {
			if t.has(join(directory, candidate)) {
				lockfile = candidate
				return true
			}
		}
		return false
	})
	if ok {
		item := dependency{Directory: directory, Inputs: []string{lockfile, "package.json"}, NodeModules: true}
		switch lockfile {
		case "pnpm-lock.yaml":
			item.Name, item.Argv = "pnpm-install", []string{"pnpm", "install", "--frozen-lockfile"}
		case "yarn.lock":
			item.Name, item.Argv = "yarn-install", []string{"yarn", "install", "--frozen-lockfile"}
			if t.has(join(directory, ".yarnrc.yml")) {
				item.Argv = []string{"yarn", "install", "--immutable"}
			}
		default:
			item.Name, item.Argv = "npm-ci", []string{"npm", "ci"}
		}
		item.ID = "dependencies/" + strings.TrimSuffix(item.Name, "-install") + "/" + displayDirectory(directory)
		result = append(result, item)
	}
	return result
}

func nearest(t tree, base string, found func(string) bool) (string, bool) {
	directory := base
	for {
		if found(directory) {
			return directory, true
		}
		if directory == "" {
			return "", false
		}
		directory = path.Dir(directory)
		if directory == "." {
			directory = ""
		}
	}
}

func displayDirectory(directory string) string {
	if directory == "" {
		return "."
	}
	return directory
}

func dependencyMode(service *manifest.Service) string {
	if service.Source != "native" {
		return "none"
	}
	if service.Refresh != nil && service.Refresh.Dependencies != "" {
		return service.Refresh.Dependencies
	}
	return "auto"
}

func watchPatterns(service *manifest.Service) []string {
	if service.Refresh != nil && len(service.Refresh.Watch) > 0 {
		return service.Refresh.Watch
	}
	if service.Compose != nil {
		return []string{filepath.ToSlash(service.Compose.File)}
	}
	return []string{"."}
}

// restartPolicy returns the effective restart policy and where it came from.
func restartPolicy(service Service) (string, string) {
	if service.Config.Refresh != nil && service.Config.Refresh.Restart != "" {
		return service.Config.Refresh.Restart, "declared"
	}
	if hotReloading(service) {
		return manifest.RefreshRestartDependencies, "detected hot reload"
	}
	return manifest.RefreshRestartChanged, "default"
}

var hotReloadMarkers = []string{
	"expo start", "react-native start", "metro", "vite", "next dev", "webpack serve",
	"webpack-dev-server", "nodemon", "ng serve", "astro dev", "react-scripts start", "remix dev", "nuxt dev",
}

// hotReloading recognizes development servers that pick up source changes
// without a restart, from the service argv and the package.json script it
// invokes.
func hotReloading(service Service) bool {
	if service.Config.Run == nil || len(service.Config.Run.Argv) == 0 {
		return false
	}
	argv := service.Config.Run.Argv
	if filepath.Base(argv[0]) == "air" {
		return true
	}
	text := strings.Join(argv, " ")
	if script := packageScript(argv, service.WorkingDirectory); script != "" {
		text += " " + script
	}
	for _, marker := range hotReloadMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// nodeModulesMissing reports a JavaScript package that declares dependencies
// but has no node_modules directory, such as a fresh clone. A package with no
// dependencies never gets node_modules, so it is never reported missing.
func nodeModulesMissing(directory string) bool {
	if _, err := os.Stat(filepath.Join(directory, "node_modules")); !os.IsNotExist(err) {
		return false
	}
	content, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		return false
	}
	var declared struct {
		Dependencies    map[string]any `json:"dependencies"`
		DevDependencies map[string]any `json:"devDependencies"`
	}
	return json.Unmarshal(content, &declared) == nil && len(declared.Dependencies)+len(declared.DevDependencies) > 0
}

func packageScript(argv []string, workingDirectory string) string {
	switch filepath.Base(argv[0]) {
	case "npm", "pnpm", "yarn", "bun":
	default:
		return ""
	}
	if len(argv) < 2 {
		return ""
	}
	name := argv[1]
	if (name == "run" || name == "run-script") && len(argv) > 2 {
		name = argv[2]
	}
	content, err := os.ReadFile(filepath.Join(workingDirectory, "package.json"))
	if err != nil {
		return ""
	}
	var manifestFile struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(content, &manifestFile) != nil {
		return ""
	}
	return manifestFile.Scripts[name]
}
