//go:build darwin || linux

package update

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

// LifecycleExecutor runs commands with the lifecycle-hook executor: resolved
// service environment, process-group timeout, and a private redacted log under
// lifecycle-logs/update/.
type LifecycleExecutor struct {
	Layout state.Layout
}

func (e LifecycleExecutor) Execute(ctx context.Context, index int, command Command) (string, error) {
	outcome, err := workspace.Execute(ctx, e.Layout, Operation, command.Root, Operation, index, manifest.LifecycleCommand{
		Name: command.Name, WorkingDirectory: command.WorkingDirectory,
		Timeout: manifest.Duration{Duration: command.Timeout}, Run: manifest.Command{Argv: command.Argv},
		Environment: command.Environment,
	})
	return outcome.Log, err
}
