package guardstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/state"
)

func TestStatusAndBaselineRequireStableGenerationAndIdentity(t *testing.T) {
	layout := guardLayout(t)
	scope := testScope()
	status := Status{ProjectID: layout.ProjectID, GenerationID: scope.GenerationID, Scope: scope, Health: "healthy"}
	if err := WriteStatus(layout, status); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := ReadStatus(layout)
	if err != nil || !exists || loaded.Scope != scope {
		t.Fatalf("unexpected status: %#v, %t, %v", loaded, exists, err)
	}
	baseline := Baseline{Scope: scope, Service: "api", ServiceIdentity: "identity", HealthyDuration: "15m0s"}
	if err := WriteBaseline(layout, baseline); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := ReadBaseline(layout, scope, "api", "identity"); err != nil || !exists {
		t.Fatalf("exact baseline did not load: %t, %v", exists, err)
	}
	rotated := scope
	rotated.RuntimePID++
	rotated.RuntimeProcessIdentity = "new runtime"
	rotated.SocketInode++
	if _, exists, err := ReadBaseline(layout, rotated, "api", "identity"); err != nil || !exists {
		t.Fatalf("stable generation baseline did not survive runtime rotation: %t, %v", exists, err)
	}
	changedCommand := rotated
	changedCommand.RuntimeCommandSHA256 = "changed"
	if _, exists, err := ReadBaseline(layout, changedCommand, "api", "identity"); err != nil || exists {
		t.Fatalf("changed runtime command reused baseline: %t, %v", exists, err)
	}
	if _, exists, err := ReadBaseline(layout, scope, "api", "changed"); err != nil || exists {
		t.Fatalf("changed service identity reused the previous baseline: %t, %v", exists, err)
	}
}

func TestCircuitResetIsDurableAndScopeVerified(t *testing.T) {
	layout := guardLayout(t)
	scope := testScope()
	status := Status{
		ProjectID: layout.ProjectID, GenerationID: scope.GenerationID, Scope: scope,
		AuthorityValid: true,
		Services:       []ServiceStatus{{Name: "api", CircuitState: "open", RestartCount: 3}},
	}
	if err := WriteStatus(layout, status); err != nil {
		t.Fatal(err)
	}
	if err := ResetCircuit(layout, scope, "api"); err != nil {
		t.Fatal(err)
	}
	if reset, err := ConsumeCircuitReset(layout, scope, "api"); err != nil || !reset {
		t.Fatalf("reset request was not consumed: %t, %v", reset, err)
	}
	if reset, err := ConsumeCircuitReset(layout, scope, "api"); err != nil || reset {
		t.Fatalf("reset request was consumed twice: %t, %v", reset, err)
	}
}

func TestIncidentRetentionAndRedactedShape(t *testing.T) {
	layout := guardLayout(t)
	scope := testScope()
	old := Incident{
		IncidentSummary: IncidentSummary{ID: "old", OccurredAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano), Subject: "api", Trigger: "cpu"},
		Scope:           scope,
	}
	if err := WriteIncident(layout, old, time.Hour); err != nil {
		t.Fatal(err)
	}
	current := Incident{
		IncidentSummary: IncidentSummary{ID: "new", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano), Subject: "api", Trigger: "cpu"},
		Scope:           scope,
	}
	if err := WriteIncident(layout, current, time.Hour); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(layout.ProjectDir, "resource-guard", "incidents")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("incident retention left %d entries: %v", len(entries), err)
	}
	content, err := os.ReadFile(filepath.Join(directory, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"\"command\":", "\"environment\":", "\"process_output\":", "\"user_arguments\":"} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("incident persisted forbidden field %q: %s", forbidden, content)
		}
	}
}

func TestBaselineRetentionIsBounded(t *testing.T) {
	layout := guardLayout(t)
	scope := testScope()
	started := time.Now().Add(-time.Hour)
	for index := 0; index < 260; index++ {
		baseline := Baseline{
			Scope: scope, Service: "api", ServiceIdentity: fmt.Sprintf("identity-%03d", index),
			UpdatedAt: started.Add(time.Duration(index) * time.Second).UTC().Format(time.RFC3339Nano),
		}
		if err := WriteBaseline(layout, baseline); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(layout.ProjectDir, "resource-guard", "baselines"))
	if err != nil || len(entries) != 256 {
		t.Fatalf("baseline retention left %d entries: %v", len(entries), err)
	}
	if _, exists, err := ReadBaseline(layout, scope, "api", "identity-000"); err != nil || exists {
		t.Fatalf("oldest baseline was retained: exists=%t err=%v", exists, err)
	}
	if _, exists, err := ReadBaseline(layout, scope, "api", "identity-259"); err != nil || !exists {
		t.Fatalf("newest baseline was pruned: exists=%t err=%v", exists, err)
	}
}

func guardLayout(t *testing.T) state.Layout {
	t.Helper()
	layout, err := state.NewLayout("example-k7m4q2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return layout
}

func testScope() AuthorityScope {
	return AuthorityScope{
		ProjectID: "example-k7m4q2", GenerationID: "0123456789abcdefabcd",
		EffectiveManifestSHA256: "manifest", RuntimePID: 42, RuntimeProcessIdentity: "runtime",
		RuntimeCommandSHA256: "command", SocketPath: "runtime.sock", SocketOwnerUID: uint32(os.Getuid()),
		SocketDevice: 1, SocketInode: 2, ProcessComposeConfigHash: "config",
	}
}
