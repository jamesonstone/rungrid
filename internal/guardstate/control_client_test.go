//go:build darwin || linux

package guardstate

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestControlClientRegistrationRequiresIsolatedDirectChild(t *testing.T) {
	layout := guardLayout(t)
	command := exec.Command("sleep", "10")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
	})
	registration, err := RegisterControlClient(layout, testScope(), command, "logs", "api", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := ListControlClients(layout, testScope())
	if err != nil || len(clients) != 1 || clients[0].PID != command.Process.Pid || clients[0].PGID != command.Process.Pid {
		t.Fatalf("unexpected clients: %#v, %v", clients, err)
	}
	otherScope := testScope()
	otherScope.GenerationID = "ffffffffffffffffffff"
	if clients, err := ListControlClients(layout, otherScope); err != nil || len(clients) != 0 {
		t.Fatalf("another generation observed registered clients: %#v, %v", clients, err)
	}
	if err := registration.Release(); err != nil {
		t.Fatal(err)
	}
	if clients, err := ListControlClients(layout, testScope()); err != nil || len(clients) != 0 {
		t.Fatalf("released client remains: %#v, %v", clients, err)
	}
}

func TestControlClientRegistrationRejectsUnknownOperation(t *testing.T) {
	command := exec.Command("sleep", "1")
	if _, err := RegisterControlClient(guardLayout(t), testScope(), command, "process-get", "api", time.Time{}); err == nil {
		t.Fatal("unsafe unbounded query operation was registered")
	}
}
