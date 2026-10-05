---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "delivery"
feature:
  id: "worktree-overrides"
  slug: "worktree-overrides"
  dir: "worktree-overrides"
relationships:
  - type: builds_on
    target: worktree-service-checkouts
  - type: builds_on
    target: terminal-resume
  - type: builds_on
    target: rungrid-v1
references:
  - id: cli-contract
    name: Rungrid CLI specification
    type: documentation
    target: CLI_SPEC.md
    relation: constrains
    read_policy: must
    used_for: public command, manifest, state, lifecycle, and output contracts
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
    name: Repository worktree override issue
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/54
    relation: tracks
    read_policy: must
    used_for: delivery ownership
    status: active
skills: []
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Let an operator point one repository's managed services at a different
checkout of that repository, normally a Git worktree holding an in-flight fix.
Rungrid keeps supervising those services from the new checkout with its resource
guard, logs, tabs, and `resume`. Reverting takes one command.

## CONTEXT

- A manifest resolves each `working_directory` against its declared
  repository root. A Platform-style manifest declares no `repositories`: it
  uses the implicit `workspace` repository rooted at the parent of many sibling
  primary checkouts, with `working_directory: <repo>`. Every managed service
  therefore runs from a primary checkout, which operators treat as read-only.
- The only workaround was `rungrid stop <service>` followed by a hand-run build
  on the same port. That loses supervision, the resource guard, logs, Warp tabs,
  and `resume`, and leaves an unmanaged process holding the port.
- GH-49 `worktrees use` is a persistent, per-service binding. It applies to the
  next start only, does not restart anything, is invisible in `status`, and
  joins the selected worktree with the workspace-relative `working_directory`.
  That join is wrong for implicit-workspace services. It does not cover this
  workflow.
- Service wrappers call `rungrid internal exec`, which resolves the checkout
  at exec time. A change in project state is therefore picked up by the next
  start without regenerating Process Compose configuration.
- Platform argv such as
  `direnv exec . ../platform/tooling/mvp/exec-local.sh make dev` contains
  relative paths that leave the service's repository.

## REQUIREMENTS

- Commands:
  - `rungrid override set <repo|service> <path|worktree>`
  - `rungrid override clear [<repo|service>]`, where omitting the argument
    clears every override
  - `rungrid override list`
  - `rungrid override sync`
  - `rungrid up --override <repo|service>=<path|worktree>`, repeatable
  - a root shortcut `rungrid <service> [<worktree>]`. With a worktree it
    equals `override set`. Without one, a TTY opens an action menu (currently
    only `worktree`) and then a worktree picker. Both pickers navigate with
    arrow keys or `j`/`k`.
- Overrides are keyed by repository. The repository is the Git top level of a
  service's declared working directory, and every managed service in it
  switches together. A service-name target resolves to its repository, and
  the output names the repository and every affected service.
- Validation fails closed:
  - the target must exist and must be a checkout of the same repository, by
    equal Git common directory or equal normalized `origin` URL;
  - each affected service's working directory must exist in the target;
  - Compose files and environment-provider paths must stay inside the target
    checkout;
  - unknown targets, `external` services, and repositories with no managed
    service are refused;
  - a dirty target is a warning, not a refusal.
- A per-service `worktree:` declaration in the manifest or local overlay names
  the same selector. `override sync` applies the declarations to the live
  runtime, and a fresh `up` seeds them. Declarations are excluded from the
  normalized manifest and generation identity.
- Lifecycle:
  - set and clear restart only affected services that are running, through the
    normal stop and `rungrid start` path;
  - global lifecycle hooks never run and keep their declared directories;
  - a stopped service stays stopped, and an operator stop intent survives;
  - overrides are generation-scoped runtime state: `resume` keeps them, `down`
    clears them, and a fresh `up` starts clean except for `--override` and
    declarations.
- Relative argv paths that resolve outside the original checkout are
  re-anchored to their original absolute location and reported. Nothing
  silently runs a different file.
- Visibility:
  - `status` and `versions` mark overridden services with their path and branch;
  - the Overview shows a banner in the service log at each start;
  - Warp tab titles mark overridden services, and tabs open in the override
    directory;
  - `override list` and `--json` report each override's repository, path,
    branch, HEAD, dirty state, and affected services.
- `--json` uses `rungrid/output/v1` envelopes. Diagnostics use the new `RG18xx`
  block.

### Non-goals

- Writing, copying, or synthesizing environment files. The override checkout's
  own `.env`/`.envrc` applies through the service's existing environment
  mechanism, such as `direnv exec .`. Callers prepare it.
- Installing dependencies or building in the override checkout.
- Creating, moving, or removing worktrees.
- Overriding lifecycle hooks or external services.

## ACCEPTED PLAN

1. Add an `internal/override` package:
   - repository discovery keyed by the Git top level of each declared working
     directory;
   - target and selector resolution plus validation;
   - an argv re-anchoring planner;
   - a generation-scoped `overrides.json` store;
   - an exec-time resolver layered over `checkout.Resolve`;
   - worktree candidate metadata for the picker;
   - human and JSON reports.
2. Route service exec, health, Compose shutdown, managed shells, Versions, and
   maintenance pause targeting through the override resolver. Exec rewrites
   argv and prints the Overview banner.
3. Add `lifecycle.ApplyOverrides`. It stops running affected services without
   recording a stop intent, starts them through `lifecycle.Start`, stops
   affected running tab processes, and reinstalls owned Warp tab configs. Wire
   it into `up` (clear, then seed before the supervisor starts), resume
   recovery (preserve), and `down` (remove on `inactive`).
4. Add the `worktree:` service field. It is extracted at load and stripped
   before normalization.
5. Add the `override` command group, `up --override`, the root service
   shortcut, and Bubble Tea pickers.
6. Mark overrides in `status`, `versions`, and Warp titles.
7. Document the feature in CLI_SPEC, README, and this spec. Cover it with unit,
   command, and integration tests and a local Process Compose smoke.

## DECISIONS

- **Command names.** The group is `override`, not `worktrees use`. A
  `worktrees` subcommand selects or maintains checkouts and never restarts
  anything. An override is runtime state that restarts services, is scoped to
  a generation, and is cleared by `down`. Manifest application is
  `override sync` because top-level `sync` already means "fast-forward default
  branches". The root shortcut `rungrid <service> [worktree]` is the preferred
  operator path. Built-in commands win any name collision; `override set`
  always works.
- **Key.** An override is keyed by the Git top level of the declared working
  directory. It is named by the declared repository name when that repository's
  root is the top level, and otherwise by the top level's workspace-relative
  path. This matches maintenance discovery and fits implicit-workspace
  manifests.
- **Same repository.** Equal Git common directory, so any registered worktree,
  or equal normalized `origin` URL, so a separate clone. A target equal to the
  original top level, or the selector `primary`, means clear.
- **Precedence.** An override wins over a GH-49 `worktrees use` binding, which
  wins over the declared root. An override maps paths from the original top
  level, so the binding is not consulted while an override is active.
- **Path mapping.** A path inside the original top level maps to the same
  relative path in the override. A path outside it stays where it is. The
  working directory always maps. The repository root used for environment
  containment maps when it is inside the top level. Otherwise, as for the
  implicit workspace, it becomes the override top level, because the
  Constitution keeps provider paths inside the selected checkout.
- **Requirement 5: re-anchor, not refuse.** The real Platform manifest runs
  `../platform/tooling/mvp/exec-local.sh` from each repository, so refusing
  would make overrides unusable there. The decision depends on where each
  argument, or the value after `=` in a `--flag=value` or `NAME=value`
  argument, lands from the original working directory:
  1. it lexically stays inside the original top level: it is kept unchanged
     and follows the override;
  2. it leaves the top level and comes back in, such as `../repo/x`: it is
     rewritten to the absolute mapped path in the override;
  3. it resolves outside the original top level: it is rewritten to its
     absolute original path, so the same file runs as before.

  Only arguments containing a `..` segment can reach cases 2 or 3, so
  ordinary arguments are never touched. Absolute arguments are never touched.
  Every rewrite is reported by `override set`, `list`, and `--json`, and named
  in the exec banner. Rewrites apply to run, health, and Compose argv.
- **Compose files are refused, not re-anchored.** The Constitution keeps
  Compose files and environment-provider paths inside the selected checkout.
  A `compose.file` or provider path that leaves the override checkout is
  therefore refused with `RG1814`. Compose `up_argv` and `down_argv` are
  ordinary argv and are re-anchored.
- **Review hardening.**
  - Exec-time resolution matches an entry by its recorded service list and
    its path, so a nested checkout under an overridden top level is never
    moved implicitly.
  - Applying re-reads the supervisor record under the lock and refuses with
    `RG1824` if the generation or PID changed.
  - An unreadable service state is reported as `restart-failed`, never as
    "not running".
  - Provider validation reuses the execution-time symlink and existence
    check, so a missing `.env` or a symlink that leaves the checkout is
    refused before any service stops.
  - A Compose service without `project_name` produces a warning.
  - The runnable root restores cobra typo suggestions in `RG1820`.
- **Lifecycle hooks** keep their declared directories. They prepare
  workspace-wide infrastructure rather than the overridden code, and they never
  rerun for a single-service change.
- **Restart semantics.** An affected `workspace` service in a running-like
  state is stopped through the supervisor, without a stop intent, then started
  through `lifecycle.Start`. Any other service is left untouched and uses the
  override at its next start, so a stop intent is never bypassed. A running
  `tab` service process is stopped, and the operator reruns the trigger in its
  tab. Rungrid never types into a user shell.
- **Runtime required.** `override set`, `clear`, `sync`, and the shortcut need
  a verified active runtime. Without one they refuse and point at
  `rungrid up --override`. That keeps "a fresh up starts clean" true.
- **Storage.** `overrides.json` in project state records project, generation,
  and per-repository entries. Entries from another generation are ignored. A
  fresh runtime start replaces the file with the seeded set. Resume recovery
  through `up` preserves a same-generation file. `down` removes it when the
  journal reaches `inactive`.
- **Declarations are not identity.** The `worktree:` field is extracted from
  the merged manifest and cleared before normalization. Editing a declaration
  therefore never changes the generation, and a live runtime can apply it with
  `override sync`. Sync is authoritative: it sets every declared repository
  and clears every other override. Two services in one repository that declare
  different selectors are refused.
- **Overview marking.** The Overview is a read-only Process Compose
  attachment, so the reliable marker is a log banner. `internal exec` prints
  it before replacing itself.
- **Picker.** Rows are ordered by `updated_at`, newest first. For a linked
  worktree, `created_at` is the modification time of its administrative
  `commondir` file, which is written once by `git worktree add`. For the
  primary checkout it is the Git directory's time. `updated_at` is the
  checkout's `logs/HEAD` modification time, falling back to the HEAD commit
  time. Rows also show the branch, short HEAD and subject, dirty state, and
  whether the row is the current checkout.

## DISCOVERIES

- A GH-49 binding on an implicit-workspace service joins the worktree with the
  workspace-relative `working_directory`, for example `<lane>/labcore`.
  Overrides avoid that join by mapping relative to the Git top level.
- `environment.Resolve` rebuilt the working directory as
  `root + working_directory`, which is wrong for any relocated checkout. Exec
  paths now call `ResolveEnvironment` with the resolved working directory.
- Generation identifiers are content hashes, so `down` followed by `up`
  reuses the same identifier. Generation scoping alone cannot clear overrides;
  `up` and `down` must clear them explicitly, as they already do for stop
  intents.

- `cmd.inputIsTTY` treated any character device as a terminal. `/dev/null`
  is one, so a Bubble Tea picker reading it never returned. It now uses a real
  terminal check, which also hardens the existing numbered pickers.
- Bubble Tea probes the terminal background with OSC 11 at startup. A PTY test
  harness must answer the probe, or the first keystrokes are consumed while
  the program waits. Real terminals answer it.
- First run against a real Platform workspace: service wrappers exec the
  Rungrid binary that started the runtime (`RUNGRID_EXECUTABLE`). A runtime
  started by a pre-override build restarted labcore in its primary checkout
  while `override set` reported success.
  - Fix: `internal exec` writes an `exec-acks/<generation>-<service>`
    acknowledgement of its working directory.
  - `ApplyOverrides` clears the acknowledgement before the restart and
    requires the expected directory afterwards. A missing acknowledgement is
    reported as `restart-failed` with a restart-the-workspace instruction.
  - Overrides therefore need a runtime started by an override-capable build.
- An explicit `down` is the only place overrides are removed. Recovery inside
  `up` runs the same journal cleanup, so removing overrides there would drop
  them before resume could preserve them.

## VALIDATION

- `make check` passed: fmt-check, vet, `go test ./...`, `go test -race ./...`,
  sanitize, license, compile, and Darwin/Linux amd64/arm64 cross-builds.
- `make lint` reported 0 issues. `make vuln` found no vulnerabilities.
- Unit tests in `internal/override`:
  - the argv re-anchoring rule, covering all three cases plus flag values,
    environment assignments, absolute paths, and URLs;
  - remote normalization;
  - repository keying across a multi-service repository;
  - selector forms and same-origin clones;
  - every refusal code from `RG1805` to `RG1814`, plus `RG1816` and
    `RG1817`, including a missing non-optional `.env` and an `.env` symlink
    that leaves the checkout;
  - entry service-list matching;
  - dirty warnings and generation-scoped storage that fails closed;
  - exec-time resolution: mapped working directory, argv, an unrelated
    service, an external service, a stale generation, and `RG1821`;
  - declarations stripped from the normalized manifest;
  - picker candidates ordered newest first with current and dirty markers.
- Lifecycle tests:
  - set, clear, and exclusive sync bookkeeping;
  - restart semantics for running workspace services, a preserved stop intent,
    tab-stopped, tab-idle, restart-failed, and unreadable state;
  - the `RG1824` changed-runtime refusal;
  - exec acknowledgement checks: a legacy runtime and a wrong directory are
    both reported as `restart-failed`;
  - seeding versus preserving on `up` and recovery;
  - status marking.
- Integration test `TestOverrideLifecycleAcrossClearAndResume` runs on real
  Git checkouts with a fake Process Compose runtime that resolves each start
  through `override.Resolve`, as `internal exec` does. Both services sharing
  `svc` restart in the worktree while the unrelated service keeps its PID.
  Clear restores the primary. After a simulated terminal restart, resume
  restarts the crashed service in the override, the operator-stopped service
  stays stopped, and `down` removes the override.
- Command tests:
  - the `RG1815` runtime-required refusals for `set`, `clear`, `sync`, and the
    shortcut;
  - the empty `OverrideList` envelope and its text;
  - shortcut refusals `RG1819` and `RG1820`, including typo suggestions;
  - `up --override` validation before start: `RG1817`, `RG1805`, `RG1806`,
    and `RG1810`;
  - `OverrideReport` text and JSON fields;
  - picker navigation with arrows, `j`/`k`, Enter, and Esc;
  - worktree row content.
- Local smoke on macOS with Process Compose v1.120.0, headless, against a
  temporary workspace with `alpha` and `beta` sharing `svc` and `gamma` in
  `other`:
  - `rungrid alpha GH-1` restarted alpha and beta in the worktree with
    `../tools/run.sh` re-anchored; gamma kept its PID;
  - `rungrid stop beta` followed by `override clear svc` restored alpha and
    left beta stopped;
  - `resume` on a live runtime, and `resume` after the Process Compose daemon
    was terminated, both kept the override;
  - `down` removed `overrides.json`, and the next `up` started clean;
  - `up --override beta=GH-1` seeded a fresh runtime;
  - adding `worktree: GH-7` and running `override sync` applied it, cleared
    the other override, and left the generation ID unchanged;
  - all refusal messages were exercised;
  - the PTY-driven `rungrid alpha` menu, worktree picker, and `j`/`k`/Enter
    selection applied the chosen worktree;
  - no processes leaked after `down`.
- Every changed source and test file is 300 lines or fewer.

## OUTCOME

Operators run `rungrid <service> <worktree>`, the interactive
`rungrid <service>`, `rungrid override set|clear|list|sync`, or
`rungrid up --override`. Every managed service of the service's repository then
runs from the chosen checkout under full supervision, marked in `status`,
`versions`, Overview logs, and Warp tab titles. Requirement 5 is resolved by
re-anchoring escaping argv to the original target and refusing escaping
Compose files and environment providers. Warp tab titles and the
graphical-tab path were not exercised in a live Warp window. They are covered
by unit tests and the headless smoke only.

## REPOSITORY MEMORY

Decision: created

Rationale: Repository overrides add a runtime contract that code alone would
not explain:
- why overrides are keyed by Git top level rather than by service;
- why argv is re-anchored while Compose files and providers are refused;
- why declarations are excluded from generation identity;
- why `down` rather than journal cleanup ends them;
- why the command group is `override` rather than `worktrees`.

The invariant and definition were promoted to the Constitution. CLI_SPEC and
README carry the public contract.

Artifacts: docs/specs/worktree-overrides/SPEC.md, docs/CONSTITUTION.md,
CLI_SPEC.md, README.md
