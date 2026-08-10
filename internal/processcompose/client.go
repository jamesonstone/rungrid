package processcompose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/subprocess"
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
	return command
}

func (c Client) Run(ctx context.Context, arguments ...string) ([]byte, error) {
	command := c.command(ctx, arguments...)
	result, err := subprocess.Run(command)
	if err != nil {
		return nil, errs.Wrap(errs.ExitFailure, "RG303", fmt.Sprintf("Process Compose %s failed", strings.Join(arguments, " ")), redactedCommandError(err, append(result.Stdout, result.Stderr...)))
	}
	return result.Stdout, nil
}

func (c Client) Ping(ctx context.Context) error {
	_, err := c.Run(ctx, "project", "state")
	return err
}

func (c Client) Start(ctx context.Context, service string) error {
	_, err := c.Run(ctx, "process", "start", service)
	return err
}

func (c Client) Stop(ctx context.Context, service string) error {
	_, err := c.Run(ctx, "process", "stop", service)
	return err
}

func (c Client) Down(ctx context.Context) error {
	_, err := c.Run(ctx, "down")
	return err
}

func (c Client) List(ctx context.Context) ([]ProcessState, json.RawMessage, error) {
	output, err := c.Run(ctx, "list", "--output", "json")
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
	output, err := c.Run(ctx, "process", "get", service, "--output", "json")
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
			Health:   stringField(object, "health", "health_status"),
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

func redactedCommandError(err error, output []byte) error {
	if len(bytes.TrimSpace(output)) == 0 {
		return err
	}
	return fmt.Errorf("%w (subprocess output redacted)", err)
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
