//go:build darwin || linux

package lifecycle

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func TestStartMaintenanceJobUsesAuthorizedProcessComposeProcess(t *testing.T) {
	stateRoot, err := os.MkdirTemp("/tmp", "rg-maint-job-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateRoot) })
	layout, err := state.NewLayout("example-k7m4q2", stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(layout.ProjectDir, "runtime.sock"))
	if err != nil {
		t.Fatal(err)
	}
	requested := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requested <- request.Method + " " + request.URL.Path
		_, _ = w.Write([]byte(`{"name":"rungrid-maintenance-sync"}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	active := Active{Layout: layout, Runtime: supervisor.Runtime{
		ProjectID: "example-k7m4q2", GenerationID: "0123456789abcdefabcd",
		WorkspaceRoot: t.TempDir(),
	}}
	workerDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for {
			request, claim, claimErr := maintenance.ClaimRequest(layout, "0123456789abcdefabcd", maintenance.OperationSync)
			if claimErr == nil {
				defer maintenance.CleanupClaim(claim)
				workerDone <- maintenance.WriteJobResult(layout, request, maintenance.SyncReport{Operation: maintenance.OperationSync}, nil)
				return
			}
			select {
			case <-ctx.Done():
				workerDone <- ctx.Err()
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	result, err := StartMaintenanceJob(context.Background(), active, maintenance.OperationSync, []string{"api"})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-workerDone; err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("unexpected result: %#v", result)
	}
	if operation := <-requested; operation != "POST /process/start/"+maintenance.SyncProcessName {
		t.Fatalf("unexpected Process Compose request: %q", operation)
	}
}

func TestMaintenanceWorktreeContainsDeclaredRepositorySubdirectory(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "workspace", "api")
	if !withinMaintenanceWorktree(root, filepath.Join(root, "services", "server")) {
		t.Fatal("declared repository subdirectory was not mapped to its Git worktree")
	}
	if withinMaintenanceWorktree(root, filepath.Join(string(filepath.Separator), "workspace", "web")) {
		t.Fatal("sibling repository was mapped to the default worktree")
	}
}
