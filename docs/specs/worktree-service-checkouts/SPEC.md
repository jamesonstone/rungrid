---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "delivery"
feature:
  id: "worktree-service-checkouts"
  slug: "worktree-service-checkouts"
  dir: "worktree-service-checkouts"
relationships:
  - type: builds_on
    target: repository-maintenance
  - type: builds_on
    target: rungrid-v1
references:
  - id: cli-contract
    name: Rungrid CLI specification
    type: documentation
    target: CLI_SPEC.md
    relation: constrains
    read_policy: must
    used_for: public command, manifest, output, and lifecycle contracts
    status: active
  - id: worktree-policy
    name: Git worktree policy
    type: documentation
    target: docs/references/worktrees.md
    relation: constrains
    read_policy: must
    used_for: fail-closed worktree inspection
    status: active
  - id: testing-reference
    name: Project testing reference
    type: documentation
    target: docs/references/testing.md
    relation: constrains
    read_policy: must
    used_for: required validation and evidence
    status: active
  - id: github-issue
    name: Per-service worktree selection issue
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/49
    relation: tracks
    read_policy: must
    used_for: delivery ownership
    status: active
  - id: interactive-issue
    name: Interactive worktree command issue
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/51
    relation: tracks
    read_policy: must
    used_for: interactive TTY command ownership
    status: active
skills: []
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Let an operator select an existing Git worktree for one managed service, run
that service from the selected checkout, and fast-forward selected feature
worktrees and/or default branches without creating lanes or rewriting history.

## CONTEXT

- Canonical linked worktrees live outside `workspace.root`, usually under
  `~/worktrees/<owner>/<repository>/<lane>`. Portable repository paths and the
  ignored overlay must stay workspace-relative, so they cannot name those
  checkouts.
- `rungrid sync` fast-forwards only each repository's local default branch and
  never mutates feature worktrees. `rungrid worktrees prune` only removes
  proven-obsolete lanes. Neither command binds a service to a worktree.
- `rungrid start`, generated wrappers, and Versions currently resolve
  `working_directory` inside the declared repository root. A Platform-style
  manifest invoked from a linked worktree therefore cannot run `api` from
  `~/worktrees/acme/api/GH-12` while leaving another service of the same Git
  repository on the primary checkout.
- Creating GitHub issues, branches, and worktrees remains Kit and native Git.
  Rungrid only selects registered worktrees and updates their existing refs.

## REQUIREMENTS

- Persist per-service checkout selection in project-local XDG state as
  `checkouts.json`. Do not write absolute worktree paths into `.rungrid.yaml`
  or require `.rungrid.local.yaml` to leave the workspace boundary.
- `rungrid worktrees list` reports every declared service repository's
  registered worktrees, including the primary, with branch, HEAD, primary flag,
  and which services currently select each checkout.
- `rungrid worktrees` with no subcommand opens a numbered action picker on a
  TTY for list, use, and update. `--json` or a non-TTY invocation without a
  subcommand fails closed.
- `rungrid worktrees use [service] [selector]` binds that service only.
  Selector is `primary`, an existing worktree path, a unique branch name, or a
  unique worktree directory base name. `--clear` removes the binding.
- With no service and/or selector, an interactive picker lists services and
  then only that service's repository worktrees. Headless or `--json`
  invocation without those values fails closed.
- A selected path must be an exact `git worktree list` entry whose Git common
  directory matches the service's declared repository. Detached HEAD, missing
  paths, foreign repositories, and unregistered directories are refused.
- Native start, health, Compose up/down, managed shells, and Versions resolve
  `working_directory` and environment providers inside the selected checkout
  and may not escape it.
- `rungrid worktrees update [--service name]... [--sync] [--dry-run] [--yes]`
  fast-forwards each targeted service's selected feature-branch worktree with
  fetch plus `merge --ff-only` (or expected-OID protection). `--sync` also runs
  the existing default-branch `sync` contract for involved repositories. On a
  TTY, omitted service and sync flags are prompted, a dry-run preview is
  shown, and apply requires confirmation unless `--yes` or `--dry-run`.
- Dirty, ahead, diverged, detached, or untracked worktrees are preserved with
  an exact reason. Never checkout, rebase, reset, stash, or force-update.
- Pause only services whose effective checkout is the worktree being updated.
- `--dry-run` is strictly non-mutating. Human and `rungrid/output/v1` JSON
  output are required.
- Do not create, move, or delete worktrees. Do not weaken `worktrees prune`.

### Non-goals

- Creating GitHub issues, branches, or canonical worktree lanes.
- Replacing Warp tab `cd` as a session selector.
- Expanding portable `workspace.root` containment to include `~/worktrees`.
- Rebasing or merging default-branch history into feature worktrees.

## ACCEPTED PLAN

1. Add an internal checkout store and resolver keyed by service name, validated
   against registered worktrees and Git common directories.
2. Add `worktrees list`, `worktrees use`, and `worktrees update` beside prune.
3. Thread the resolver through service execution, Compose shutdown, managed
   shells, Versions, and maintenance pause targeting.
4. Reuse the existing sync engine when `--sync` is set; keep feature-worktree
   fast-forward in the checkout package.
5. Document the commands in CLI_SPEC, README, and help. Cover selection,
   isolation, refusal, execution cwd, and update fast-forward in tests.

## DECISIONS

- Selection is per service, not per repository, so two processes in one Git
  clone can run from different registered worktrees.
- Rungrid selects existing worktrees only. Lane creation stays Git/Kit.
- Absolute selected paths live in XDG project state because they are
  machine-local and routinely outside `workspace.root`.
- `worktrees update` defaults to selected feature worktrees. `--sync` is the
  explicit default-branch path so GH-20's "never change feature branches"
  contract remains the `sync` command.

## DISCOVERIES

- Canonical lanes live outside `workspace.root`, so `.rungrid.local.yaml`
  cannot name them. XDG `checkouts.json` is the only portable-safe binding.
- `git merge-base --is-ancestor` treats a missing object as "not ancestor"
  both ways. Dry-run therefore has to `cat-file -e` the remote OID first and
  classify an absent object as `would-fast-forward`, not `diverged`.
- Two services in one Git repository must keep independent selections. Pause
  targeting uses each service's effective checkout, not the declared root.
- `rungrid sync` stays default-branch-only. Feature-worktree fast-forward is
  `worktrees update`; `--sync` is the explicit combined path.
- A selected feature worktree may contain a `working_directory` that does not
  exist on the primary checkout. Resolve must bind the selected path first and
  only require the declared working directory when no selection is stored.
- Numbered TTY pickers match prune and reconcile. `--json` and non-TTY stdin
  stay fail-closed so scripts never block on a prompt.

## VALIDATION

- `make check` passed: `fmt-check`, `vet`, `go test ./...`, `go test -race
  ./...`, `make sanitize`, license scan, compile, and Darwin/Linux amd64/arm64
  cross-builds.
- `make lint` reported 0 issues.
- `make vuln` found no reachable vulnerabilities.
- `make release-snapshot` built GoReleaser archives. Local Syft was
  unavailable; SBOM remains a hosted CI concern.
- Focused checkout tests cover per-service isolation, primary/clear, foreign
  and detached refusal, selected working-directory resolution, list
  selection, dirty preservation, and dry-run plus apply fast-forward.
- Source and test files in the affected scope are at most 300 lines.

## OUTCOME

Operators can `rungrid worktrees list`, bind one service with `use`, run
that service from the selected checkout, and `worktrees update [--sync]`
to fast-forward selected feature worktrees and optionally default branches.
Lane creation remains Git/Kit. Dirty, ahead, diverged, detached, and
untracked worktrees stay preserved.

## REPOSITORY MEMORY

Decision: created

Rationale: Per-service worktree selection is a new runtime contract that code
alone would hide: why overlay paths cannot represent canonical worktrees, why
selection is per service, and why update is split from `sync`. The
Constitution now records the selected-checkout boundary and the explicit
feature-worktree fast-forward exception.

Artifacts:

- `docs/specs/worktree-service-checkouts/SPEC.md`
- `docs/CONSTITUTION.md`
- `CLI_SPEC.md`
- `README.md`
