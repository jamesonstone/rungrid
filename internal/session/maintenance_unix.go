//go:build darwin || linux

package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
)

type maintenanceRequest struct {
	APIVersion      string `json:"api_version"`
	ProjectID       string `json:"project_id"`
	GenerationID    string `json:"generation_id"`
	Service         string `json:"service"`
	RequestID       string `json:"request_id"`
	SessionPID      int    `json:"session_pid"`
	SessionIdentity string `json:"session_identity"`
	Action          string `json:"action"`
	ExpiresAt       string `json:"expires_at"`
}

type maintenanceAck struct {
	APIVersion string `json:"api_version"`
	RequestID  string `json:"request_id"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

type MaintenanceHandle struct {
	layout  state.Layout
	request maintenanceRequest
}

func Pause(ctx context.Context, layout state.Layout, generationID, service string, timeout time.Duration) (*MaintenanceHandle, error) {
	registration, live := Active(layout, generationID, service)
	if !live {
		return nil, errs.New(errs.ExitConflict, "RG819", "tab-owned service has no live owning session")
	}
	requestID, err := randomRequestID()
	if err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG820", "create maintenance request id", err)
	}
	request := maintenanceRequest{
		APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID,
		GenerationID: generationID, Service: service, RequestID: requestID,
		SessionPID: registration.PID, SessionIdentity: registration.ProcessIdentity,
		Action: "pause", ExpiresAt: time.Now().Add(2 * timeout).UTC().Format(time.RFC3339Nano),
	}
	if err := writeMaintenanceRequest(layout, request); err != nil {
		return nil, err
	}
	handle := &MaintenanceHandle{layout: layout, request: request}
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := handle.wait(waitContext, "paused"); err != nil {
		// Withdraw the request so a late session does not act on it.
		handle.remove()
		return nil, err
	}
	return handle, nil
}

func (h *MaintenanceHandle) Resume(ctx context.Context) error {
	h.request.Action = "resume"
	if err := writeMaintenanceRequest(h.layout, h.request); err != nil {
		return err
	}
	err := h.wait(ctx, "running")
	h.remove()
	return err
}

func (h *MaintenanceHandle) wait(ctx context.Context, wanted string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ack, exists := readMaintenanceAck(h.layout, h.request); exists {
			if ack.State == "failed" {
				return errs.New(errs.ExitPartial, "RG821", "service maintenance transition failed: "+ack.Error)
			}
			if ack.State == wanted {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(errs.ExitNotReady, "RG822", "wait for service maintenance "+wanted, ctx.Err())
		case <-ticker.C:
		}
	}
}

func readMaintenanceRequest(layout state.Layout, identity Registration) (maintenanceRequest, bool) {
	content, err := os.ReadFile(maintenanceRequestPath(layout, identity.GenerationID, identity.Service))
	if err != nil {
		return maintenanceRequest{}, false
	}
	var request maintenanceRequest
	if json.Unmarshal(content, &request) != nil || request.APIVersion != "rungrid/output/v1" ||
		request.ProjectID != layout.ProjectID || request.GenerationID != identity.GenerationID ||
		request.Service != identity.Service || request.SessionPID != identity.PID ||
		request.SessionIdentity != identity.ProcessIdentity || request.RequestID == "" || requestExpired(request) {
		return maintenanceRequest{}, false
	}
	return request, true
}

func acknowledgeMaintenance(layout state.Layout, request maintenanceRequest, stateValue string, transitionErr error) error {
	ack := maintenanceAck{APIVersion: "rungrid/output/v1", RequestID: request.RequestID, State: stateValue, UpdatedAt: state.RuntimeTimestamp()}
	if transitionErr != nil {
		ack.State, ack.Error = "failed", transitionErr.Error()
	}
	content, err := json.MarshalIndent(ack, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(filepath.Dir(maintenanceAckPath(layout, request.GenerationID, request.Service)), filepath.Base(maintenanceAckPath(layout, request.GenerationID, request.Service)), append(content, '\n'), 0o600)
}

func writeMaintenanceRequest(layout state.Layout, request maintenanceRequest) error {
	if err := layout.Ensure(); err != nil {
		return err
	}
	content, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	path := maintenanceRequestPath(layout, request.GenerationID, request.Service)
	return state.WriteFileAtomic(filepath.Dir(path), filepath.Base(path), append(content, '\n'), 0o600)
}

func readMaintenanceAck(layout state.Layout, request maintenanceRequest) (maintenanceAck, bool) {
	content, err := os.ReadFile(maintenanceAckPath(layout, request.GenerationID, request.Service))
	if err != nil {
		return maintenanceAck{}, false
	}
	var ack maintenanceAck
	if json.Unmarshal(content, &ack) != nil || ack.APIVersion != "rungrid/output/v1" || ack.RequestID != request.RequestID {
		return maintenanceAck{}, false
	}
	return ack, true
}

func (h *MaintenanceHandle) remove() {
	removeMaintenanceTransition(h.layout, h.request)
}

func removeMaintenanceTransition(layout state.Layout, request maintenanceRequest) {
	current, exists := readMaintenanceRequestFile(layout, request.GenerationID, request.Service)
	if exists && current.RequestID != request.RequestID {
		return
	}
	for _, path := range []string{maintenanceRequestPath(layout, request.GenerationID, request.Service), maintenanceAckPath(layout, request.GenerationID, request.Service)} {
		_ = os.Remove(path)
	}
}

func readMaintenanceRequestFile(layout state.Layout, generationID, service string) (maintenanceRequest, bool) {
	content, err := os.ReadFile(maintenanceRequestPath(layout, generationID, service))
	if err != nil {
		return maintenanceRequest{}, false
	}
	var request maintenanceRequest
	return request, json.Unmarshal(content, &request) == nil
}

func maintenanceRequestPath(layout state.Layout, generationID, service string) string {
	return filepath.Join(layout.ProjectDir, "maintenance", fmt.Sprintf("%s-%s.request.json", generationID, service))
}

func maintenanceAckPath(layout state.Layout, generationID, service string) string {
	return filepath.Join(layout.ProjectDir, "maintenance", fmt.Sprintf("%s-%s.ack.json", generationID, service))
}

func randomRequestID() (string, error) {
	content := make([]byte, 12)
	if _, err := rand.Read(content); err != nil {
		return "", err
	}
	return hex.EncodeToString(content), nil
}
