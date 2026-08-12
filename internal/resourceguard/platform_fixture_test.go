package resourceguard

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
)

func TestPlatformFixtureAuthorityBoundary(t *testing.T) {
	loaded, err := manifest.Load(filepath.Join("..", "..", "testdata", "platform-resource-boundary", ".rungrid.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Manifest.Project.ID != "lsmc-platform-local-r7g4k2" {
		t.Fatalf("unexpected Platform project identity: %s", loaded.Manifest.Project.ID)
	}
	runtimePID, aquariumPID := 10, 20
	owned := processSnapshot{
		runtimePID:  {PID: runtimePID, PPID: 1, PGID: runtimePID, StartIdentity: "platform-runtime"},
		aquariumPID: {PID: aquariumPID, PPID: runtimePID, PGID: aquariumPID, StartIdentity: "managed-aquarium", Threads: 1},
	}
	aquarium, _ := manifest.FindService(&loaded.Manifest, "aquarium")
	monitor := &serviceMonitor{service: aquarium, policy: loaded.Manifest.Runtime.ResourceGuard, baseline: &baselineTracker{learningWindow: time.Minute}}
	monitor.observe(time.Now(), time.Second, owned, processcompose.ProcessState{Name: "aquarium", Status: "Running", PID: aquariumPID}, runtimePID, 1<<30)
	if !monitor.authorityValid {
		t.Fatalf("Platform-owned Aquarium tree was not enforceable: %s", monitor.degradedReason)
	}
	manual := processSnapshot{aquariumPID: {PID: aquariumPID, PPID: 1, PGID: aquariumPID, StartIdentity: "manual-aquarium", Threads: 1}}
	monitor.observe(time.Now(), time.Second, manual, processcompose.ProcessState{Name: "aquarium", Status: "Running", PID: aquariumPID}, runtimePID, 1<<30)
	if monitor.authorityValid || monitor.state != "degraded" {
		t.Fatal("independently started Aquarium process acquired Platform authority")
	}
	for _, name := range []string{"postgres", "localstack"} {
		service, _ := manifest.FindService(&loaded.Manifest, name)
		external := &serviceMonitor{service: service, policy: loaded.Manifest.Runtime.ResourceGuard}
		external.observe(time.Now(), time.Second, manual, processcompose.ProcessState{Name: name, Status: "Running", PID: aquariumPID}, runtimePID, 1<<30)
		if external.state != "observe_only" || external.authorityValid {
			t.Fatalf("Platform external service %s acquired enforcement authority", name)
		}
	}
}
