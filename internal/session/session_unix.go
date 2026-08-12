//go:build darwin || linux

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/state"
)

type Registration struct {
	APIVersion      string `json:"api_version"`
	ProjectID       string `json:"project_id"`
	GenerationID    string `json:"generation_id"`
	Service         string `json:"service"`
	PID             int    `json:"pid"`
	ProcessIdentity string `json:"process_identity"`
	TabID           string `json:"tab_id,omitempty"`
	StartedAt       string `json:"started_at"`
}

type Lock struct {
	file         *os.File
	registration string
	identity     Registration
}

func Acquire(layout state.Layout, generationID, service, tabID string) (*Lock, error) {
	if err := layout.Ensure(); err != nil {
		return nil, err
	}
	shutdownMarker := filepath.Join(layout.ProjectDir, "locks", "down-"+generationID+".json")
	if _, err := os.Lstat(shutdownMarker); err == nil {
		return nil, errs.New(errs.ExitConflict, "RG815", "workspace shutdown is in progress")
	} else if !os.IsNotExist(err) {
		return nil, errs.Wrap(errs.ExitConflict, "RG816", "inspect workspace shutdown marker", err)
	}
	lockPath := filepath.Join(layout.ProjectDir, "locks", generationID+"-"+service+".lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errs.Wrap(errs.ExitConflict, "RG801", "open service session lock", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, errs.Wrap(errs.ExitConflict, "RG802", "secure service session lock", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errs.New(errs.ExitConflict, "RG803", fmt.Sprintf("service %s already has an owning session", service))
	}
	if _, err := os.Lstat(shutdownMarker); err == nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, errs.New(errs.ExitConflict, "RG817", "workspace shutdown started while acquiring session ownership")
	}
	registration := filepath.Join(layout.ProjectDir, "sessions", generationID+"-"+service+".json")
	processIdentity, err := procidentity.Current()
	if err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, errs.Wrap(errs.ExitConflict, "RG818", "record service session process identity", err)
	}
	identity := Registration{
		APIVersion:      "rungrid/output/v1",
		ProjectID:       layout.ProjectID,
		GenerationID:    generationID,
		Service:         service,
		PID:             os.Getpid(),
		ProcessIdentity: processIdentity,
		TabID:           tabID,
		StartedAt:       state.RuntimeTimestamp(),
	}
	content, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, errs.Wrap(errs.ExitFailure, "RG804", "encode session registration", err)
	}
	if err := state.WriteFileAtomic(filepath.Dir(registration), filepath.Base(registration), append(content, '\n'), 0o600); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	return &Lock{file: file, registration: registration, identity: identity}, nil
}

func (l *Lock) Release() error {
	var result error
	if content, err := os.ReadFile(l.registration); err == nil {
		var current Registration
		if json.Unmarshal(content, &current) == nil && current == l.identity {
			if err := os.Remove(l.registration); err != nil && !os.IsNotExist(err) {
				result = errs.Wrap(errs.ExitPartial, "RG805", "remove service session registration", err)
			}
		}
	}
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil && result == nil {
		result = errs.Wrap(errs.ExitPartial, "RG806", "unlock service session", err)
	}
	if err := l.file.Close(); err != nil && result == nil {
		result = errs.Wrap(errs.ExitPartial, "RG807", "close service session lock", err)
	}
	return result
}

func Active(layout state.Layout, generationID, service string) (Registration, bool) {
	filename := filepath.Join(layout.ProjectDir, "sessions", generationID+"-"+service+".json")
	content, err := os.ReadFile(filename)
	if err != nil {
		return Registration{}, false
	}
	var registration Registration
	if json.Unmarshal(content, &registration) != nil || registration.ProjectID != layout.ProjectID || registration.GenerationID != generationID || registration.Service != service {
		return Registration{}, false
	}
	if !procidentity.Matches(registration.PID, registration.ProcessIdentity) {
		return Registration{}, false
	}
	return registration, true
}

func WaitGenerationReleased(ctx context.Context, layout state.Layout, generationID string, services []string) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		owned := false
		for _, service := range services {
			if _, live := Active(layout, generationID, service); live {
				owned = true
				break
			}
		}
		if !owned {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
