package processcompose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
)

const (
	queryTimeout      = 5 * time.Second
	actionTimeout     = 10 * time.Second
	shutdownTimeout   = 30 * time.Second
	maxQueryResponse  = 4 << 20
	maxActionResponse = 64 << 10
)

type Client struct {
	Executable string
	Socket     string
	LogFile    string
	WorkDir    string
}

type ProcessState struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Health   string `json:"health,omitempty"`
	PID      int    `json:"pid,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

func (c Client) command(ctx context.Context, arguments ...string) *exec.Cmd {
	base := []string{"-U", "-u", c.Socket, "-L", c.LogFile}
	command := exec.CommandContext(ctx, c.Executable, append(base, arguments...)...)
	command.Dir = c.WorkDir
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return command
}

func (c Client) Ping(ctx context.Context) error {
	_, err := c.request(ctx, queryTimeout, http.MethodGet, "/live", maxActionResponse, "liveness query")
	return err
}

func (c Client) Start(ctx context.Context, service string) error {
	_, err := c.request(ctx, actionTimeout, http.MethodPost, "/process/start/"+url.PathEscape(service), maxActionResponse, "start process")
	return err
}

func (c Client) Stop(ctx context.Context, service string) error {
	_, err := c.request(ctx, actionTimeout, http.MethodPatch, "/process/stop/"+url.PathEscape(service), maxActionResponse, "stop process")
	return err
}

func (c Client) Down(ctx context.Context) error {
	_, err := c.request(ctx, shutdownTimeout, http.MethodPost, "/project/stop", maxActionResponse, "stop project")
	return err
}

func (c Client) List(ctx context.Context) ([]ProcessState, json.RawMessage, error) {
	output, err := c.request(ctx, queryTimeout, http.MethodGet, "/processes", maxQueryResponse, "list processes")
	if err != nil {
		return nil, nil, err
	}
	states, err := decodeStates(output)
	if err != nil {
		return nil, nil, err
	}
	return states, append(json.RawMessage(nil), bytes.TrimSpace(output)...), nil
}

func (c Client) Get(ctx context.Context, service string) (ProcessState, error) {
	output, err := c.request(ctx, queryTimeout, http.MethodGet, "/process/"+url.PathEscape(service), maxQueryResponse, "get process")
	if err != nil {
		return ProcessState{}, err
	}
	states, err := decodeStates(output)
	if err != nil {
		return ProcessState{}, err
	}
	if len(states) == 0 {
		return ProcessState{}, errs.New(errs.ExitFailure, "RG304", "Process Compose returned no process state")
	}
	return states[0], nil
}

func (c Client) request(
	ctx context.Context,
	timeout time.Duration,
	method, path string,
	limit int64,
	operation string,
) ([]byte, error) {
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	socket := c.Socket
	if !filepath.IsAbs(socket) {
		socket = filepath.Join(c.WorkDir, socket)
	}
	socket, err := socketDialPath(socket)
	if err != nil {
		return nil, errs.Wrap(errs.ExitConflict, "RG303", "prepare Process Compose Unix socket path", err)
	}
	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(requestContext, method, "http://process-compose"+path, nil)
	if err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG303", "create Process Compose "+operation, err)
	}
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost || method == http.MethodPatch {
		request.Header.Set("Content-Type", "application/json")
	}
	if token, tokenErr := apiToken(); tokenErr != nil {
		return nil, errs.Wrap(errs.ExitDependency, "RG303", "read Process Compose API token", tokenErr)
	} else if token != "" {
		request.Header.Set("X-PC-Token-Key", token)
	}
	response, err := (&http.Client{
		Transport:     transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}).Do(request)
	if err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG303", "Process Compose "+operation+" failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	content, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG303", "read Process Compose "+operation+" response", err)
	}
	if int64(len(content)) > limit {
		return nil, errs.New(errs.ExitFailure, "RG303", "Process Compose "+operation+" response exceeded its size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, errs.New(errs.ExitFailure, "RG303", fmt.Sprintf("Process Compose %s returned HTTP %d (response redacted)", operation, response.StatusCode))
	}
	return content, nil
}

func apiToken() (string, error) {
	if token := os.Getenv("PC_API_TOKEN"); token != "" {
		return token, nil
	}
	path := os.Getenv("PC_API_TOKEN_PATH")
	if path == "" {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(content) > 64<<10 {
		return "", fmt.Errorf("process Compose API token file exceeds 64 KiB")
	}
	return strings.TrimSpace(string(content)), nil
}

func (c Client) LogsCommand(ctx context.Context, services []string, follow bool, tail int, raw bool, stdin io.Reader, stdout, stderr io.Writer) *exec.Cmd {
	arguments := []string{"process", "logs", strings.Join(services, ",")}
	if follow {
		arguments = append(arguments, "--follow")
	}
	if tail >= 0 {
		arguments = append(arguments, "--tail", fmt.Sprintf("%d", tail))
	}
	if raw {
		arguments = append(arguments, "--raw-log")
	}
	command := c.command(ctx, arguments...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command
}

func (c Client) AttachCommand(ctx context.Context, readOnly bool, stdin io.Reader, stdout, stderr io.Writer) *exec.Cmd {
	arguments := []string{}
	if readOnly {
		arguments = append(arguments, "--read-only")
	}
	arguments = append(arguments, "attach")
	command := c.command(ctx, arguments...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command
}

func decodeStates(content []byte) ([]ProcessState, error) {
	var value any
	if err := json.Unmarshal(content, &value); err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG305", "decode Process Compose state", err)
	}
	var objects []map[string]any
	collectStateObjects(value, &objects)
	result := make([]ProcessState, 0, len(objects))
	for _, object := range objects {
		name := stringField(object, "name", "process", "process_name")
		status := stringField(object, "status", "state")
		if name == "" && status == "" {
			continue
		}
		result = append(result, ProcessState{
			Name:     name,
			Status:   status,
			Health:   stringField(object, "health", "health_status", "is_ready"),
			PID:      intField(object, "pid"),
			ExitCode: intField(object, "exit_code", "exitCode"),
		})
	}
	return result, nil
}

func collectStateObjects(value any, target *[]map[string]any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectStateObjects(item, target)
		}
	case map[string]any:
		if stringField(typed, "status", "state") != "" {
			*target = append(*target, typed)
			return
		}
		for _, item := range typed {
			collectStateObjects(item, target)
		}
	}
}

func stringField(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if item, ok := value[key].(string); ok {
			return item
		}
	}
	return ""
}

func intField(value map[string]any, keys ...string) int {
	for _, key := range keys {
		switch item := value[key].(type) {
		case float64:
			return int(item)
		case json.Number:
			parsed, _ := item.Int64()
			return int(parsed)
		}
	}
	return 0
}

func InternalLog() string { return os.DevNull }

func EnvironmentWithRuntime(base []string, executable, stateRoot, workspaceRoot, generationID string) []string {
	result := make([]string, 0, len(base)+4)
	for _, value := range base {
		if strings.HasPrefix(value, "RUNGRID_EXECUTABLE=") ||
			strings.HasPrefix(value, "RUNGRID_STATE_DIR=") ||
			strings.HasPrefix(value, "RUNGRID_WORKSPACE_ROOT=") ||
			strings.HasPrefix(value, "RUNGRID_GENERATION_ID=") {
			continue
		}
		result = append(result, value)
	}
	return append(result,
		"RUNGRID_EXECUTABLE="+executable,
		"RUNGRID_STATE_DIR="+stateRoot,
		"RUNGRID_WORKSPACE_ROOT="+workspaceRoot,
		"RUNGRID_GENERATION_ID="+generationID,
	)
}

func ExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
