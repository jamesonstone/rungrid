package maintenance

import "context"

type ResumeFunc func(context.Context) error

type Coordinator interface {
	AffectedServices(ctx context.Context, worktreePath string) ([]string, error)
	Pause(ctx context.Context, worktreePath string) ([]string, ResumeFunc, error)
}

// ProcessOwner is an optional coordinator capability used by filesystem
// reconciliation to distinguish verified Rungrid-owned processes from other
// processes that make a worktree unsafe to mutate.
type ProcessOwner interface {
	OwnedProcessIDs(ctx context.Context, worktreePath string) ([]string, error)
}

type NoopCoordinator struct{}

func (NoopCoordinator) AffectedServices(context.Context, string) ([]string, error) {
	return nil, nil
}

func (NoopCoordinator) Pause(context.Context, string) ([]string, ResumeFunc, error) {
	return nil, func(context.Context) error { return nil }, nil
}
