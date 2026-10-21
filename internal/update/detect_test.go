package update

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
)

func TestMatchPattern(t *testing.T) {
	for _, tc := range []struct {
		pattern, path string
		want          bool
	}{
		{".", "any/file.go", true},
		{"migrations", "migrations/001.sql", true},
		{"migrations", "migrations", true},
		{"migrations", "migrationsx/001.sql", false},
		{"go.mod", "go.mod", true},
		{"*.go", "main.go", true},
		{"*.go", "internal/main.go", false},
		{"**/*.go", "internal/deep/main.go", true},
		{"**/*.go", "main.go", true},
		{"api/**", "api/v1/openapi.yaml", true},
		{"api/**/*.yaml", "api/openapi.yaml", true},
		{"api/**/*.yaml", "web/openapi.yaml", false},
	} {
		if got := matchPattern(tc.pattern, tc.path); got != tc.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestFingerprintTracksOnlyMatchedContent(t *testing.T) {
	before := tree{"web/package-lock.json": "100644 blob a", "web/app.tsx": "100644 blob b", "api/go.mod": "100644 blob c"}
	after := tree{"web/package-lock.json": "100644 blob a", "web/app.tsx": "100644 blob z", "api/go.mod": "100644 blob c"}
	lock := []string{"package-lock.json", "package.json"}
	if before.fingerprint("web", lock) != after.fingerprint("web", lock) {
		t.Fatal("unrelated source change altered the lockfile fingerprint")
	}
	if before.fingerprint("web", []string{"."}) == after.fingerprint("web", []string{"."}) {
		t.Fatal("source change did not alter the directory fingerprint")
	}
	if before.fingerprint("api", []string{"."}) != after.fingerprint("api", []string{"."}) {
		t.Fatal("change outside the base altered its fingerprint")
	}
}

func TestDetectDependencies(t *testing.T) {
	files := tree{
		"go.mod": "", "go.sum": "",
		"web/yarn.lock": "", "web/.yarnrc.yml": "", "web/package.json": "",
		"admin/pnpm-lock.yaml": "", "admin/package.json": "",
		"docs/README.md": "",
	}
	ids := func(base string) map[string][]string {
		result := map[string][]string{}
		for _, item := range detectDependencies(files, base) {
			result[item.ID] = item.Argv
		}
		return result
	}
	if got, want := ids("cmd/server"), map[string][]string{"dependencies/go/.": {"go", "mod", "download"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested Go service = %v, want %v", got, want)
	}
	if got := ids("web/src"); !reflect.DeepEqual(got["dependencies/yarn/web"], []string{"yarn", "install", "--immutable"}) {
		t.Errorf("yarn berry = %v", got)
	}
	if got := ids("admin"); !reflect.DeepEqual(got["dependencies/pnpm/admin"], []string{"pnpm", "install", "--frozen-lockfile"}) {
		t.Errorf("pnpm = %v", got)
	}
	if got := ids("docs"); len(got) != 1 {
		t.Errorf("docs should share only the root Go module, got %v", got)
	}
}

func TestRestartPolicyDetection(t *testing.T) {
	directory := t.TempDir()
	writeFile(t, filepath.Join(directory, "package.json"), `{"scripts":{"start":"expo start --port 8464","serve":"node server.js"}}`)
	service := func(argv ...string) Service {
		return Service{Config: &manifest.Service{Name: "web", Source: "native", Run: &manifest.Run{Argv: argv}}, WorkingDirectory: directory}
	}
	for _, tc := range []struct {
		service Service
		want    string
	}{
		{service("npm", "run", "start"), manifest.RefreshRestartDependencies},
		{service("npm", "start"), manifest.RefreshRestartDependencies},
		{service("npx", "vite", "--port", "3000"), manifest.RefreshRestartDependencies},
		{service("npm", "run", "serve"), manifest.RefreshRestartChanged},
		{service("go", "run", "./cmd/api"), manifest.RefreshRestartChanged},
		{service("air"), manifest.RefreshRestartDependencies},
	} {
		if got, _ := restartPolicy(tc.service); got != tc.want {
			t.Errorf("restartPolicy(%v) = %s, want %s", tc.service.Config.Run.Argv, got, tc.want)
		}
	}
	declared := service("npm", "run", "start")
	declared.Config.Refresh = &manifest.Refresh{Restart: manifest.RefreshRestartChanged}
	if got, source := restartPolicy(declared); got != manifest.RefreshRestartChanged || source != "declared" {
		t.Errorf("declared policy = %s (%s)", got, source)
	}
}

func TestComposeServicesWatchOnlyTheirComposeFile(t *testing.T) {
	service := &manifest.Service{Name: "db", Source: "compose", Compose: &manifest.Compose{File: "docker-compose.yml", Service: "db"}}
	if got := watchPatterns(service); !reflect.DeepEqual(got, []string{"docker-compose.yml"}) || dependencyMode(service) != "none" {
		t.Fatalf("compose watch = %v, dependencies = %s", got, dependencyMode(service))
	}
}

func TestNodeModulesMissingOnlyWhenDependenciesAreDeclared(t *testing.T) {
	empty, declared := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(empty, "package.json"), `{"name":"empty"}`)
	writeFile(t, filepath.Join(declared, "package.json"), `{"dependencies":{"expo-sqlite":"~16.0.0"}}`)
	if nodeModulesMissing(empty) {
		t.Fatal("a package without dependencies never gets node_modules and must not rerun forever")
	}
	if !nodeModulesMissing(declared) {
		t.Fatal("a package with dependencies and no node_modules must reinstall")
	}
	writeFile(t, filepath.Join(declared, "node_modules", ".package-lock.json"), "{}")
	if nodeModulesMissing(declared) {
		t.Fatal("installed node_modules reported missing")
	}
}
