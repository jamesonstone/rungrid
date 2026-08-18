//go:build darwin || linux

package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

func StartMaintenanceJob(ctx context.Context, active Active, operation string, repositories []string) (maintenance.JobResult, error) {
	request, err := maintenance.NewRequest(active.Layout, active.Runtime.GenerationID, operation, repositories)
	if err != nil {
		return maintenance.JobResult{}, err
	}
	if err := maintenance.WriteRequest(active.Layout, request); err != nil {
		return maintenance.JobResult{}, err
	}
	if err := supervisor.Client(active.Layout, active.Runtime).Start(ctx, maintenance.ProcessName(operation)); err != nil {
		maintenance.CancelRequest(active.Layout, request)
		return maintenance.JobResult{}, err
	}
	waitContext, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	result, err := maintenance.WaitJobResult(waitContext, active.Layout, request)
	if err != nil {
		return result, err
	}
	if !result.Success {
		return result, errs.New(result.ErrorCode, result.Diagnostic, result.Error)
	}
	return result, nil
}

func RunMaintenanceWorker(ctx context.Context, projectID, generationID, operation, stateOverride string, stdout io.Writer) error {
	if stateOverride == "" {
		stateOverride = os.Getenv("RUNGRID_STATE_DIR")
	}
	layout, err := state.NewLayout(projectID, stateOverride)
	if err != nil {
		return err
	}
	request, claimPath, err := maintenance.ClaimRequest(layout, generationID, operation)
	if err != nil {
		return err
	}
	defer maintenance.CleanupClaim(claimPath)
	active, err := LoadActive(ctx, projectID, stateOverride)
	if err != nil {
		_ = maintenance.WriteJobResult(layout, request, nil, err)
		return err
	}
	if active.Runtime.GenerationID != generationID {
		err = errs.New(errs.ExitConflict, "RG1120", "maintenance worker generation is stale")
		_ = maintenance.WriteJobResult(layout, request, nil, err)
		return err
	}
	lock, err := workspace.Acquire(ctx, layout)
	if err != nil {
		_ = maintenance.WriteJobResult(layout, request, nil, err)
		return err
	}
	loaded := &manifest.Loaded{Manifest: *active.Manifest, WorkspaceRoot: active.Runtime.WorkspaceRoot}
	// The worker writes into a Process Compose job log, never a terminal, so
	// its human report is always colorless.
	workerStyle := present.New(false)
	options := maintenance.Options{Repositories: request.Repositories}
	var data any
	var runErr error
	switch operation {
	case maintenance.OperationSync:
		report, operationErr := maintenance.Sync(ctx, loaded, options, nil, NewMaintenanceCoordinator(active))
		data, runErr = report, operationErr
		_ = maintenance.WriteSyncHuman(stdout, workerStyle, report)
	case maintenance.OperationPrune:
		report, operationErr := maintenance.Prune(ctx, loaded, options, nil)
		data, runErr = report, operationErr
		_ = maintenance.WritePruneHuman(stdout, workerStyle, report)
	default:
		runErr = errs.New(errs.ExitUsage, "RG1121", "unknown maintenance operation")
	}
	runErr = errors.Join(runErr, lock.Release())
	resultErr := maintenance.WriteJobResult(layout, request, data, runErr)
	return errors.Join(runErr, resultErr)
}

func DecodeSyncJob(result maintenance.JobResult) (maintenance.SyncReport, error) {
	var report maintenance.SyncReport
	return report, json.Unmarshal(result.Data, &report)
}

func DecodePruneJob(result maintenance.JobResult) (maintenance.PruneReport, error) {
	var report maintenance.PruneReport
	return report, json.Unmarshal(result.Data, &report)
}
