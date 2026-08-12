//go:build darwin || linux

package supervisor

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/rungrid/internal/state"
)

func TestStaticScopeUsesGeneratedGenerationNotSourceManifest(t *testing.T) {
	layout, runtimeState, sourceManifest := liveScopeFixture(t)
	if !StaticScopeMatches(layout, runtimeState) {
		t.Fatal("fresh runtime scope did not match")
	}
	if scope := AuthorityScope(layout, runtimeState); scope.SocketPath != runtimeState.Socket {
		t.Fatalf("authority scope did not retain the exact socket path: %#v", scope)
	}
	if err := os.WriteFile(sourceManifest, []byte("edited source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !StaticScopeMatches(layout, runtimeState) {
		t.Fatal("editing a source manifest changed active generation authority")
	}
	if err := os.WriteFile(runtimeState.Configuration, []byte("tampered config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if StaticScopeMatches(layout, runtimeState) {
		t.Fatal("tampered generated Process Compose configuration retained authority")
	}
	if err := os.WriteFile(runtimeState.Configuration, []byte("generated config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout.ProjectDir, "generations", runtimeState.GenerationID, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("tampered generated manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if StaticScopeMatches(layout, runtimeState) {
		t.Fatal("tampered generated effective manifest retained authority")
	}
}

func liveScopeFixture(t *testing.T) (state.Layout, Runtime, string) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "rg-scope-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	layout, err := state.NewLayout("example-k7m4q2", root)
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	generationID := "0123456789abcdefabcd"
	generation := filepath.Join(layout.ProjectDir, "generations", generationID)
	if err := os.MkdirAll(generation, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestContent := []byte("generated manifest\n")
	configurationContent := []byte("generated config\n")
	manifestPath := filepath.Join(generation, "manifest.yaml")
	configurationPath := filepath.Join(generation, "process-compose.yaml")
	if err := os.WriteFile(manifestPath, manifestContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configurationPath, configurationContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteFileAtomic(layout.ProjectDir, "current", []byte(generationID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(layout.ProjectDir, "runtime.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	device, inode, uid, err := inspectSocket(socket)
	if err != nil {
		t.Fatal(err)
	}
	runtimeState := Runtime{
		APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID, GenerationID: generationID,
		EffectiveManifestSHA256: state.Hash(manifestContent), PID: os.Getpid(),
		ProcessIdentity: "fixture", ProcessCommand: "process-compose fixture", Socket: socket,
		SocketDevice: device, SocketInode: inode, SocketOwnerUID: uid,
		Configuration: configurationPath, ConfigurationHash: state.Hash(configurationContent),
	}
	if err := Write(layout, runtimeState); err != nil {
		t.Fatal(err)
	}
	sourceManifest := filepath.Join(root, ".rungrid.yaml")
	if err := os.WriteFile(sourceManifest, []byte("source manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return layout, runtimeState, sourceManifest
}
