package environment

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func Resolve(ctx context.Context, service *manifest.Service, root string) ([]string, map[string]string, error) {
	workingDirectory := filepath.Join(root, service.WorkingDirectory)
	return ResolveEnvironment(ctx, service.Environment, workingDirectory, root)
}

func ResolveEnvironment(
	ctx context.Context,
	configuration manifest.Environment,
	workingDirectory string,
	serviceRoot string,
) ([]string, map[string]string, error) {
	values := parseEnvironment(os.Environ())
	for _, provider := range configuration.Providers {
		switch provider.Type {
		case "dotenv":
			filename := filepath.Join(workingDirectory, provider.Path)
			if err := ensureProviderPath(serviceRoot, filename, provider.Optional); err != nil {
				return nil, nil, err
			}
			content, err := os.ReadFile(filename)
			if err != nil {
				if provider.Optional && os.IsNotExist(err) {
					continue
				}
				return nil, nil, errs.Wrap(errs.ExitDependency, "RG501", "read dotenv environment provider", err)
			}
			providerValues, err := parseDotenv(content)
			if err != nil {
				return nil, nil, err
			}
			merge(values, providerValues)
		case "command":
			providerContext, cancel := context.WithTimeout(ctx, provider.Timeout.Duration)
			providerValues, err := runCommandProvider(providerContext, provider.Argv, workingDirectory, values)
			cancel()
			if err != nil {
				return nil, nil, err
			}
			merge(values, providerValues)
		case "direnv":
			providerContext, cancel := context.WithTimeout(ctx, provider.Timeout.Duration)
			directory := filepath.Join(workingDirectory, provider.Directory)
			if err := ensureProviderPath(serviceRoot, directory, false); err != nil {
				cancel()
				return nil, nil, err
			}
			providerValues, err := runDirenv(providerContext, directory, values)
			cancel()
			if err != nil {
				return nil, nil, err
			}
			merge(values, providerValues)
		default:
			return nil, nil, errs.New(errs.ExitUsage, "RG502", "unsupported environment provider")
		}
	}
	merge(values, configuration.Values)
	return flatten(values), values, nil
}

// CheckProviderPath applies the execution-time provider containment check:
// candidate must resolve, through symlinks, inside root, and must exist unless
// optional. Override validation uses it before stopping any service.
func CheckProviderPath(root, candidate string, optional bool) error {
	return ensureProviderPath(root, candidate, optional)
}

func ensureProviderPath(root, candidate string, optional bool) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errs.Wrap(errs.ExitConflict, "RG514", "resolve environment provider service root", err)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil && optional && os.IsNotExist(err) {
		resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(candidate))
		if parentErr != nil {
			return errs.Wrap(errs.ExitConflict, "RG511", "resolve optional environment provider parent", parentErr)
		}
		resolved = filepath.Join(resolvedParent, filepath.Base(candidate))
		err = nil
	}
	if err != nil {
		return errs.Wrap(errs.ExitConflict, "RG512", "resolve environment provider path", err)
	}
	if !pathWithin(resolvedRoot, resolved) {
		return errs.New(errs.ExitConflict, "RG513", "environment provider path resolves outside the service repository")
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func parseDotenv(content []byte) (map[string]string, error) {
	result := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 1024), subprocess.DefaultLimit)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		index := strings.IndexByte(line, '=')
		if index <= 0 {
			return nil, errs.New(errs.ExitDependency, "RG503", fmt.Sprintf("invalid dotenv provider line %d", lineNumber))
		}
		key := strings.TrimSpace(line[:index])
		if !validKey(key) {
			return nil, errs.New(errs.ExitDependency, "RG504", fmt.Sprintf("invalid dotenv key on line %d", lineNumber))
		}
		value := strings.TrimSpace(line[index+1:])
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		result[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG505", "read dotenv provider output", err)
	}
	return result, nil
}

func runCommandProvider(ctx context.Context, argv []string, directory string, environment map[string]string) (map[string]string, error) {
	path, err := LookPath(argv[0], directory, environment)
	if err != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG506", "resolve environment provider executable", err)
	}
	command := exec.CommandContext(ctx, path, argv[1:]...)
	command.Dir = directory
	command.Env = flatten(environment)
	result, err := subprocess.Run(command)
	if err != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG507", "environment command provider failed (output redacted)", err)
	}
	return parseDotenv(result.Stdout)
}

func runDirenv(ctx context.Context, directory string, environment map[string]string) (map[string]string, error) {
	path, err := LookPath("direnv", directory, environment)
	if err != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG508", "resolve direnv executable", err)
	}
	command := exec.CommandContext(ctx, path, "export", "json")
	command.Dir = directory
	command.Env = flatten(environment)
	result, err := subprocess.Run(command)
	if err != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG509", "direnv provider failed (output redacted)", err)
	}
	var values map[string]string
	if err := json.Unmarshal(result.Stdout, &values); err != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG510", "decode direnv provider output", err)
	}
	return values, nil
}

func parseEnvironment(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		if index := strings.IndexByte(value, '='); index > 0 {
			result[value[:index]] = value[index+1:]
		}
	}
	return result
}

func flatten(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func merge(target, source map[string]string) {
	for key, value := range source {
		target[key] = value
	}
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for index, r := range key {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func LookPath(file, directory string, environment map[string]string) (string, error) {
	if strings.ContainsRune(file, filepath.Separator) {
		candidate := file
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(directory, candidate)
		}
		return executable(candidate)
	}
	pathValue := environment["PATH"]
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	for _, directoryName := range filepath.SplitList(pathValue) {
		if directoryName == "" {
			directoryName = directory
		}
		if candidate, err := executable(filepath.Join(directoryName, file)); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func executable(filename string) (string, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("not executable")
	}
	return filepath.Clean(filename), nil
}
