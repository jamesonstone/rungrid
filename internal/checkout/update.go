package checkout

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

type UpdateOptions struct {
	Services []string
	Sync     bool
	DryRun   bool
}

func Update(ctx context.Context, layout state.Layout, loaded *manifest.Loaded, options UpdateOptions, runner maintenance.Runner, coordinator maintenance.Coordinator) (UpdateReport, error) {
	if runner == nil {
		runner = maintenance.CommandRunner{}
	}
	if coordinator == nil {
		coordinator = maintenance.NoopCoordinator{}
	}
	report := UpdateReport{Operation: "worktrees-update", DryRun: options.DryRun, Sync: options.Sync, StartedAt: timestamp()}
	services, err := selectedServices(&loaded.Manifest, options.Services)
	if err != nil {
		return report, err
	}
	seen := map[string]bool{}
	for _, service := range services {
		roots, resolveErr := Resolve(ctx, layout, loaded, service, runner)
		if resolveErr != nil {
			report.Failures = append(report.Failures, Failure{Service: service.Name, Operation: "resolve", Error: resolveErr.Error()})
			continue
		}
		if !roots.Selected || roots.Primary {
			continue
		}
		if seen[roots.RepositoryRoot] {
			continue
		}
		seen[roots.RepositoryRoot] = true
		target, failures := updateWorktree(ctx, runner, coordinator, service.Name, roots, options.DryRun)
		report.Targets = append(report.Targets, target)
		report.Failures = append(report.Failures, failures...)
	}
	report.FinishedAt = timestamp()
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1716", fmt.Sprintf("worktree update completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

func selectedServices(m *manifest.Manifest, names []string) ([]*manifest.Service, error) {
	if len(names) == 0 {
		result := make([]*manifest.Service, 0, len(m.Services))
		for index := range m.Services {
			result = append(result, &m.Services[index])
		}
		return result, nil
	}
	var result []*manifest.Service
	for _, name := range names {
		service, exists := manifest.FindService(m, name)
		if !exists {
			return nil, errs.New(errs.ExitUsage, "RG1715", "unknown service: "+name)
		}
		result = append(result, service)
	}
	return result, nil
}

func updateWorktree(ctx context.Context, runner maintenance.Runner, coordinator maintenance.Coordinator, service string, roots Roots, dryRun bool) (UpdateTarget, []Failure) {
	target := UpdateTarget{Service: service, Path: roots.RepositoryRoot, Branch: roots.Branch, LocalOID: roots.HeadOID, State: "unavailable", Action: "preserved"}
	branch, err := gitText(ctx, runner, roots.RepositoryRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch == "" {
		target.State, target.Detail = "detached", "feature worktree is detached"
		return target, nil
	}
	target.Branch = branch
	if !dryRun {
		if _, err := git(ctx, runner, roots.RepositoryRoot, "fetch", "--prune", "origin"); err != nil {
			return target, []Failure{{Service: service, Operation: "fetch", Path: roots.RepositoryRoot, Error: err.Error()}}
		}
	}
	remoteOID, err := remoteBranchOID(ctx, runner, roots.RepositoryRoot, branch, dryRun)
	if err != nil {
		target.State, target.Detail = "untracked", "feature branch has no origin/"+branch+" ref"
		return target, nil
	}
	localOID, err := gitText(ctx, runner, roots.RepositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return target, []Failure{{Service: service, Operation: "inspect-head", Path: roots.RepositoryRoot, Error: err.Error()}}
	}
	target.LocalOID, target.RemoteOID = localOID, remoteOID
	if localOID == remoteOID {
		target.State, target.Action = "current", "none"
		return target, nil
	}
	if _, err := git(ctx, runner, roots.RepositoryRoot, "cat-file", "-e", remoteOID+"^{commit}"); err != nil {
		if dryRun {
			target.State, target.Action, target.Detail = "behind", "would-fast-forward", "remote feature commit is not available locally"
			return target, nil
		}
		target.Detail = "remote feature commit is not available locally"
		return target, nil
	}
	behind, behindErr := isAncestor(ctx, runner, roots.RepositoryRoot, localOID, remoteOID)
	ahead, aheadErr := isAncestor(ctx, runner, roots.RepositoryRoot, remoteOID, localOID)
	if behindErr != nil || aheadErr != nil {
		if dryRun && localOID != remoteOID {
			target.State, target.Action, target.Detail = "behind", "would-fast-forward", "remote feature commit is not available locally"
			return target, nil
		}
		target.Detail = "feature-branch ancestry could not be classified"
		return target, nil
	}
	switch {
	case behind && !ahead:
		target.State = "behind"
	case ahead && !behind:
		target.State, target.Detail = "ahead", "feature branch is ahead of origin"
		return target, nil
	default:
		target.State, target.Detail = "diverged", "feature branch has diverged from origin"
		return target, nil
	}
	clean, cleanErr := cleanWorktree(ctx, runner, roots.RepositoryRoot)
	if cleanErr != nil {
		return target, []Failure{{Service: service, Operation: "inspect-worktree", Path: roots.RepositoryRoot, Error: cleanErr.Error()}}
	}
	if !clean {
		target.State, target.Detail = "dirty", "selected worktree has local changes"
		return target, nil
	}
	if dryRun {
		target.Action = "would-fast-forward"
		return target, nil
	}
	services, resume, pauseErr := coordinator.Pause(ctx, roots.RepositoryRoot)
	if pauseErr != nil {
		target.State, target.Detail = "blocked", "running services could not be paused"
		return target, []Failure{{Service: service, Operation: "pause-services", Path: roots.RepositoryRoot, Error: pauseErr.Error()}}
	}
	_ = services
	if _, err := git(ctx, runner, roots.RepositoryRoot, "merge", "--ff-only", remoteOID); err != nil {
		_ = resume(ctx)
		return target, []Failure{{Service: service, Operation: "fast-forward", Path: roots.RepositoryRoot, Error: err.Error()}}
	}
	updatedOID, err := gitText(ctx, runner, roots.RepositoryRoot, "rev-parse", "HEAD")
	if err != nil || updatedOID != remoteOID {
		_ = resume(ctx)
		return target, []Failure{{Service: service, Operation: "fast-forward", Path: roots.RepositoryRoot, Error: "feature branch did not reach expected remote OID"}}
	}
	if resumeErr := resume(ctx); resumeErr != nil {
		target.Action, target.State, target.LocalOID = "fast-forwarded", "current", remoteOID
		target.Detail = "feature branch advanced but running services could not be resumed: " + resumeErr.Error()
		return target, []Failure{{Service: service, Operation: "resume-services", Path: roots.RepositoryRoot, Error: resumeErr.Error()}}
	}
	target.Action, target.State, target.LocalOID = "fast-forwarded", "current", remoteOID
	return target, nil
}

func remoteBranchOID(ctx context.Context, runner maintenance.Runner, directory, branch string, dryRun bool) (string, error) {
	if dryRun {
		content, err := gitText(ctx, runner, directory, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
		if err != nil {
			return "", err
		}
		fields := strings.Fields(content)
		if len(fields) < 2 || fields[1] != "refs/heads/"+branch {
			return "", fmt.Errorf("feature branch is absent on origin")
		}
		return fields[0], nil
	}
	return gitText(ctx, runner, directory, "rev-parse", "--verify", "origin/"+branch)
}

func isAncestor(ctx context.Context, runner maintenance.Runner, directory, ancestor, descendant string) (bool, error) {
	_, err := git(ctx, runner, directory, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}
	if strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}
	return false, err
}

func cleanWorktree(ctx context.Context, runner maintenance.Runner, directory string) (bool, error) {
	content, err := git(ctx, runner, directory, "status", "--porcelain", "--untracked-files=all")
	return len(strings.TrimSpace(string(content))) == 0, err
}
