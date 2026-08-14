//go:build darwin || linux

package processcompose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLongSocketPathUsesExactPrivateAlias(t *testing.T) {
	socket := filepath.Join("/tmp", strings.Repeat("long-project-segment-", 6), "runtime.sock")
	alias, err := socketDialPath(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveSocketAlias(socket) })
	if alias == socket || len(alias) > directUnixSocketPathLimit {
		t.Fatalf("long socket path was not shortened: %q", alias)
	}
	target, err := os.Readlink(alias)
	if err != nil || target != socket {
		t.Fatalf("socket alias target=%q err=%v, want %q", target, err, socket)
	}
	if second, err := socketDialPath(socket); err != nil || second != alias {
		t.Fatalf("socket alias was not deterministic: %q, %v", second, err)
	}
	if err := RemoveSocketAlias(socket); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(alias); !os.IsNotExist(err) {
		t.Fatalf("socket alias remains after cleanup: %v", err)
	}
}
