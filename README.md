```text
██████╗ ██╗   ██╗███╗   ██╗ ██████╗ ██████╗ ██╗██████╗
██╔══██╗██║   ██║████╗  ██║██╔════╝ ██╔══██╗██║██╔══██╗
██████╔╝██║   ██║██╔██╗ ██║██║  ███╗██████╔╝██║██║  ██║
██╔══██╗██║   ██║██║╚██╗██║██║   ██║██╔══██╗██║██║  ██║
██║  ██║╚██████╔╝██║ ╚████║╚██████╔╝██║  ██║██║██████╔╝
╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝╚═╝╚═════╝

                         one workspace. truthful lifecycle.
```

Rungrid turns a multi-service development workspace into one observable,
controllable system. A portable `.rungrid.yaml` is compiled into a detached
Process Compose runtime and, on macOS, an ordered Warp workspace:

1. Overview — read-only Process Compose TUI and selectable logs.
2. Versions — live process, listener, branch, commit, and worktree state.
3. Service tabs — one exclusive lifecycle owner per tab-owned application.

<!-- BEGIN KIT-MANAGED README BADGES -->
[![Last commit](https://img.shields.io/github/last-commit/jamesonstone/rungrid)](https://github.com/jamesonstone/rungrid/commits) [![Open issues](https://img.shields.io/github/issues/jamesonstone/rungrid)](https://github.com/jamesonstone/rungrid/issues) [![Pull requests](https://img.shields.io/github/issues-pr/jamesonstone/rungrid)](https://github.com/jamesonstone/rungrid/pulls) [![CI](https://github.com/jamesonstone/rungrid/actions/workflows/ci.yml/badge.svg)](https://github.com/jamesonstone/rungrid/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/jamesonstone/rungrid)](https://github.com/jamesonstone/rungrid/releases)
<!-- END KIT-MANAGED README BADGES -->

## Requirements

- macOS with Warp and zsh for the graphical workspace;
- macOS or Linux for headless use; and
- Process Compose `>=1.120.0,<2.0.0`.

Repository maintenance additionally uses authenticated GitHub CLI metadata and
`lsof` for pull-request and process proof. Stale dirty-primary recovery through
`rungrid reconcile` also requires `gitleaks` before it can create a WIP commit.

Native and Compose services may add their own executable requirements. Run
`rungrid doctor` for a redacted, project-specific report.

## Quick start

Create a manifest with the guided onboarding flow:

```sh
rungrid init
rungrid plan
rungrid doctor
rungrid up
```

To hand workspace wiring to a coding agent, generate a self-contained brief
before or after initialization:

```sh
rungrid instructions . ../api ../web
# Exact alias:
rungrid agent-start . ../api ../web
```

The brief tells the agent how to inspect each repository, preserve its rules,
choose a portable common workspace root, model shared infrastructure and
tab-owned applications, validate the result, and keep `.rungrid.yaml` as the
single service inventory. Supplied paths are printed as data and are never
executed. The command does not require a manifest or mutate workspace state.

Headless operation uses the same lifecycle:

```sh
rungrid up --headless --no-open
rungrid status
rungrid session api
rungrid down
```

Keep every configured repository's local default branch current without
switching or rewriting active feature worktrees:

```sh
rungrid sync --dry-run
rungrid sync
```

Preview and then reclaim clean linked worktrees whose pull requests merged and
whose remote branches were deleted:

```sh
rungrid worktrees prune --dry-run
rungrid worktrees prune
```

The prune command confirms removals interactively; automation must pass
`--yes`. Dirty, active, non-canonical, or unverifiable worktrees are preserved
with an exact reason. When Process Compose is active, authorized maintenance
appears in the Overview under the `maintenance` namespace with the same
lifecycle and logs.

Reconcile one physical clone, or recursively discover and reconcile every
clone beneath a directory, without changing manifest-scoped `rungrid sync`:

```sh
rungrid reconcile ~/src --dry-run --json
rungrid reconcile ~/src
```

The command discovers `origin`'s symbolic default branch, preserves recent or
process-active primary work, and applies the same native worktree cleanup proof.
Use `--include-submodules` to include submodule clones. High-trust agent mode is
available as `--agent`, `--agent=select`, `--agent=claude`, `--agent=warp`, or
`--agent=codex`; it delegates orchestration while requiring the native JSON dry
run and native apply command to remain the only mutation primitives.

Inside a Warp service tab, Ctrl-C stops that tab's service and returns to the
same managed zsh. Running the configured exact trigger, such as `make dev`,
restarts it. Other invocations of that executable pass through unchanged.

## Manifest

```yaml
api_version: rungrid/v1
kind: Workspace
project:
  name: Example Workspace
  slug: example-workspace
  id: example-workspace-k7m4q2
workspace:
  root: ..
repositories:
  api:
    path: api
    remote: origin
terminal:
  mode: warp
lifecycle:
  before_up:
    - name: prepare-database
      working_directory: control
      timeout: 2m
      run:
        argv: [docker, compose, up, --detach, --wait, database]
  after_down:
    - name: remove-infrastructure
      working_directory: control
      timeout: 2m
      run:
        argv: [docker, compose, down, --remove-orphans]
services:
  - name: database
    source: external
    activation: workspace
    external:
      command:
        argv: [database-ready, --quiet]
  - name: api
    repository: api
    source: native
    activation: tab
    working_directory: .
    run:
      argv: [go, run, ./cmd/server]
    terminal:
      trigger_argv: [make, dev]
    depends_on:
      database: running
```

`workspace.root` is relative to the manifest directory and may include sibling
repositories. Optional logical `repositories` keep each service's working
directory, Compose files, and environment providers inside its owning source
root; services without `repository` retain the implicit `workspace` behavior.
One-shot lifecycle commands run in order around the supervised workspace and
remain recoverable after a failed startup or cleanup. Workspace-owned services
start during `up`; tab-owned services remain disabled in Process Compose until
an exclusive `rungrid session` owns them. External services are readiness
dependencies only and are never directly started or stopped by service
commands.

The complete product and safety contract is [CLI_SPEC.md](CLI_SPEC.md). Durable
implementation rationale lives in
[docs/specs/rungrid-v1/SPEC.md](docs/specs/rungrid-v1/SPEC.md), with repository
maintenance decisions in
[docs/specs/repository-maintenance/SPEC.md](docs/specs/repository-maintenance/SPEC.md),
and filesystem reconciliation in
[docs/specs/repository-reconcile/SPEC.md](docs/specs/repository-reconcile/SPEC.md).

## Commands

Rungrid v1 provides `init`, `doctor`, `plan`, `generate`, `up`, `open`,
`attach`, `versions`, `status`, `logs`, `sync`, `reconcile`, `worktrees prune`,
`session`, `start`, `stop`, `down`, `uninstall`, `config`, `instructions`
(alias `agent-start`), `completion`, and `version`. Every JSON-capable command
uses a `rungrid/output/v1` envelope.

`rungrid --help` and `rungrid help` present the workspace lifecycle, service
ownership model, and commands grouped by workflow. Interactive terminals use
the Rungrid color palette; redirected output is stable plain text. Use
`--no-color` or set `NO_COLOR` to suppress ANSI styling explicitly.

Generated files, runtime identity, locks, logs, and terminal ownership live in
project-scoped XDG state. Rungrid verifies ownership hashes, PID start identity,
and Unix-socket identity before lifecycle mutation. `uninstall` removes only
verified project-owned state and Warp Tab Configs.

## Development

```sh
make build
make run ARGS="version"
make install
make check
make test-e2e
make release-snapshot
```

`make build` writes `./bin/rungrid` and links it as
`/usr/local/bin/rungrid`, matching the local command convention. Override
`PREFIX` to use another prefix, for example `make build PREFIX="$HOME/.local"`.
The link step requests administrator privileges only when the destination is
not writable, refuses to replace a regular file, and fails unless the final
symlink targets the just-built repository binary. `make run ARGS="..."`
executes the repository binary, and `make install` installs it with the active
Go toolchain. `make check` checks formatting, vets, runs unit/race and
dependency-license tests, verifies the specification sanitization contract,
and builds macOS/Linux targets without changing the global link. The opt-in
end-to-end suite launches a real Process Compose v1 runtime in temporary XDG
state.

## Maintainers

Maintained with 🪖 and ❤️ by [Jameson](https://github.com/jamesonstone)
(`jamesonstone`).
