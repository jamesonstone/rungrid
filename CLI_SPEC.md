# Rungrid CLI specification

Specification version: 2

Status: authoritative v1 contract

## 1. Purpose

Rungrid is a portable command-line tool for declaring, generating, and running
a local development workspace. A checked-in `.rungrid.yaml` describes the
services a project needs. Rungrid compiles that declaration into a detached
Process Compose runtime and, when requested, an ordered Warp workspace.

The v1 workspace preserves a simple operator model:

1. **Overview** is a read-only Process Compose TUI with selectable process
   logs.
2. **Versions** continuously reports process, listener, and source-control
   state.
3. Every remaining tab belongs to one configured application service. The tab
   may stop and restart its service while Process Compose remains the lifecycle
   authority.

The same manifest and lifecycle commands work headlessly on macOS and Linux.
Warp is the only graphical terminal adapter in v1.

## 2. Product boundaries

### 2.1 In scope

- declarative native, Compose, and external services;
- portable workspace roots that may contain multiple sibling repositories;
- ordered one-shot workspace prerequisite and teardown commands;
- workspace-owned and tab-owned activation;
- structured commands, environment providers, dependencies, health checks,
  restart policies, namespaces, and terminal metadata;
- detached Process Compose lifecycle over a project-scoped Unix socket;
- ordered Warp Tab Config generation and opening;
- exclusive service-tab sessions with signal-safe cleanup;
- interactive and non-interactive onboarding;
- self-contained coding-agent workspace-wiring instructions;
- deterministic plans and machine-readable output;
- project-scoped generation, runtime state, logs, and uninstall;
- shell completion and release metadata derived from the source repository.

### 2.2 Out of scope for v1

- a Rungrid-owned process dashboard;
- a separate all-logs terminal tab;
- passive per-service log tabs that do not own lifecycle;
- graphical terminal adapters other than Warp;
- a custom terminal-adapter protocol;
- Windows execution;
- container-orchestrator deployment;
- command-free multi-pane workspaces;
- management of external service lifecycle;
- storing resolved secret values in generated files or plans.

## 3. Compatibility and identifiers

The manifest API is `rungrid/v1`. Machine-readable command envelopes use
`rungrid/output/v1`.

Rungrid v1 requires Process Compose `>=1.120.0,<2.0.0`. It relies on disabled
processes, daemon lifecycle, Unix-socket clients, remote TUI attachment, and
process log streaming.

The executable, module path, repository URL, package coordinates, and install
metadata are derived from build metadata and the repository. Product behavior
must not hard-code a repository owner, package namespace, or installation tap.

## 4. Workspace files

### 4.1 Checked-in manifest

The project manifest is `.rungrid.yaml`. Its directory is the manifest
directory. The execution boundary is the separately resolved workspace root,
which defaults to the manifest directory but may be one of its ancestors. The
source manifest is portable and never contains an absolute developer path.

### 4.2 Local override

`.rungrid.local.yaml` is an optional ignored overlay adjacent to the source
manifest. It uses the same API and kind. Maps merge recursively, scalars
replace, and sequences replace in full. A service overlay merges by service
name. A lifecycle phase supplied by the overlay replaces that complete ordered
phase. The final merged document is validated as one manifest.

The local overlay must not be required for a repository's default workflow.
Secrets remain references, not literal values.

### 4.3 Imports

`imports` loads additional manifest fragments before the local overlay. Import
paths are resolved relative to the importing document, must remain within the
resolved workspace root, and may not form a cycle. Imports merge in listed
order. The root manifest has precedence over imports. Imported fragments may
not redefine `workspace.root`; the source manifest establishes the execution
boundary before imports are traversed.

## 5. Manifest contract

### 5.1 Top-level shape

```yaml
api_version: rungrid/v1
kind: Workspace

project:
  name: Example Workspace
  slug: example-workspace

workspace:
  root: .

repositories: {}

imports: []

runtime:
  startup_timeout: 45s
  shutdown_timeout: 20s
  process_compose:
    log_level: info
    log_rotation:
      max_size_mb: 10
      max_backups: 1

terminal:
  mode: warp
  open: true

lifecycle:
  before_up: []
  after_down: []

services: []
```

Allowed top-level fields are exactly `api_version`, `kind`, `project`,
`workspace`, `repositories`, `imports`, `runtime`, `terminal`, `lifecycle`,
and `services`. Unknown fields are errors.

### 5.2 Project

```yaml
project:
  name: Example Workspace
  slug: example-workspace
  id: example-workspace-k7m4q2
```

- `name` is the human label and is required.
- `slug` is optional and defaults to a normalized form of `name`.
- `id` is optional in source configuration. `rungrid init` generates and
  persists `<slug>-<random>` using a lowercase random suffix.
- An ID is stable across checkout moves and copies only when it is present in
  the manifest. Rungrid never hashes or persists the workspace's absolute path
  to create identity.

### 5.3 Workspace

```yaml
workspace:
  root: ..
```

`root` defaults to `.` and is resolved relative to the manifest directory. It
must be relative and may identify an ancestor directory so one manifest can
describe sibling repositories. The manifest directory must remain within the
resolved root.

Service working directories, environment-provider paths, Compose files, and
other workspace-owned paths resolve relative to the workspace root. Every
resolved path must remain within that root after normalization and
symlink-aware boundary checks. The adjacent local overlay is the only source
file whose location is not changed by this setting.

The relative declaration participates in deterministic generation. The
resolved absolute root exists only in machine-local runtime state and never
contributes to project identity.

### 5.4 Repository roots

```yaml
repositories:
  api:
    path: services/api
    remote: origin
  web:
    path: services/web
    default_branch: trunk
```

`repositories` is an optional map of stable logical names to directories
relative to `workspace.root`. Names match `[a-z][a-z0-9-]*`. Paths must be
relative, existing, distinct directories within the symlink-resolved workspace
boundary.

`remote` defaults to `origin` and must be a simple configured Git remote name.
Rungrid normally discovers the live default branch from that remote's symbolic
`HEAD`. `default_branch` is an optional fallback for remotes that do not
advertise a symbolic default; it never causes Rungrid to assume that every
repository uses `main`.

The reserved implicit repository name `workspace` always identifies
`workspace.root` and may not be redefined. A local overlay may replace a
declared repository path for a different checkout layout. Imports merge
repository maps recursively but cannot expand the outer workspace boundary.

Repository declarations are portable input. Plans, generation hashes, and
normalized manifests retain their logical names and relative paths. Absolute
resolved repository paths are machine-local runtime values and never project
identity material.

### 5.5 Lifecycle

```yaml
lifecycle:
  before_up:
    - name: prepare-database
      working_directory: tools
      timeout: 2m
      run:
        argv: [docker, compose, up, --detach, --wait, database]
      environment:
        providers:
          - type: dotenv
            path: .env
            optional: true
  after_down:
    - name: remove-infrastructure
      working_directory: tools
      timeout: 2m
      run:
        argv: [docker, compose, down, --remove-orphans]
```

Lifecycle commands are ordered one-shot Rungrid operations. They are not
Process Compose services and do not receive terminal tabs.
The phase fields are `lifecycle.before_up` and `lifecycle.after_down`.

- `name` is required and unique within its phase;
- `working_directory` defaults to the workspace root and is a checked
  workspace-relative directory;
- `run.argv` is a required non-empty argument vector and is never evaluated by
  an implicit shell;
- `timeout` is optional and defaults to the corresponding runtime startup or
  shutdown timeout;
- `environment` uses the same providers, precedence, execution-time secret
  resolution, and redaction rules as services;
- commands run sequentially in manifest order;
- a non-zero exit, timeout, signal, or executable failure fails the phase; and
- teardown continues through every command and reports the aggregate result.

`allow_failure` is not part of v1. A local overlay replaces an entire
`before_up` or `after_down` sequence rather than merging list elements.

### 5.6 Runtime

```yaml
runtime:
  startup_timeout: 45s
  shutdown_timeout: 20s
  log_retention: 168h
  resource_guard:
    sample_interval: 1s
    learning_window: 15m
    emergency_window: 3s
    sustained_window: 1m
    restart_limit: 3
    restart_window: 1h
    backoff_initial: 2s
    backoff_maximum: 8s
    emergency:
      cpu_percent: 75
      memory_percent: 50
      processes: 512
      threads: 2048
      thread_growth: 512
      thread_growth_window: 10s
  process_compose:
    executable: process-compose
    log_level: info
    log_rotation:
      max_size_mb: 10
      max_backups: 1
```

Durations use Go duration syntax. Executable values are names resolved through
the current execution environment unless an operator-local override supplies a
path. Required Process Compose compatibility is always enforced. Managed
process logs rotate when they reach `max_size_mb`, retain at most
`max_backups` compressed rollovers, and expire using `log_retention` rounded up
to whole days. The safe defaults are 10 MB and one rollover. Process Compose
internal diagnostics, which expose no rotation policy, are not persisted.

The resource guard is enabled for every managed native or Compose service.
It samples one batched host-process snapshot per interval. Three consecutive
emergency samples trigger immediate containment. Healthy Running or Ready
samples mature a service-specific P99 baseline after 15 minutes; only then can
a continuous one-minute sustained breach trigger containment. Learned limits
use `min(emergency, max(floor, P99 * multiplier, P99 + headroom))` and never
raise an emergency ceiling. The sustained defaults are five percent CPU and
memory floors, 32 processes, 128 threads, and 128 new threads per minute, with
the multipliers and headroom defined by the generated schema.

The global sample interval must be between 500 milliseconds and 10 seconds.
All timing, threshold, process-count, restart, and backoff overrides are
validated for ordering and safe bounds. A service may set `resource_guard` to
override its effective policy except for the workspace-wide sample interval.
An external service accepts policy only for reporting; it always remains
`observe_only`, and no override grants signaling authority.

### 5.7 Terminal

```yaml
terminal:
  mode: warp
  open: true
  theme: system
```

`mode` is `warp` or `headless`. `open` defaults to true for Warp and false for
headless mode. Terminal generation is skipped in headless mode. Themes are
advisory and may be omitted when the terminal does not expose a stable value.

### 5.8 Services

Service order is significant. It determines stable plan order and service-tab
order.

```yaml
services:
  - name: database
    source: compose
    activation: workspace
    compose:
      file: compose.yaml
      service: database
    health:
      command:
        argv: [database-ready, --quiet]
      interval: 2s
      timeout: 3s
      retries: 30

  - name: api
    repository: api
    source: native
    activation: tab
    working_directory: .
    run:
      argv: [go, run, ./cmd/server]
    terminal:
      title: API
      trigger_argv: [make, dev]
      include_in_versions: true
    depends_on:
      database: healthy
    environment:
      values:
        APP_ENV: development
      providers:
        - type: dotenv
          path: .env
          optional: true

  - name: web
    repository: web
    source: native
    activation: tab
    working_directory: .
    run:
      argv: [npm, run, dev]
    terminal:
      title: Web
      include_in_versions: true

  - name: documentation
    source: external
    activation: workspace
    external:
      url: http://localhost:9000/health
```

Each service supports:

- `name`: required stable identifier matching `[a-z][a-z0-9-]*`;
- `repository`: declared logical repository name, defaulting to `workspace`;
- `source`: `native`, `compose`, or `external`;
- `activation`: `workspace` or `tab`;
- `working_directory`: directory relative to the selected repository;
- exactly one source block appropriate to `source`;
- `environment` providers and literal non-secret values;
- `depends_on` lifecycle requirements;
- `health` readiness behavior;
- `restart` policy;
- optional validated `resource_guard` policy overrides;
- `namespace` grouping metadata;
- `terminal` tab behavior;
- optional `ports` hints used only when process listener discovery is
  unavailable.

Service names are unique. A dependency must name another service. Dependency
cycles are rejected.

### 5.9 Native source

```yaml
source: native
run:
  argv: [go, run, ./cmd/server]
  stdin: false
```

`run.argv` is a non-empty argument vector. It is never evaluated by a shell.
`stdin` defaults to false because Process Compose owns the service process.

### 5.10 Compose source

```yaml
source: compose
compose:
  file: compose.yaml
  project_name: example-workspace
  service: database
  profiles: [development]
  up_argv: [docker, compose]
  down_argv: [docker, compose]
```

Rungrid expands Compose commands as exact argument vectors. Startup targets the
configured service and waits according to its health contract. Shutdown uses
the matching file, project name, profiles, and service. Rungrid records the
exact expanded shutdown vector in generation state and reuses it during
`down`; it does not guess from the current shell.

Multiple services may reference one Compose project. Shutdown deduplicates
project-level actions while preserving service-specific stop semantics.

### 5.11 External source

```yaml
source: external
external:
  url: http://localhost:9000/health
  command:
    argv: [external-ready, --quiet]
```

External services are readiness dependencies only. Rungrid never starts,
stops, restarts, or uninstalls them.

The resource guard may report their readiness context and host observations,
but never signals them. Lifecycle hooks remain responsible for any external
startup or teardown they declare.

### 5.12 Activation

`workspace` services start as part of `rungrid up`. Their Process Compose
entries are enabled unless their source is external.

`tab` services are emitted as disabled Process Compose processes. A service
session starts them explicitly after acquiring exclusive ownership. Headless
operators use `rungrid session <service>` for the same semantics.

External services must use workspace activation because no tab can own their
lifecycle.

### 5.13 Terminal service metadata

```yaml
terminal:
  title: API
  trigger_argv: [make, dev]
  include_in_versions: true
```

`trigger_argv` defaults to `run.argv`. The two vectors remain distinct: the
supervised process can be exact while the interactive trigger stays familiar.
An empty trigger is invalid for a tab-owned service.

The managed service shell intercepts only an exact argument-vector match.
Invocations of the same executable with different arguments pass through to
the user's normal command. Shell aliases do not change stored trigger
semantics.

### 5.14 Environment

```yaml
environment:
  values:
    APP_ENV: development
  providers:
    - type: dotenv
      path: .env
      optional: true
    - type: command
      argv: [environment-export, --format, dotenv]
      timeout: 10s
    - type: direnv
      directory: .
```

Providers execute in listed order; later values override earlier values, and
explicit `values` override providers. Supported v1 provider types are `dotenv`,
`command`, and `direnv`.

Provider output is resolved only immediately before execution. Plans and
generated Process Compose files contain provider references or runtime wrapper
commands, never resolved secret values. Diagnostics redact values for keys
matching secret-like names and values explicitly marked sensitive.

### 5.15 Dependencies

```yaml
depends_on:
  database: healthy
  cache: running
```

Allowed requirements are `running`, `healthy`, and `completed_successfully`.
Rungrid validates the graph and compiles supported requirements into Process
Compose dependencies. External health dependencies are checked by a generated
wrapper.

### 5.16 Health checks

```yaml
health:
  command:
    argv: [curl, --fail, --silent, http://localhost:8080/health]
  interval: 2s
  timeout: 3s
  retries: 30
  start_period: 1s
```

Health commands use argument vectors and inherit the service's resolved
execution environment. A health failure is observable in status and Overview
and participates in startup timeout behavior.

### 5.17 Restart policy

```yaml
restart:
  policy: on-failure
  max_restarts: 5
  backoff: 1s
```

Policies are `no`, `always`, and `on-failure`. Tab-owned processes default to
`no` so a stopped tab returns control to its shell. Workspace-owned native
services default to `on-failure` with bounded retries.

## 6. Project state and generated artifacts

Rungrid follows XDG conventions. The effective state root is:

```text
$XDG_STATE_HOME/rungrid/projects/<project-id>/
```

When `XDG_STATE_HOME` is unset, the operating system's standard user state directory is
used. No generated state is placed in the repository except an explicitly
requested onboarding draft.

The project directory contains:

```text
generations/<generation-id>/
  manifest.yaml
  plan.json
  process-compose.yaml
  ownership.json
  terminal/warp/
    00_overview.toml
    01_versions.toml
    02_api.toml
    03_web.toml
  wrappers/
  logs/
runtime.json
lifecycle.json
lifecycle-logs/<generation-id>/
resource-guard/
  baselines/
  incidents/
  status.json
current
sessions/
tabs/
locks/
```

`current` identifies the active generation without embedding the checkout's
absolute path. Runtime-only records may contain the currently resolved
workspace path because execution needs it; they are replaced whenever the
workspace is reopened and are not identity inputs.

`lifecycle.json` is a crash-safe project journal. It records the project and
generation, manifest and lifecycle hashes, lifecycle state, completed
`before_up` commands, whether teardown is required, verified runtime identity
when one exists, timestamps, sanitized command outcomes, and the latest cleanup
failure. Its lifecycle states are `inactive`, `starting`, `active`,
`stopping`, and `cleanup-required`.

The journal is written atomically before an external prerequisite begins. Once
teardown is required it remains required until every configured `after_down`
command completes successfully, even if the supervisor never started or its
runtime record is absent. Command output is redacted and stored in private,
generation-scoped lifecycle logs rather than deterministic artifacts.

Resource-guard state is private and atomic. Every baseline, incident, status,
control-client registration, and circuit-reset request carries the complete
safe authority scope. Baselines are keyed by project, generation scope,
service, and a redacted service command/configuration hash. Only bounded
summaries are checkpointed. Incident records contain no command arguments,
environment, secrets, or raw process output and expire according to
`runtime.log_retention`.

All state directories and files are user-only. Files are written to a sibling
temporary file, synced when durability matters, permissioned, and atomically
renamed. Generated files have an ownership record containing:

- output API version;
- project ID;
- generation ID;
- artifact kind;
- generator version;
- SHA-256 content hash.

Regeneration replaces a prior artifact only when its ownership record names the
same project and its current hash equals the recorded hash. Otherwise Rungrid
fails closed and identifies the conflicting path.

## 7. Planning and generation

`rungrid plan` performs no lifecycle or terminal mutation. It loads imports and
the local overlay, validates the final manifest, computes the generation hash,
and prints the manifest directory, relative workspace root, declared logical
repository roots, ordered lifecycle commands, timeouts, teardown semantics,
and service actions. Secret values are never resolved and no absolute developer
path appears in deterministic output.

`rungrid generate` materializes a complete generation in a temporary directory,
validates every artifact, writes ownership metadata, and atomically promotes it.
An identical input and generator version produce the same generation ID and
artifact content. Runtime timestamps are excluded from deterministic files.

The generated Process Compose configuration:

- uses the recorded project-scoped Unix socket;
- emits native and Compose wrappers as exact commands;
- emits a hidden generation-scoped `rungrid-resource-guard` process;
- marks tab-owned processes disabled;
- emits disabled `maintenance` namespace jobs for authorized repository sync
  and worktree-prune requests;
- compiles dependencies, health checks, restart policy, namespaces, and logs;
- contains no resolved secrets;
- never generates a process for lifecycle control of an external service.

Lifecycle commands are validated and represented in plans and the journal, but
are not compiled into Process Compose configuration. `generate` never runs
them.

## 8. Runtime identity

The detached Process Compose daemon is authoritative for the active generation.
`runtime.json` records:

- project and generation IDs;
- normalized effective-manifest SHA-256;
- daemon PID, process start identity, and command identity;
- Unix socket path, owner, device, and inode;
- Process Compose version;
- generated configuration hash;
- resolved workspace path;
- start time.

Before any mutating client call, Rungrid verifies all of the following:

1. the runtime record belongs to the selected project;
2. the PID exists and its process start identity still matches;
3. the Unix socket is owned by the current user and has the recorded identity;
4. the Process Compose server answers over that socket;
5. the runtime generation matches the intended operation.

Finite Process Compose operations use bounded HTTP over the verified Unix
socket: liveness, get, and list queries have five-second maximums; start, stop,
and project-down use operation-specific deadlines and bounded responses. The
Process Compose executable remains only for daemon launch, log streaming, and
TUI attachment. Streaming clients run in isolated process groups and register
their project, generation, manifest, runtime, parent, process-start, operation,
and optional service identity for resource monitoring.

Every enforcement action is scoped by project ID, generation ID,
effective-manifest hash, verified Process Compose runtime identity, and proven
process ancestry. Filesystem location, executable name, port ownership, and
service name are supporting observations only and never confer termination
authority. If any scope or ownership proof is missing or changes, enforcement
fails closed.

The authority is the generated normalized effective manifest for the active
generation, not the current source manifest, import, or local overlay. Source
edits take effect only through normal reconciliation. Modification of the
generated manifest or Process Compose configuration invalidates enforcement.

Mismatch is a stale-runtime error, not permission to signal an arbitrary PID or
delete an arbitrary socket. The sole automatic recovery is a conclusively dead
runtime under the project lifecycle lock: the private runtime record and
lifecycle journal must match the selected project and generation, the recorded
PID must not exist, the expected socket path must be absent, and an immediate
re-read must prove the record unchanged. Rungrid may then remove only the stale
runtime record, clear the journal's runtime identity, finish any required
teardown, and restart normally. A live PID, present socket, changed record, or
identity mismatch remains a fail-closed conflict.

## 9. Lifecycle

### 9.1 Up

`rungrid up`:

1. loads, merges, and validates the complete manifest before external mutation;
2. acquires the exclusive project lifecycle lock and reconciles the journal
   with exact runtime identity, retiring only a conclusively dead, unchanged
   runtime record whose PID and socket are both absent;
3. finishes any required cleanup before accepting a different generation;
4. plans and generates, then atomically records `starting` and teardown intent;
5. runs `before_up` sequentially in manifest order;
6. starts or safely reuses the detached Process Compose daemon only after every
   prerequisite succeeds;
7. waits for workspace-owned services and external dependencies;
8. opens the ordered Warp workspace unless headless or explicitly disabled;
9. waits for requested tab-owned services to report a lifecycle state;
10. records `active` and prints a stable summary.

Tab-owned processes are not started merely because the daemon starts. Their
service tabs acquire ownership and start them.

An already active, exactly verified generation is reused without rerunning
prerequisites. Any failure or ownership-ending signal after teardown becomes
required stops the verified partial runtime, attempts every configured
`after_down` command, and records either `inactive` or `cleanup-required`.
Prerequisite failure never opens Warp.

### 9.2 Session ownership

`rungrid session <service>` is the lifecycle primitive used by Warp and
headless operators:

1. verify the active generation and service activation;
2. acquire an exclusive lock keyed by project, generation, and service;
3. register the owning shell/session identity;
4. start the exact Process Compose process;
5. stream raw service logs in the foreground;
6. on service stop, return to the caller's shell; and
7. on Ctrl-C, HUP, or TERM, stop the exact process and release ownership.

Only one live owner may exist for a service in a generation. Stale ownership is
reclaimed only after process identity checks prove the owner is gone.

A matching session also quiesces and releases ownership when generation
shutdown begins or its recorded runtime/socket identity disappears or changes.
The containing managed tab shell observes the same exact generation boundary,
terminates its child shell, and releases its tab registration; a shell from
another project or generation is unaffected.
During an intentional resource restart, a verified owner remains active,
prints each incident once, and waits for the service to return. A tab service
without a verified owner remains stopped after containment.

The session command returns to the shell when `rungrid stop <service>` stops its
process. The operator may then run the configured trigger to start a new session
in the same tab.

### 9.3 Start

For a workspace-owned service, `rungrid start <service>` directly starts its
Process Compose process and waits according to the configured readiness policy.
An open resource circuit blocks ordinary start. The explicit
`--reset-resource-circuit` flag closes the exact scope-verified service circuit
after operator review; changing the effective service command/configuration
identity also starts with a new baseline and circuit history.

For a tab-owned service:

- if a live owning tab exists, Rungrid does not create a duplicate owner and
  instructs the operator to run the configured trigger in that tab;
- if no live tab exists and Warp is available, Rungrid opens that service's Tab
  Config in the existing workspace window when possible, otherwise in a new
  window; and
- in headless mode, Rungrid instructs the operator to use `rungrid session`.

### 9.4 Stop

`rungrid stop <service>` stops the exact Process Compose process after runtime
identity verification. A tab-owned session observes the stop, releases its
registration, and returns to its managed shell.

Stopping an external service is an error because Rungrid does not own it.
Starting or stopping one service never runs global lifecycle hooks.

### 9.5 Down

`rungrid down`:

1. acquires the project lifecycle lock and records `stopping`;
2. publishes the immutable generation shutdown marker, suppresses guard
   restarts, and quiesces only matching sessions and managed tab shells;
3. stops tab-owned services in reverse dependency order;
4. stops workspace-owned native services in reverse dependency order;
5. runs the recorded exact Compose shutdown commands;
6. leaves external services untouched;
7. asks Process Compose to shut down and verifies daemon exit;
8. removes only verified runtime registrations and sockets; and
9. whenever the journal says teardown is required, attempts every configured
   `after_down` command in manifest order, even when the supervisor never
   started or is already absent.

A partial failure is reported per action and produces a non-zero exit. Rungrid
continues safe independent shutdown and teardown actions. Cleanup failure
retains `cleanup-required`, including the sanitized failure, so a later `down`
can retry. Successful teardown records `inactive`; repeated `down` is then a
no-op.

Rungrid never silently reruns prerequisites for an unverified stale runtime.
Generation or lifecycle-hash mismatch is explicit. Cleanup required by an old
generation must finish under its recorded manifest before a new generation may
start. When exact PID or socket identity cannot be proven, Rungrid fails closed
rather than signaling, deleting, or treating the workspace as clean.

## 10. Warp presentation

Rungrid generates ordered single-tab Warp Tab Config files:

```text
00_overview.toml
01_versions.toml
02_<first-service>.toml
03_<second-service>.toml
```

Only tab-owned services receive service tabs. Manifest order is preserved.
Filenames are normalized and collision checked.

### 10.1 Overview

Overview starts an exact remote Process Compose TUI attachment using the
recorded Unix socket and generated configuration identity. The attachment is
read-only: lifecycle keys are disabled. Process selection and log viewing are
provided by Process Compose.

The generated `maintenance` namespace contains disabled sync and worktree-prune
jobs. The public CLI writes a short-lived, generation-scoped authorized request
before starting either job. Starting one through a mutable client without a
valid request fails closed. Overview therefore displays the same operation
lifecycle and selectable logs without becoming an authorization surface.

### 10.2 Versions

Versions runs:

```text
rungrid versions --watch
```

It displays, for every included service:

- lifecycle state and health;
- PID when locally owned;
- listening ports discovered from the process tree;
- selected logical repository and Git branch and short commit for the service
  working directory;
- clean, dirty, or unavailable source-control state;
- worktree identity when available.

Output refreshes without clearing terminal scrollback unnecessarily and reacts
to terminal resizing. `--once` prints one snapshot. `--json` emits a
`rungrid/output/v1` envelope and implies one snapshot unless paired with an
explicit streaming mode. Watch mode polls lightweight process state every
second, batches listener discovery at most every five seconds, refreshes Git
metadata at most every ten seconds, and redraws only after material state
changes.

### 10.3 Service tabs

Each service tab starts a generated, isolated managed zsh. The shell:

1. sources the user's normal interactive zsh configuration;
2. registers the tab shell with project, generation, and service identity;
3. installs a wrapper for the executable named by `trigger_argv[0]`;
4. starts the initial `rungrid session <service>`; and
5. unregisters on shell exit.

The wrapper compares the complete argument vector. An exact configured trigger
starts a new session. Any other invocation resolves and executes the original
command unchanged. Wrapper recursion is prevented by recording the resolved
original executable before function installation.

Closing the tab sends HUP to the managed shell, which stops the service and
releases the lock. Ctrl-C while logs are foregrounded has the same ownership
effect without closing the tab.

### 10.4 Opening Warp

`rungrid open` opens the generated Tab Configs in order. It uses Warp's supported
URI mechanism and requests a new window for the workspace. `rungrid start` may
open one absent service tab without recreating the complete workspace.

Rungrid verifies generated files and executable availability before opening a
URI. URI values are encoded as data, never interpolated shell text.

## 11. Command-line interface

Global syntax:

```text
rungrid [global flags] <command> [arguments]
```

Global flags:

```text
--config <file>       manifest path; default .rungrid.yaml
--local <file>        local overlay path; default .rungrid.local.yaml
--state-dir <dir>     operator-local state-root override
--project <id>        select a known project when outside a workspace
--json                emit rungrid/output/v1 JSON where supported
--no-color            disable ANSI color
--quiet               suppress non-error human output
--verbose             add redacted diagnostics
```

Help output is a human interface with a stable plain-text fallback:

- root help presents a Rungrid ASCII banner, the manifest-to-runtime lifecycle
  model, service-ownership semantics, workflow-grouped commands, global flags,
  and the standard command-help hint;
- subcommand help uses consistent Usage, Aliases, Examples, Available Commands,
  Flags, and Global Flags sections when each section applies;
- interactive terminal output uses intentional ANSI styling and concise emoji
  section labels inspired by the repository's developer-tool conventions;
- redirected output, non-terminal writers, `--no-color`, and a non-empty
  `NO_COLOR` environment variable emit equivalent content without ANSI escapes
  or terminal-only heading decoration;
- color changes presentation only and never changes command names, ordering,
  aliases, flag parsing, completion, exit status, or machine-readable output.

### 11.1 init

```text
rungrid init [--non-interactive] [--from-compose <file>] [--force]
```

Interactive `init` uses a Bubble Tea flow that:

- identifies the workspace root and generates project identity;
- discovers Compose files and repository directories, assigning selected
  sibling Git roots stable logical names;
- infers candidate commands with confidence and evidence;
- requires confirmation for inferred execution behavior;
- configures source, activation, dependencies, health, environment, and
  terminal metadata;
- supports backtracking and terminal resizing;
- writes secret-free resumable drafts;
- invalidates affected draft choices when discovery inputs change;
- presents the final rendered manifest and plan before writing; and
- atomically writes `.rungrid.yaml` only after confirmation.

Non-interactive mode consumes explicit flags or standard input, validates the
complete result, prints actionable missing fields, and never guesses a command.
`--force` may replace only a file previously identified as Rungrid-owned or
after explicit interactive confirmation.

### 11.2 doctor

```text
rungrid doctor [--fix]
```

Doctor reports manifest validity, workspace and declared repository path
boundaries, required executables, authenticated GitHub CLI and `lsof`
availability for worktree proof, Process Compose compatibility, Warp/zsh
availability when graphical mode is selected, lifecycle working directories
and executables, state and journal permissions, stale runtime or
cleanup-required evidence, port conflicts, and source-control availability.
Doctor does not execute lifecycle commands.
`--fix` is limited to safe project-owned state repairs and requires
confirmation for user-visible changes.

### 11.3 plan

```text
rungrid plan [--output human|json]
```

Prints the merged configuration fingerprint, generation ID, service graph,
activation decisions, required providers, files to generate, and lifecycle
actions in exact order with working directories, argument vectors, timeouts,
and rollback behavior. It does not resolve secrets or change runtime state.

### 11.4 generate

```text
rungrid generate [--check]
```

Generates project state atomically. `--check` performs the full compilation and
comparison but writes nothing; it fails when checked artifacts are missing,
stale, modified, or nondeterministic.

### 11.5 up

```text
rungrid up [service ...] [--headless] [--no-open]
```

Starts or reuses the workspace runtime, waits for workspace activation, and
opens Warp unless disabled. It runs prerequisites and rollback according to the
lifecycle contract. Optional service arguments identify tab services whose
lifecycle state must be observed before success.

### 11.6 open

```text
rungrid open [service]
```

Opens the full ordered Warp workspace or one absent service tab. It does not
bypass session ownership.

### 11.7 attach

```text
rungrid attach [--read-only]
```

Attaches to the active Process Compose TUI over the verified socket. The
default is read-only. A mutable attachment requires an explicit flag and clear
warning because it can compete with tab ownership.

### 11.8 versions

```text
rungrid versions [--watch|--once] [--json]
```

Displays the Versions surface described above. Human terminal output defaults
to watch; redirected output defaults to once. Interactive watch mode uses the
terminal's alternate screen, renders its first frame from the top, and restores
the prior screen and cursor when it exits. Explicit watch output redirected to
a file or pipe remains plain text without terminal control sequences. A watch
exits cleanly when its generation begins shutdown or its recorded runtime scope
disappears or changes.

### 11.9 status

```text
rungrid status [service ...] [--json]
```

Reports runtime identity, generation, service lifecycle/health, ownership, and
readiness without mutation. It also reports lifecycle journal state, teardown
requirement, completed prerequisites, and the latest sanitized cleanup failure,
including when no supervisor exists.

Status also reports the complete active guard authority scope and proof state,
guard health, heartbeat and degraded reason, current metrics, baseline
maturity, effective limits, resource-restart history, circuit state, and the
latest service and control-plane incidents. Persisted incident summaries remain
available when the Process Compose runtime is inactive. JSON carries the same
fields in the `rungrid/output/v1` envelope.

### 11.10 logs

```text
rungrid logs [service ...] [--follow] [--tail <lines>] [--raw]
```

Reads logs over the Process Compose client when possible and from verified
generation logs as fallback. Raw mode is used by sessions and preserves service
bytes without Rungrid prefixes.

### 11.11 sync

```text
rungrid sync [--repository <name>]... [--dry-run] [--json]
```

Queries every unique declared Git common directory, fetches and prunes its
configured remote, and fast-forwards only the local default branch. If the
implicit `workspace` repository is not a Git worktree, Rungrid inspects only
the resolved working directories of workspace-owned services and deduplicates
their containing Git top-levels; it does not recursively scan the workspace.
Those inferred repositories are reported by workspace-relative top-level path.
Selecting `--repository workspace` selects that complete inferred set; reported
paths become individual selectors only when the manifest declares matching
logical repository names.
A checked-out default branch must be clean and advances with `git merge
--ff-only`; an unattached default ref advances only with an expected-old OID.
Missing, ahead, diverged, dirty, unavailable, or concurrently changed branches
are preserved.

Feature branches and their worktrees are never checked out, merged, rebased,
reset, or stopped. If a running service uses the checked-out default worktree,
Rungrid cooperatively pauses that exact process, advances the files, and
resumes it. A tab session retains its generation-scoped ownership throughout.
`--dry-run` may query the remote but performs no fetch, ref write, state write,
or process operation.

### 11.12 reconcile

```text
rungrid reconcile [path] [--dry-run] [--include-submodules] [--json]
rungrid reconcile [path] [--agent[=copilot|select|claude|warp|codex]]
```

Defaults `path` to the current directory. A path inside a Git worktree selects
only that physical clone. Other directories are scanned recursively without
following directory symlinks; clones are deduplicated by physical Git common
directory and submodules are omitted unless explicitly included. Reconciliation
always uses `origin`, requires its live symbolic HEAD, and does not inherit
manifest repository overrides or guess a default branch.

Apply fetches and prunes, re-reads the live default, and advances it only with
the expected-OID synchronization contract above. It then evaluates the primary
checkout using separate HEAD-commit, HEAD-reflog, dirty-path-mtime, process-cwd,
and dirty-open-file evidence. Recent or process-active feature roots are
preserved while an unoccupied default ref may still advance. A clean feature
root switches only after exact merged-PR proof or 72 hours of inactivity.

A dirty feature root always requires 72 hours of inactivity, exact
`GH-<number>` issue ownership, an initially empty index, human Git identity,
safe explicit paths, and a successful staged `gitleaks` scan before Rungrid may
create its prescribed WIP commit and switch the primary checkout. A stale dirty
default that is strictly behind may be stashed with nonignored untracked files;
the exact stash OID is reported and the stash is never popped or dropped.
Linked worktrees retain the stricter native prune proof and never receive the
primary-checkout exception.

When the repository belongs to a verified active workspace, apply acquires the
project maintenance lock and pauses/resumes only coordinator-owned affected
services. Any unattributed process or failed ownership inspection preserves the
dependent checkout. `--dry-run` may query filesystems, processes, Git, GitHub,
and the remote, but never fetches, writes refs or state, stages, stashes,
commits, switches, pauses, removes, or prunes. Independent repositories
continue after failures; any unsynchronized default or required safety failure
returns the typed partial status.

Agent mode is high-trust, mutually exclusive with `--dry-run` and `--json`, and
uses the following exact adapters after executable validation:

```text
copilot -C <path> -p <prompt> --allow-all --no-ask-user --autopilot
claude -p --dangerously-skip-permissions <prompt>  # cwd is <path>
oz agent run -C <path> --prompt <prompt>
codex exec -C <path> --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox <prompt>
```

The prompt requires the agent to run native JSON dry-run first, obey repository
rules, preserve uncertainty, use native reconcile as its sole mutation
primitive, and summarize the native result. Bare `--agent` selects Copilot;
`select` uses `fzf` when installed and otherwise requires a numbered terminal
selection. Provider output and exit status pass through unchanged.

### 11.13 worktrees prune

```text
rungrid worktrees prune [--repository <name>]... [--dry-run] [--yes] [--json]
```

Inspects exact registered linked worktrees once per Git common directory. A
candidate must be canonical, clean, inactive, non-primary, non-current,
non-detached, unlocked, absent from the manifest, backed by exactly one
same-repository pull request merged into the discovered default branch, equal
to that pull request's head OID, absent from the live remote, and unused as the
working directory of any live process. Process inspection is fail-closed.

The command previews every decision and requires interactive confirmation.
Non-interactive execution requires `--yes`. It revalidates each candidate,
removes only verified expected environment symlinks, runs ordinary non-force
`git worktree remove` and `git branch -d`, restores links if removal fails, and
prunes stale metadata. One candidate or repository failure does not block an
independently proven candidate; any failure still makes the complete command a
typed partial result. It never uses direct recursive deletion or deletes a
remote branch. `--dry-run` performs no local Git, state, process, symlink,
metadata, or filesystem mutation.

### 11.14 session

```text
rungrid session <service>
```

Acquires exclusive tab ownership, starts the service, follows raw logs, and
stops/releases on ownership-ending signals.

### 11.15 start and stop

```text
rungrid start <service> [--reset-resource-circuit]
rungrid stop <service>
```

Perform the activation-aware behavior defined in the lifecycle contract.

### 11.16 down

```text
rungrid down [--timeout <duration>]
```

Performs ordered project-owned shutdown. It is idempotent when no verified
runtime exists only when the lifecycle journal also proves no teardown is
required. A missing runtime does not suppress required `after_down` commands.

### 11.17 uninstall

```text
rungrid uninstall [--keep-logs] [--keep-config]
```

Runs safe `down`, verifies ownership records and path boundaries, and removes
only this project's generated state. It does not remove the checked-in manifest,
local overlay, external services, user shell configuration, Process Compose,
Warp, or unrelated terminal files. `--keep-config` preserves generated config
for diagnosis; it does not refer to the source manifest.

`--keep-logs` also preserves redacted resource incidents while removing
baselines, live status, client registrations, and pending reset requests.

Uninstall refuses to discard a cleanup-required journal. It succeeds only
after required teardown completes or when no teardown was ever required.

### 11.18 config

```text
rungrid config validate
rungrid config show [--merged] [--redacted]
rungrid config schema
rungrid config path
```

`show` is redacted by default. Schema output is stable JSON Schema derived from
the v1 manifest contract.

### 11.19 completion

```text
rungrid completion bash|zsh|fish
```

Writes shell completion to standard output without installing it.

### 11.20 version

```text
rungrid version [--json]
```

Reports semantic version, commit, build time, dirty marker when known, target,
and supported manifest/output APIs. Release and repository metadata come from
the build, not a hard-coded owner namespace.

### 11.21 instructions

```text
rungrid instructions [project-path ...]
rungrid agent-start [project-path ...]
```

`agent-start` is an exact alias of `instructions`. Both names are visible in
command help so a coding agent can discover the workflow without prior Rungrid
knowledge.

The command prints a self-contained Markdown brief for wiring the named
projects into one portable Rungrid workspace. The brief includes:

- the selected manifest path and ordered project-path hints as inert data;
- repository-rule and existing-work discovery requirements;
- the workspace-root, lifecycle, activation, dependency, environment, and
  terminal ownership contracts needed to build `.rungrid.yaml`;
- single-inventory and project-specific information boundaries;
- read-only validation followed by authorized lifecycle validation; and
- implementation, documentation, and pull-request handoff expectations.

When no project path is supplied, the command uses `.` as the discovery hint.
It does not require an existing manifest, inspect or execute the supplied
paths, access the network, generate state, or start services. Path values are
never interpreted as shell input or agent instructions. With `--json`, the
same brief and structured inputs are emitted in an `AgentInstructions`
`rungrid/output/v1` envelope.

## 12. Machine-readable output

JSON-capable commands emit one envelope:

```json
{
  "api_version": "rungrid/output/v1",
  "kind": "Plan",
  "project_id": "example-workspace-k7m4q2",
  "data": {},
  "diagnostics": []
}
```

Field names are snake case. Times use RFC 3339 UTC. Durations are strings.
Argument vectors remain arrays. New optional fields may be added within v1;
existing field meaning may not change.

Diagnostics have stable codes, severity, summary, optional field path, and a
redacted detail. Human and JSON modes share codes.

Filesystem reconciliation uses kind `RepositoryReconcileReport`. Its data
contains the deduplicated inventory, common directories, live default and local
and remote OIDs, per-source primary activity evidence, process ownership,
root and cleanup decisions, mutation OIDs, preservation reasons, and complete
failures. For example:

```json
{
  "api_version": "rungrid/output/v1",
  "kind": "RepositoryReconcileReport",
  "data": {
    "operation": "reconcile",
    "target": "/path/to/repository",
    "dry_run": true,
    "started_at": "2026-08-09T12:00:00Z",
    "finished_at": "2026-08-09T12:00:01Z",
    "repositories": [{
      "name": "repository",
      "path": "/path/to/repository",
      "common_dir": "/path/to/repository/.git",
      "default_branch": "trunk",
      "sync": {
        "name": "repository",
        "remote": "origin",
        "default_branch": "trunk",
        "local_oid": "1111111111111111111111111111111111111111",
        "remote_oid": "2222222222222222222222222222222222222222",
        "state": "behind",
        "action": "would-fast-forward"
      },
      "root": {
        "path": "/path/to/repository",
        "branch": "GH-123",
        "head_oid": "3333333333333333333333333333333333333333",
        "dirty": false,
        "activity_at": "2026-08-01T12:00:00Z",
        "head_commit_at": "2026-08-01T12:00:00Z",
        "head_reflog_at": "2026-08-01T12:00:00Z",
        "action": "preserved",
        "reason": "recent-clean-feature-root"
      },
      "worktrees": []
    }],
    "failures": []
  },
  "diagnostics": []
}
```

## 13. Exit status

```text
0  success
1  operation failed
2  usage or configuration error
3  dependency or compatibility error
4  runtime identity or ownership conflict
5  requested service not ready before timeout
6  partial shutdown, uninstall, maintenance, or reconciliation
130 interrupted by Ctrl-C
```

Signal-derived exit status follows operating-system conventions when a child execution
is the command's direct result.

## 14. Safety and privacy

Rungrid must:

- execute subprocesses with argument vectors, never shell-concatenated user
  input;
- resolve secrets only at execution time and redact diagnostics, plans, state,
  and evidence;
- reject imports and lifecycle working directories that escape the workspace;
- reject repository roots that escape the workspace and service-owned paths
  that escape their selected repository;
- require a relative workspace root and prove the manifest directory remains
  inside its symlink-resolved boundary;
- reject generated or uninstall paths that escape the selected project state;
- use user-only permissions for state, locks, sockets, logs, and terminal files;
- verify PID start identity and socket ownership before signaling or deleting;
- scope the global lifecycle lock by project and session locks by project,
  generation, and service;
- avoid following unverified symlinks during generation and uninstall;
- preserve modified generated artifacts and report the ownership conflict;
- leave external services untouched;
- avoid storing absolute workspace paths in stable project identity, manifests,
  plans, or deterministic generation hashes;
- omit environment values and command output from telemetry; and
- perform no network reporting unless a future explicit opt-in contract is
  added.

## 15. Legacy-workspace migration contract

A legacy managed-development workspace migrates by creating one portable
manifest as the sole service inventory:

1. choose a relative workspace root that includes every referenced repository;
2. declare each service-owning repository with a stable logical name;
3. represent one-shot infrastructure initialization and final teardown as
   lifecycle commands;
4. represent continuously supervised shared infrastructure as workspace-owned
   services;
5. represent applications as tab-owned native services tied to their declared
   repositories;
6. convert existing workspace startup, shutdown, status, attachment, and
   Versions entry points into thin Rungrid wrappers;
7. keep unrelated command-free workspaces outside Rungrid;
8. retain previous managed-development scripts as inactive rollback material
   for one release cycle;
9. isolate rollback state, socket, and ownership markers from Rungrid; and
10. remove legacy active-path documentation only after graphical and headless
   parity is proven.

`rungrid instructions <project-path>...` produces the neutral coding-agent
brief for this migration. The generated brief is guidance, not authority: the
agent must still follow each consumer repository's rules and preserve the
user's selected scope.

The migration must not maintain a second active service inventory. Project-only
details belong in that project's manifest, not in Rungrid source, tests,
examples, release metadata, or this specification.

## 16. Acceptance criteria

Rungrid v1 is complete when all of the following are demonstrated:

- schema/default/merge, workspace-root and declared-repository boundaries,
  lifecycle ordering and overlay replacement, path, dependency, trigger,
  redaction, journal transition,
  ownership, atomic write, lock, identity, and exit-code unit and fuzz tests
  pass;
- generic manifests, Process Compose output, plans, JSON, and ordered Warp files
  match reviewed golden fixtures;
- fake executable integration tests verify exact Process Compose, Compose,
  environment-provider, zsh, listener, Git, and Warp arguments;
- PTY tests prove initial tab start, duplicate rejection, Ctrl-C and HUP stop,
  same-shell trigger restart, unrelated-command pass-through, and absent-tab
  reopening;
- onboarding transition, backtracking, resize, profile, inference, confirmation,
  draft, resume, invalidation, and secret-free snapshot tests pass;
- a temporary-XDG end-to-end test covers init, plan, generate, up, Overview,
  Versions, session stop/restart, prerequisite rollback, missing-runtime
  teardown, retry after cleanup failure, down, and uninstall without changing
  unrelated files;
- Process Compose at the minimum supported version passes real integration;
- controlled macOS Warp and Linux headless smoke tests pass;
- a generic multi-repository workspace demonstrates exact tab/layout,
  lifecycle, crash-recovery, and teardown parity;
- formatting, vet, race, vulnerability, license, multi-OS build, release
  snapshot, fresh-install, and identifier/path sanitization checks pass; and
- release candidate evidence is immutable, scoped, and redacted.
