//go:build darwin || linux

package guardstate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

type ClientRegistration struct {
	layout   state.Layout
	path     string
	expected ControlClient
}

func RegisterControlClient(
	layout state.Layout,
	scope AuthorityScope,
	command *exec.Cmd,
	operation, service string,
	deadline time.Time,
) (*ClientRegistration, error) {
	if operation != "logs" && operation != "attach" {
		return nil, errs.New(errs.ExitUsage, "RG1311", "unsafe Process Compose client operation registration")
	}
	if command == nil || command.Process == nil || command.Process.Pid <= 1 {
		return nil, errs.New(errs.ExitConflict, "RG1312", "Process Compose client has not started")
	}
	parentPID := os.Getpid()
	pid, ppid, pgid, identity, err := inspectClientProcess(command.Process.Pid)
	if err != nil || ppid != parentPID || pgid != pid {
		return nil, errs.New(errs.ExitConflict, "RG1313", "Process Compose client is not an isolated child of the registering Rungrid process")
	}
	parentIdentity, err := procidentity.Current()
	if err != nil {
		return nil, errs.Wrap(errs.ExitConflict, "RG1314", "record Process Compose client parent identity", err)
	}
	record := ControlClient{
		APIVersion: apiVersion, Scope: scope, PID: pid, ProcessIdentity: identity,
		PGID: pgid, ParentPID: parentPID, ParentIdentity: parentIdentity,
		Operation: operation, Service: service, StartedAt: state.RuntimeTimestamp(),
	}
	if !deadline.IsZero() {
		record.DeadlineAt = deadline.UTC().Format(time.RFC3339Nano)
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%d.json", scope.GenerationID, pid)
	path := filepath.Join(layout.ProjectDir, "resource-guard", "clients", name)
	if err := state.WriteFileAtomic(layout.ProjectDir, filepath.Join("resource-guard", "clients", name), append(content, '\n'), 0o600); err != nil {
		return nil, err
	}
	return &ClientRegistration{layout: layout, path: path, expected: record}, nil
}

func (r *ClientRegistration) Release() error {
	var current ControlClient
	exists, err := readPrivateJSON(r.path, &current)
	if err != nil || !exists || current != r.expected {
		return err
	}
	if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func RemoveControlClient(layout state.Layout, expected ControlClient) error {
	name := fmt.Sprintf("%s-%d.json", expected.Scope.GenerationID, expected.PID)
	path := filepath.Join(layout.ProjectDir, "resource-guard", "clients", name)
	var current ControlClient
	exists, err := readPrivateJSON(path, &current)
	if err != nil || !exists {
		return err
	}
	if current != expected {
		return errs.New(errs.ExitConflict, "RG1316", "Process Compose client registration changed before removal")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ListControlClients(layout state.Layout, scope AuthorityScope) ([]ControlClient, error) {
	directory := filepath.Join(layout.ProjectDir, "resource-guard", "clients")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]ControlClient, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var record ControlClient
		exists, readErr := readPrivateJSON(filepath.Join(directory, entry.Name()), &record)
		if readErr != nil {
			return nil, readErr
		}
		if exists && record.APIVersion == apiVersion && record.Scope == scope {
			result = append(result, record)
		}
	}
	return result, nil
}

func inspectClientProcess(pid int) (int, int, int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "ps", "-o", "pid=,ppid=,pgid=,lstart=", "-p", strconv.Itoa(pid))
	command.Env = cLocaleEnvironment(os.Environ())
	result, err := subprocess.Run(command)
	if err != nil {
		return 0, 0, 0, "", err
	}
	fields := strings.Fields(string(result.Stdout))
	if len(fields) != 8 {
		return 0, 0, 0, "", fmt.Errorf("unexpected Process Compose client identity")
	}
	actualPID, pidErr := strconv.Atoi(fields[0])
	ppid, ppidErr := strconv.Atoi(fields[1])
	pgid, pgidErr := strconv.Atoi(fields[2])
	if pidErr != nil || ppidErr != nil || pgidErr != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid Process Compose client identity")
	}
	return actualPID, ppid, pgid, strings.Join(fields[3:], " "), nil
}

func cLocaleEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		if !strings.HasPrefix(value, "LC_ALL=") && !strings.HasPrefix(value, "LANG=") {
			result = append(result, value)
		}
	}
	return append(result, "LC_ALL=C")
}
