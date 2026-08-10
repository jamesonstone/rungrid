---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "complete"
feature:
  id: "repository-reconcile"
  slug: "repository-reconcile"
  dir: "repository-reconcile"
relationships:
  - type: builds_on
    target: repository-maintenance
references:
  - id: maintenance-spec
    name: Manifest-scoped repository maintenance
    type: documentation
    target: docs/specs/repository-maintenance/SPEC.md
    relation: builds_on
    read_policy: must
    used_for: native synchronization, pruning, and lifecycle safety
    status: active
  - id: worktree-policy
    name: Git worktree policy
    type: documentation
    target: docs/references/worktrees.md
    relation: constrains
    read_policy: must
    used_for: primary-checkout and linked-worktree mutation safety
    status: active
  - id: github-issue
    name: Filesystem repository reconciliation issue
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/23
    relation: tracks
    read_policy: must
    used_for: delivery ownership
    status: active
  - id: copilot-cli
    name: GitHub Copilot CLI command reference
    type: documentation
    target: https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference
    relation: constrains
    read_policy: must
    used_for: unattended Copilot adapter arguments
    status: active
  - id: claude-cli
    name: Claude Code CLI reference
    type: documentation
    target: https://code.claude.com/docs/en/cli-usage
    relation: constrains
    read_policy: must
    used_for: print and permission adapter arguments
    status: active
  - id: warp-cli
    name: Warp Oz CLI reference
    type: documentation
    target: https://docs.warp.dev/reference/cli
    relation: constrains
    read_policy: must
    used_for: local agent adapter arguments
    status: active
skills: []
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Add an explicitly invoked filesystem-oriented reconciliation command that can
safely synchronize a repository or recursively discovered repository tree
without changing Rungrid's existing manifest-scoped `sync` contract or
disrupting active root-checkout work.

## CONTEXT

- `rungrid sync` deliberately treats the portable manifest as its complete
  repository inventory and coordinates services declared by that workspace.
- Operators also maintain parent directories containing unrelated repositories
  and need one conservative command that deduplicates physical clones, updates
  their remote-default branches, and reclaims independently proven merged
  linked worktrees.
- Primary checkouts are expected to remain on the remote default, but an agent
  may occasionally leave issue work there. Reconciliation must distinguish
  recent or process-owned work from abandoned work before changing that
  checkout.
- Filesystem reconciliation is an explicit exception to Rungrid's normal
  manifest-only maintenance and no-stash/no-feature-checkout rules. The
  exception must be narrow, observable, and independently revalidated.
- An optional coding-agent backend is high trust. It may orchestrate only the
  same native reconcile command and may not become an alternate Git mutation
  implementation.

## REQUIREMENTS

- Add `rungrid reconcile [path]`; default the path to the current directory.
- Preserve `rungrid sync`, `rungrid worktrees prune`, and `rungrid up --sync`
  behavior and interfaces unchanged.
- If the target is inside a Git worktree, process only that physical Git common
  directory. Otherwise scan recursively without following directory symlinks,
  deduplicate by physical common directory, and skip submodules unless
  `--include-submodules` is supplied.
- Use only `origin`; discover its live symbolic HEAD and never guess `main` or
  use manifest remote/default overrides.
- Apply by default. `--dry-run` may read filesystem, Git, GitHub, remote, and
  process state but must not fetch, write refs or state, stage, stash, commit,
  switch branches, pause services, remove worktrees, or prune metadata.
- Emit a stable human report and `rungrid/output/v1` JSON kind
  `RepositoryReconcileReport`. Continue across independent repositories and
  return Rungrid's typed partial result when any required synchronization or
  safety operation fails.
- Reuse Rungrid's native expected-OID fast-forward and worktree-prune proof.
  Do not invoke KP, `git wt`, reset, clean, force operations, recursive delete,
  remote deletion, or force branch deletion.
- Protect running work. For a verified active Rungrid workspace, use its
  maintenance lock and coordinator. Outside that ownership boundary, any
  process whose cwd is in a checkout or which holds a dirty path open blocks
  the dependent mutation.
- Determine primary activity from the newest HEAD commit, primary HEAD reflog,
  dirty-path mtime, process cwd, and dirty-path open handles. Treat inspection
  failure as active when inactivity is required.
- Leave a recent non-default primary branch untouched while still updating the
  local default ref when it is safe and unoccupied.
- A clean non-default primary may switch immediately only when exactly one
  same-repository PR into the discovered default is merged and its head OID
  matches HEAD. Otherwise require 72 hours of inactivity.
- A dirty non-default primary always requires 72 hours of inactivity and may
  be recovered only on exact `GH-<number>`, with an exact same-repository issue,
  no staged changes, human Git identity, and a safe explicit path inventory.
  Refuse ignored files, environment or credential material, unsafe symlinks,
  submodules, and ambiguous status records.
- Stage only exact recovered paths, run
  `gitleaks git --staged --redact --no-banner`, and commit
  `wip(GH-N): :construction: work in progress: <issue title>`. On scan or
  commit failure, restore the previously empty index by unstaging only those
  exact paths and verify it is empty.
- A stale dirty checked-out default that is strictly behind may stash tracked
  and nonignored untracked work under a unique reconciliation label. Record
  the exact stash OID, fast-forward, and never pop or drop it automatically.
- Native cleanup applies without a second confirmation but retains the stricter
  existing proof: canonical registered linked lane, clean except exact verified
  `.env`/`.envrc` symlinks, no unpushed commits, no process cwd, exactly one
  merged same-repository PR with matching head OID, and deleted remote head.
- Support `--agent[=copilot|select|claude|warp|codex]`. Agent mode is mutually
  exclusive with `--dry-run` and `--json`, validates the provider executable,
  streams its output, propagates its exit status, and prompts it to use the
  native JSON preview and apply command as the sole mutation primitive.
- `--agent=select` uses `fzf` when available and a numbered terminal menu
  otherwise; noninteractive selection fails before provider launch.
- Keep every affected handwritten source and test file at or below 300
  physical lines.

### Non-goals

- Changing or replacing manifest-scoped repository maintenance.
- Rebasing, merging into, resetting, publishing, or deleting feature branches.
- Inferring that age alone makes dirty non-`GH-N` work safe to commit.
- Coordinating unverified runtimes or stopping processes Rungrid does not own.
- Sharing an implementation package or runtime dependency with KP.

## ACCEPTED PLAN

1. Add a filesystem inventory that emits existing native maintenance repository
   values keyed by physical Git common directory.
2. Extract reusable native synchronization and prune entry points that accept
   an explicit inventory while keeping manifest commands behavior-identical.
3. Implement activity inspection, stale-primary recovery, exact-path WIP
   commits, retained stashes, and apply-time revalidation as small focused
   reconciliation components.
4. Add a command adapter, human/JSON report, typed partial failures, and
   provider adapters with exact argv and terminal-selection tests.
5. Update public help, CLI documentation, Constitution, worktree guidance, and
   agent instructions for the explicit reconciliation exception.
6. Validate fake gateways, real temporary remotes and worktrees, process
   blocking, strict dry-run snapshots, complete project gates, source-size
   compliance, and the local headless lifecycle suite.

## DECISIONS

- The separate verb is `reconcile`; `sync` retains its portable-manifest
  meaning.
- Filesystem inventory is converted to Rungrid's native maintenance repository
  type so synchronization and pruning retain one proof implementation.
- `origin` is fixed for filesystem reconciliation because there is no manifest
  authority from which to obtain a repository-local override.
- Recent primary issue work is a normal preservation result when the default
  ref can still be synchronized; it is not command failure.
- The 72-hour threshold is fixed in v1 rather than user-configurable so one
  deterministic safety policy is testable and auditable.
- Explicit `--agent` authorizes the provider's unattended mode, but the prompt
  confines mutation to the native reconciliation command.

## DISCOVERIES

- The existing expected-OID synchronization and worktree-prune engine accepted
  a small explicit-inventory entry point, so manifest `sync`, `up --sync`, and
  confirmed `worktrees prune` did not need interface or behavior changes.
- A paused tab session intentionally remains a live cwd process. Reconciliation
  therefore needs positive PID ownership evidence from the verified lifecycle
  coordinator; removing all process checks after pausing would admit unrelated
  shells and coding agents.
- Primary activity must remain decomposed in machine output. A single newest
  timestamp is useful for the 72-hour gate but insufficient to explain whether
  commit, reflog, filesystem, cwd, or open-file evidence caused preservation.
- Current installed help and the official Copilot, Claude Code, and Warp
  references support the accepted unattended argument vectors. Codex's local
  `exec --help` supports the accepted `-C`, repository-check, and permission
  bypass flags.
- A linked-worktree target is represented as a protected declared path while
  the physical inventory continues to identify the clone's primary checkout.
  This processes one common directory without making the requested lane a
  cleanup candidate.

## VALIDATION

- Focused maintenance, reconciliation, agent-adapter, lifecycle, and command
  tests passed. Real temporary remotes cover current/behind/ahead defaults,
  recent and stale roots, merged-PR switching, exact-path rename recovery,
  staged-work refusal, secret-scan rollback, retained stash OIDs, recursive
  partial completion, common-directory deduplication, symlink exclusion, and
  opt-in submodules.
- Live child-process tests proved that a process cwd beneath the primary and an
  open handle to a dirty path block root mutation. Coordinator tests proved
  verified owned processes pause/resume successfully and that an unattributed
  process prevents any service pause.
- The strict dry-run test snapshots refs, index tree, stash ref, worktree
  registrations, status, and tracked filesystem content. The live command
  `go run . reconcile . --dry-run --json` emitted the versioned report, made no
  mutation, and truthfully returned partial status for this clone's ahead local
  default and one uninspectable historical lane.
- `make check` passed formatting, vet, all unit/integration tests, the complete
  race suite, contract sanitization, dependency licenses, local compilation,
  and CGO-free Darwin/Linux amd64/arm64 cross-builds.
- `make lint` reported zero issues. `make vuln` found no called
  vulnerabilities. `tests/end-to-end/local/run.sh` passed with bounded evidence
  at `tmp/2026-08-09/rungrid-headless-e2e/1`.
- `make release-snapshot` validated GoReleaser, reran all Go tests, and built
  four archives. Syft and Cosign were unavailable, so local SBOM generation and
  signing were not claimed.
- The complete affected handwritten source/test audit found no file above 300
  physical lines.
- `kit check --project` remains blocked by the same eight pre-existing
  Kit-instruction drift findings on both the stacked base checkout and this
  lane; this feature did not modify or claim those unrelated baseline gaps.

## OUTCOME

Rungrid now exposes `reconcile [path]` as a filesystem-scoped complement to the
unchanged manifest `sync`. Its native backend inventories physical clones,
proves the live `origin` default, fast-forwards with expected OIDs, reports
decomposed activity evidence, conservatively recovers only eligible primary
checkouts, and applies existing strict linked-lane cleanup proof.

Verified active workspaces use the project lock and lifecycle coordinator.
Owned process identities are explicit, unattributed processes fail closed, and
resume failures remain visible after successful Git mutations. JSON uses
`RepositoryReconcileReport` under `rungrid/output/v1`; human mode summarizes
the same decisions while independent repository failures produce exit 6.

Agent mode provides exact Copilot, Claude, Warp, and Codex adapters with output
and exit passthrough. Its prompt requires native JSON preview, native-only
mutation, repository-rule compliance, conservative preservation, and a final
result summary.

## REPOSITORY MEMORY

Decision: created

Rationale: The manifest exception, root-recovery authority, unattended-agent
boundary, and relationship to existing lifecycle ownership are consequential
product decisions that code and tests alone cannot preserve.

Artifacts:

- `docs/specs/repository-reconcile/SPEC.md`
- `docs/CONSTITUTION.md`
- `CLI_SPEC.md`
- `docs/references/worktrees.md`
- `docs/agents/TOOLING.md`
- `docs/PROJECT_PROGRESS_SUMMARY.md`
