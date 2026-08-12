//go:build darwin || linux

package processcompose

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientUsesUnixSocketHTTPRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /live", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	})
	mux.HandleFunc("GET /processes", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"api","status":"Running","is_ready":"Ready","pid":42}]}`))
	})
	mux.HandleFunc("GET /process/api", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"api","status":"Running","is_ready":"Ready","pid":42}`))
	})
	for route, body := range map[string]string{
		"POST /process/start/api": `{"name":"api"}`,
		"PATCH /process/stop/api": `{"name":"api"}`,
		"POST /project/stop":      `{"status":"stopped"}`,
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
	}
	client := serveUnixClient(t, mux)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	states, raw, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Name != "api" || states[0].PID != 42 || states[0].Health != "Ready" {
		t.Fatalf("unexpected states: %#v", states)
	}
	if !strings.Contains(string(raw), `"data"`) {
		t.Fatalf("raw response was not preserved: %s", raw)
	}
	state, err := client.Get(context.Background(), "api")
	if err != nil || state.Status != "Running" {
		t.Fatalf("unexpected get result: %#v, %v", state, err)
	}
	if err := client.Start(context.Background(), "api"); err != nil {
		t.Fatal(err)
	}
	if err := client.Stop(context.Background(), "api"); err != nil {
		t.Fatal(err)
	}
	if err := client.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientBoundsDeadlineResponseAndErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /live", func(w http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	})
	mux.HandleFunc("GET /processes", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", maxQueryResponse+1))
	})
	mux.HandleFunc("GET /process/api", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"secret command material"}`))
	})
	client := serveUnixClient(t, mux)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.Ping(ctx); err == nil {
		t.Fatal("expected liveness deadline failure")
	}
	if _, _, err := client.List(context.Background()); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected bounded response error, got %v", err)
	}
	if _, err := client.Get(context.Background(), "api"); err == nil || strings.Contains(err.Error(), "secret command material") {
		t.Fatalf("expected redacted HTTP error, got %v", err)
	}
}

func TestClientForwardsProcessComposeTokenWithoutExposingIt(t *testing.T) {
	t.Setenv("PC_API_TOKEN", "01234567890123456789-secret")
	client := serveUnixClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-PC-Token-Key") != "01234567890123456789-secret" {
			t.Fatal("Process Compose token was not forwarded")
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"01234567890123456789-secret"}`))
	}))
	err := client.Ping(context.Background())
	if err == nil || strings.Contains(err.Error(), "01234567890123456789-secret") {
		t.Fatalf("expected redacted token error, got %v", err)
	}
}

func serveUnixClient(t *testing.T, handler http.Handler) Client {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "rg-pc-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socket := filepath.Join(directory, "runtime.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
	})
	return Client{Socket: "runtime.sock", WorkDir: directory}
}
