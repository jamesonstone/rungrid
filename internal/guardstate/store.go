package guardstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
)

const apiVersion = "rungrid/output/v1"

func WriteStatus(layout state.Layout, status Status) error {
	status.APIVersion = apiVersion
	content, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1301", "encode resource guard status", err)
	}
	return state.WriteFileAtomic(layout.ProjectDir, filepath.Join("resource-guard", "status.json"), append(content, '\n'), 0o600)
}

func ReadStatus(layout state.Layout) (Status, bool, error) {
	var status Status
	exists, err := readPrivateJSON(filepath.Join(layout.ProjectDir, "resource-guard", "status.json"), &status)
	if err != nil || !exists {
		return Status{}, exists, err
	}
	if status.APIVersion != apiVersion || status.ProjectID != layout.ProjectID {
		return Status{}, false, errs.New(errs.ExitConflict, "RG1302", "resource guard status ownership does not match")
	}
	return status, true, nil
}

func WriteBaseline(layout state.Layout, baseline Baseline) error {
	baseline.APIVersion = apiVersion
	content, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1303", "encode resource guard baseline", err)
	}
	if err := state.WriteFileAtomic(
		layout.ProjectDir,
		filepath.Join("resource-guard", "baselines", baselineKey(baseline.Service, baseline.ServiceIdentity)+".json"),
		append(content, '\n'),
		0o600,
	); err != nil {
		return err
	}
	return pruneBaselines(layout, 256)
}

func ReadBaseline(layout state.Layout, scope AuthorityScope, service, identity string) (Baseline, bool, error) {
	var baseline Baseline
	filename := filepath.Join(layout.ProjectDir, "resource-guard", "baselines", baselineKey(service, identity)+".json")
	exists, err := readPrivateJSON(filename, &baseline)
	if err != nil || !exists {
		return Baseline{}, exists, err
	}
	if baseline.APIVersion != apiVersion || baseline.Scope.ProjectID != layout.ProjectID ||
		baseline.Service != service || baseline.ServiceIdentity != identity {
		return Baseline{}, false, errs.New(errs.ExitConflict, "RG1304", "resource guard baseline scope does not match")
	}
	if !SameEffectiveGeneration(baseline.Scope, scope) {
		return Baseline{}, false, nil
	}
	return baseline, true, nil
}

// SameEffectiveGeneration permits learning and circuit history to survive a
// normal runtime restart. Ephemeral PID, process-start, and socket-inode proofs
// remain recorded as provenance but never become authority for the new runtime.
func SameEffectiveGeneration(left, right AuthorityScope) bool {
	return left.ProjectID == right.ProjectID &&
		left.GenerationID == right.GenerationID &&
		left.EffectiveManifestSHA256 == right.EffectiveManifestSHA256 &&
		left.RuntimeCommandSHA256 == right.RuntimeCommandSHA256 &&
		left.SocketPath == right.SocketPath &&
		left.SocketOwnerUID == right.SocketOwnerUID &&
		left.SocketDevice == right.SocketDevice &&
		left.ProcessComposeConfigHash == right.ProcessComposeConfigHash
}

func WriteIncident(layout state.Layout, incident Incident, retention time.Duration) error {
	incident.APIVersion = apiVersion
	content, err := json.MarshalIndent(incident, "", "  ")
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1305", "encode resource guard incident", err)
	}
	stamp, err := time.Parse(time.RFC3339Nano, incident.OccurredAt)
	if err != nil {
		return errs.Wrap(errs.ExitFailure, "RG1306", "validate resource guard incident time", err)
	}
	name := fmt.Sprintf("%020d-%s.json", stamp.UnixNano(), incident.ID)
	if err := state.WriteFileAtomic(layout.ProjectDir, filepath.Join("resource-guard", "incidents", name), append(content, '\n'), 0o600); err != nil {
		return err
	}
	return pruneIncidents(layout, retention, 256)
}

func MarkShutdown(layout state.Layout, generationID string) error {
	status, exists, err := ReadStatus(layout)
	if err != nil || !exists || status.GenerationID != generationID {
		return err
	}
	status.Shutdown = true
	status.Health = "stopping"
	status.HeartbeatAt = state.RuntimeTimestamp()
	return WriteStatus(layout, status)
}

func LatestServiceIncident(layout state.Layout, generationID, service, afterID string) (*IncidentSummary, error) {
	status, exists, err := ReadStatus(layout)
	if err != nil || !exists || status.GenerationID != generationID {
		return nil, err
	}
	for index := range status.Services {
		latest := status.Services[index].LatestIncident
		if status.Services[index].Name == service && latest != nil && latest.ID != afterID {
			copy := *latest
			return &copy, nil
		}
	}
	return nil, nil
}

func ResetCircuit(layout state.Layout, scope AuthorityScope, service string) error {
	status, exists, err := ReadStatus(layout)
	if err != nil {
		return err
	}
	if !exists || status.Scope != scope || !status.AuthorityValid {
		return errs.New(errs.ExitConflict, "RG1307", "resource guard scope is not valid for circuit reset")
	}
	for index := range status.Services {
		item := &status.Services[index]
		if item.Name != service {
			continue
		}
		item.CircuitState = "closed"
		item.RestartHistory = nil
		item.RestartCount = 0
		item.State = "monitoring"
		if err := WriteStatus(layout, status); err != nil {
			return err
		}
		request := CircuitReset{
			APIVersion: apiVersion, Scope: scope, Service: service,
			RequestedAt: state.RuntimeTimestamp(),
		}
		content, marshalErr := json.MarshalIndent(request, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		return state.WriteFileAtomic(
			layout.ProjectDir,
			circuitResetPath(service),
			append(content, '\n'),
			0o600,
		)
	}
	return errs.New(errs.ExitUsage, "RG1308", "resource guard service status is unavailable: "+service)
}

func ConsumeCircuitReset(layout state.Layout, scope AuthorityScope, service string) (bool, error) {
	path := filepath.Join(layout.ProjectDir, circuitResetPath(service))
	var request CircuitReset
	exists, err := readPrivateJSON(path, &request)
	if err != nil || !exists {
		return false, err
	}
	if request.APIVersion != apiVersion || request.Scope != scope || request.Service != service {
		return false, errs.New(errs.ExitConflict, "RG1315", "resource circuit reset scope does not match")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

func readPrivateJSON(filename string, target any) (bool, error) {
	info, err := os.Lstat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false, errs.New(errs.ExitConflict, "RG1309", "resource guard state is not a private regular file")
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(content, target); err != nil {
		return false, errs.Wrap(errs.ExitConflict, "RG1310", "decode resource guard state", err)
	}
	return true, nil
}

func baselineKey(service, identity string) string {
	return state.Hash([]byte(service), []byte(identity))[:32]
}

func circuitResetPath(service string) string {
	return filepath.Join("resource-guard", "resets", baselineKey(service, "circuit-reset")+".json")
}

func pruneIncidents(layout state.Layout, retention time.Duration, maximum int) error {
	directory := filepath.Join(layout.ProjectDir, "resource-guard", "incidents")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	cutoff := time.Now().Add(-retention)
	removeCount := len(entries) - maximum
	for index, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filename := filepath.Join(directory, entry.Name())
		var incident Incident
		exists, readErr := readPrivateJSON(filename, &incident)
		if readErr != nil || !exists || incident.Scope.ProjectID != layout.ProjectID {
			continue
		}
		occurred, parseErr := time.Parse(time.RFC3339Nano, incident.OccurredAt)
		if index < removeCount || (parseErr == nil && occurred.Before(cutoff)) {
			if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
