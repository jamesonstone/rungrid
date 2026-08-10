//go:build darwin || linux

package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

func Down(ctx context.Context, active Active) error {
	return DownProject(ctx, active.Layout)
}

func DownProject(ctx context.Context, layout state.Layout) error {
	if _, err := os.Lstat(layout.ProjectDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return errs.Wrap(errs.ExitConflict, "RG1131", "inspect project state before shutdown", err)
	}
	lock, err := workspace.Acquire(ctx, layout)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	return downProjectLocked(ctx, layout)
}

func downProjectLocked(ctx context.Context, layout state.Layout) error {
	journal, exists, err := workspace.ReadJournalIfPresent(layout)
	if err != nil {
		return err
	}
	if !exists {
		journal, exists, err = journalFromRuntime(layout)
		if err != nil || !exists {
			return err
		}
		if err := workspace.WriteJournal(layout, journal); err != nil {
			return err
		}
	}
	if journal.State == workspace.StateInactive && !journal.TeardownRequired {
		if _, runtimeErr := supervisor.Read(layout); errors.Is(runtimeErr, os.ErrNotExist) {
			return nil
		}
	}
	cleanupContext, cancel := durableCleanupContext(ctx)
	defer cancel()
	return cleanupJournalLocked(cleanupContext, layout, &journal)
}

func cleanupJournalLocked(ctx context.Context, layout state.Layout, journal *workspace.Journal) error {
	configuration, err := journalManifest(layout, *journal)
	if err != nil {
		return recordCleanupFailure(layout, journal, err)
	}
	journal.State = workspace.StateStopping
	journal.CleanupFailure = ""
	if err := workspace.WriteJournal(layout, *journal); err != nil {
		return err
	}

	var failures []string
	runtimeState, runtimeExists, err := verifiedJournalRuntime(ctx, layout, journal)
	if err != nil {
		return recordCleanupFailure(layout, journal, err)
	}
	if runtimeExists {
		runtimeContext, cancel := context.WithTimeout(ctx, configuration.Runtime.ShutdownTimeout.Duration)
		err := stopRuntime(runtimeContext, Active{Layout: layout, Runtime: runtimeState, Manifest: configuration})
		cancel()
		if err != nil {
			failures = append(failures, err.Error())
		} else {
			journal.Runtime = nil
		}
	}
	if journal.TeardownRequired {
		if err := runAfterDown(ctx, layout, journal, configuration); err != nil {
			failures = append(failures, err.Error())
		} else {
			journal.TeardownRequired = false
		}
	}
	if len(failures) == 0 {
		journal.State = workspace.StateInactive
		journal.CleanupFailure = ""
	} else {
		journal.State = workspace.StateCleanup
		journal.CleanupFailure = strings.Join(failures, "; ")
	}
	if err := workspace.WriteJournal(layout, *journal); err != nil {
		return err
	}
	if len(failures) > 0 {
		return errs.New(errs.ExitPartial, "RG1132", "workspace cleanup remains required:\n  - "+strings.Join(failures, "\n  - "))
	}
	return nil
}

func journalFromRuntime(layout state.Layout) (workspace.Journal, bool, error) {
	runtimeState, err := supervisor.Read(layout)
	if errors.Is(err, os.ErrNotExist) {
		return workspace.Journal{}, false, nil
	}
	if err != nil {
		return workspace.Journal{}, false, err
	}
	configuration, content, err := runtimeManifest(layout, runtimeState)
	if err != nil {
		return workspace.Journal{}, false, err
	}
	journal := workspace.NewJournal(
		layout.ProjectID,
		runtimeState.GenerationID,
		state.Hash(content),
		workspace.LifecycleDigest(configuration.Lifecycle),
		runtimeState.WorkspaceRoot,
		len(configuration.Lifecycle.BeforeUp) > 0 || len(configuration.Lifecycle.AfterDown) > 0,
	)
	journal.State = workspace.StateActive
	attachRuntime(&journal, runtimeState)
	return journal, true, nil
}

func recordCleanupFailure(layout state.Layout, journal *workspace.Journal, failure error) error {
	journal.State = workspace.StateCleanup
	journal.CleanupFailure = failure.Error()
	if err := workspace.WriteJournal(layout, *journal); err != nil {
		return err
	}
	return failure
}

func durableCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}
	return context.WithCancel(context.Background())
}

func runtimeManifest(layout state.Layout, runtimeState supervisor.Runtime) (*manifest.Manifest, []byte, error) {
	filename := filepath.Join(layout.ProjectDir, "generations", runtimeState.GenerationID, "manifest.yaml")
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, errs.Wrap(errs.ExitConflict, "RG1133", "read recorded generation manifest", err)
	}
	configuration, err := manifest.LoadGenerated(filename, runtimeState.WorkspaceRoot)
	return configuration, content, err
}

func journalManifest(layout state.Layout, journal workspace.Journal) (*manifest.Manifest, error) {
	filename := filepath.Join(layout.ProjectDir, "generations", journal.GenerationID, "manifest.yaml")
	content, err := os.ReadFile(filename)
	if err != nil || state.Hash(content) != journal.ManifestSHA256 {
		return nil, errs.New(errs.ExitConflict, "RG1134", "recorded lifecycle generation manifest is missing or modified")
	}
	configuration, err := manifest.LoadGenerated(filename, journal.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if workspace.LifecycleDigest(configuration.Lifecycle) != journal.LifecycleSHA256 {
		return nil, errs.New(errs.ExitConflict, "RG1135", "recorded lifecycle configuration hash does not match")
	}
	return configuration, nil
}

func verifiedJournalRuntime(
	ctx context.Context,
	layout state.Layout,
	journal *workspace.Journal,
) (supervisor.Runtime, bool, error) {
	runtimeState, err := supervisor.Read(layout)
	if errors.Is(err, os.ErrNotExist) {
		if journal.Runtime != nil && procidentity.Matches(journal.Runtime.PID, journal.Runtime.ProcessIdentity) {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1136", "recorded runtime process still exists without its runtime record")
		}
		if _, socketErr := os.Lstat(filepath.Join(layout.ProjectDir, "runtime.sock")); socketErr == nil {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1137", "unverified runtime socket exists without its runtime record")
		} else if !os.IsNotExist(socketErr) {
			return supervisor.Runtime{}, false, socketErr
		}
		return supervisor.Runtime{}, false, nil
	}
	if err != nil {
		return supervisor.Runtime{}, false, err
	}
	if runtimeState.GenerationID != journal.GenerationID {
		return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1138", "runtime generation does not match lifecycle journal")
	}
	if journal.Runtime != nil {
		if !runtimeIdentityMatches(*journal.Runtime, runtimeState) {
			return supervisor.Runtime{}, false, errs.New(errs.ExitConflict, "RG1139", "runtime identity does not match lifecycle journal")
		}
		retired, err := supervisor.RetireStaleRuntime(layout, runtimeState)
		if err != nil {
			return supervisor.Runtime{}, false, err
		}
		if retired {
			journal.Runtime = nil
			return supervisor.Runtime{}, false, nil
		}
	}
	if err := supervisor.Verify(ctx, layout, runtimeState); err != nil {
		return supervisor.Runtime{}, false, err
	}
	return runtimeState, true, nil
}

func runtimeIdentityMatches(record workspace.RuntimeIdentity, runtimeState supervisor.Runtime) bool {
	return record.PID == runtimeState.PID && record.ProcessIdentity == runtimeState.ProcessIdentity &&
		record.Socket == runtimeState.Socket && record.SocketDevice == runtimeState.SocketDevice &&
		record.SocketInode == runtimeState.SocketInode
}
