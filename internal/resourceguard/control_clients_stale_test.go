//go:build darwin || linux

package resourceguard

import (
	"os"
	"os/exec"
	"testing"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/procidentity"
)

// A client that started after the last snapshot is absent from it; only the
// process itself can say whether the registration is stale.
func TestControlClientRegistrationStaleAsksTheProcess(t *testing.T) {
	t.Parallel()
	identity, err := procidentity.Current()
	if err != nil {
		t.Fatal(err)
	}
	live := guardstate.ControlClient{PID: os.Getpid(), ProcessIdentity: identity}
	if controlClientRegistrationStale(live) {
		t.Fatal("a live client was called stale")
	}
	reused := guardstate.ControlClient{PID: os.Getpid(), ProcessIdentity: "Thu Jan  1 00:00:00 1970"}
	if !controlClientRegistrationStale(reused) {
		t.Fatal("a reused PID was not called stale")
	}
	exited := exec.Command("true")
	if err := exited.Run(); err != nil {
		t.Fatal(err)
	}
	if !controlClientRegistrationStale(guardstate.ControlClient{PID: exited.Process.Pid, ProcessIdentity: identity}) {
		t.Fatal("an exited client was not called stale")
	}
}
