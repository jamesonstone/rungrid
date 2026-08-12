---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "validation"
feature:
  id: "runtime-resource-guard"
  slug: "runtime-resource-guard"
  dir: "runtime-resource-guard"
relationships:
  - type: builds_on
    target: rungrid-v1
  - type: builds_on
    target: bounded-local-runtime
references:
  - id: process-compose-client
    name: Process Compose client
    type: code
    target: internal/processcompose/client.go
    relation: replaces
    read_policy: must
    used_for: bounded control-plane operations
    status: active
  - id: runtime-identity
    name: Runtime identity
    type: code
    target: internal/supervisor/runtime.go
    relation: extends
    read_policy: must
    used_for: immutable enforcement authority
    status: active
  - id: testing-reference
    name: Testing reference
    type: documentation
    target: docs/references/testing.md
    relation: constrains
    read_policy: must
    used_for: validation contract
    status: active
skills: []
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Prevent a Rungrid-owned control client or managed service tree from consuming
unbounded host resources, while making every containment action provably local
to one immutable generated runtime.

## CONTEXT

A long-lived `rungrid session aquarium` spawned a one-shot Process Compose
`process get` client. After the recorded runtime and Unix socket disappeared,
that client spun for hours and saturated most of the host. Memory pressure was
healthy, the Aquarium application was not the source, and lifecycle teardown
had completed, so the failure crossed Rungrid's control-plane and session
cleanup boundaries.

Rungrid currently launches the Process Compose binary for state and lifecycle
queries even though Process Compose 1.120 exposes equivalent HTTP routes over
the recorded Unix socket. Existing runtime records already preserve strong PID,
process-start, socket, generation, owner, and configuration proofs, but they do
not bind those proofs to the normalized effective manifest or expose a resource
containment policy.

## REQUIREMENTS

- Use bounded Unix-socket HTTP for Process Compose ping, get, list, start, stop,
  and project stop operations. Retain the binary only for daemon launch, log
  streaming, and TUI attachment.
- Make a matching session exit and release ownership when generation shutdown
  begins, runtime state disappears, or runtime identity no longer matches.
- Scope every guard action to the exact project, generation, normalized
  effective-manifest hash, Process Compose process-start and command identity,
  socket identity, generated Process Compose configuration hash, and proven
  process ancestry.
- Treat names, paths, directories, listener ports, and repositories only as
  observations. They never grant termination authority.
- Enforce managed native and Compose host process trees. External services and
  manually started processes remain observation-only and cannot be promoted to
  guard ownership by manifest overrides.
- Sample one bounded Darwin/Linux process snapshot per interval without
  commands, environments, or raw process output. Derive cumulative CPU, RSS,
  process count, thread count, ancestry, PGID, and process-start identity.
- Apply three-sample emergency ceilings and learned sustained limits. Learn
  only from healthy Running or Ready samples for 15 minutes; a baseline may
  raise a sustained threshold but never an emergency ceiling.
- Persist redacted, private, atomic baseline, incident, and current status
  records. Bound saved baseline summaries across generations, keep incidents
  according to runtime log retention, and retain incidents with
  `uninstall --keep-logs`.
- Stop through Process Compose first. Revalidate authority before direct
  signaling, prove every group member belongs to the captured tree, then use
  TERM and identity-matched KILL only if graceful stop fails.
- Automatically restart with 2, 4, and 8 second backoff. A fourth resource
  breach in one rolling hour opens the circuit and leaves the service stopped.
- Expose guard authority, health, heartbeat, metrics, baseline maturity,
  effective limits, restart history, circuit state, and latest incidents in
  human and JSON status, including when the runtime is inactive.
- Support validated workspace defaults at `runtime.resource_guard`, optional
  service overrides at `service.resource_guard`, and explicit recovery through
  `rungrid start <service> --reset-resource-circuit`.
- Keep every affected handwritten implementation and test file at or below 300
  physical lines.

### Default policy

- Snapshot interval: one second, configurable only from 500 milliseconds to 10
  seconds.
- Emergency after three consecutive samples: 75 percent host CPU, 50 percent
  host memory, 512 processes, 2,048 threads, or 512 new threads in 10 seconds.
- Sustained breach window: one continuous minute after 15 healthy minutes.
- Sustained threshold: `min(emergency, max(floor, P99 * multiplier, P99 +
  headroom))`.
- CPU: five percent floor, multiplier three, two percentage-point headroom.
- Memory: five percent floor, multiplier 1.5, two percentage-point headroom.
- Processes: 32 floor, multiplier two, 16 headroom.
- Threads: 128 floor, multiplier two, 64 headroom, and 128 threads per minute
  sustained-growth limit.

### Authority invariant

> Every enforcement action is scoped by project ID, generation ID,
> effective-manifest hash, verified Process Compose runtime identity, and proven
> process ancestry. Filesystem location, executable name, port ownership, and
> service name are supporting observations only and never confer termination
> authority. If any scope or ownership proof is missing or changes, enforcement
> fails closed.

The active authority is the immutable generated normalized manifest, not the
current source `.rungrid.yaml`, its imports, or its selected local overlay.
Source edits take effect only after reconciliation compiles a new generation.
Modification of a generated manifest or Process Compose configuration
invalidates the guard scope.

### Non-goals

- Changing Platform or Aquarium source or service commands.
- Signaling processes inside Docker Desktop's VM.
- Owning lifecycle hooks or external PostgreSQL and LocalStack processes.
- Opening an upstream Process Compose issue without separate authorization.
- Treating the soak collector as a separate user-facing service.

## ACCEPTED PLAN

1. Extend the manifest and normalized generation with validated resource-guard
   defaults and overrides, immutable effective-manifest identity, and a hidden
   generation-scoped guard process.
2. Replace query and lifecycle CLI calls with a bounded Unix-socket HTTP
   gateway using operation-specific deadlines and response limits.
3. Strengthen session registration and teardown so exact-generation sessions
   and their managed tab shells quiesce instead of retrying or surviving a
   missing or changed runtime.
4. Add private authority-scope, process-snapshot, baseline, incident, circuit,
   and status components. Register and isolate the remaining streaming clients.
5. Implement fail-closed containment, graceful stop, verified TERM/KILL
   escalation, restart coordination, tab-owner behavior, and shutdown
   suppression.
6. Extend status, CLI documentation, lifecycle cleanup, and retained incident
   reporting without changing external-service ownership.
7. Add deterministic unit, authority, integration, Process Compose E2E, and
   Platform compatibility coverage, then run the full repository validation
   contract and the time-bound soak.
8. Prepare a sanitized upstream reproducer draft, but do not publish it.

## DECISIONS

- The guard is compiled into and launched by the same Process Compose
  generation it observes, but it receives no lifecycle authority from that
  parent relationship alone. Every mutation repeats the full scope proof.
- Direct Unix-socket HTTP removes the runaway one-shot client class at its
  source. Resource limiting for remaining streaming clients is defense in
  depth, not the primary fix.
- Emergency protection is active immediately. Sustained enforcement waits for
  15 healthy minutes to avoid treating ordinary compile and reload spikes as
  a baseline.
- Three resource-triggered restarts are allowed within one rolling hour. The
  fourth breach opens the circuit.
- Overrides may tune thresholds and timing only within safe schema bounds.
  They cannot disable ownership checks, exceed hard emergency schema ceilings,
  or grant authority over external services.

## DISCOVERIES

- The observed hot process was a Rungrid-spawned Process Compose control client,
  not the Aquarium application or Process Compose supervisor.
- Process Compose 1.120 exposes all finite state and lifecycle operations needed
  by Rungrid through its Unix-socket HTTP API.
- Process Compose starts managed services in process groups, which provides a
  useful containment boundary only after ancestry, group membership, and
  process-start identities are independently verified.
- Darwin rejects long absolute Unix-socket paths even when Process Compose
  successfully bound the socket through a short relative path. Direct HTTP
  therefore dials long recorded paths through a deterministic private short
  symlink, while the original absolute socket identity remains enforcement
  authority and the alias grants none.
- Darwin `ps` exposes thread counts by emitting one row per thread rather than
  a portable thread-count column. The sampler aggregates the fixed rightmost
  identity/metric columns by PID and never retains command text.
- A mature adaptive baseline must not ingest a sample that already breaches
  its effective sustained limit; otherwise a continuous anomaly can move P99
  upward before the one-minute containment timer elapses.
- Persisted baselines are capped at 256 verified summaries per project. An
  exited or PID-reused streaming control client loses its exact registration;
  uncertain ancestry or mixed group ownership remains recorded and simply
  disables enforcement.
- Healthy learning and circuit history survive a normal runtime rotation only
  when project, generation, effective manifest, runtime command, socket
  location/owner/device, generated configuration, and service identity remain
  unchanged. The old runtime PID, start identity, and socket inode remain
  provenance but never grant authority to the replacement runtime; any stable
  identity change resets the persisted state fail closed.
- An isolated Process Compose TUI process group must be made the controlling
  terminal's foreground group atomically during fork. Assigning only a new
  process group lets Darwin suspend the client for background terminal access
  before the Overview can draw; Rungrid restores its prior foreground group
  after the TUI exits.
- Circuit state becomes visible before the current containment stop and
  escalation finish. Explicit reset recovery therefore waits, within the
  configured shutdown timeout, for Process Compose to leave its stopping state
  before requesting a manual start.

## VALIDATION

- Unit and integration coverage passes for direct UDS routes/deadlines/size
  limits/redaction, defaults and unsafe overrides, effective-manifest and
  generated-config tampering, PID/start/parent identity, mixed PGIDs, external
  observation-only behavior, adaptive math and anomaly exclusion, baseline
  identity reset, incident retention/redaction, circuit history/reset, Darwin
  thread aggregation, socket alias safety, status persistence, and Platform's
  Aquarium/PostgreSQL/LocalStack boundary.
- Real Process Compose v1.120.0 E2E passes for lifecycle/session quiescence,
  CPU, memory, process-count, and thread-count emergency breaches, a
  TERM-resistant KILL escalation, three 500 ms/1 s/2 s restarts, fourth-breach
  circuits, explicit reset, manual/external process survival, inactive-runtime
  incident reporting, and exact cleanup.
- The explicit circuit-reset E2E reproduces the stop/start race and passes with
  the bounded containment-completion wait.
- Real sustained E2E passes after a one-minute healthy baseline and records a
  process-count containment 60 seconds after the anomalous tree is observed.
- A Darwin pseudo-terminal integration test proves that an isolated attach
  client owns the terminal foreground while it runs and that Rungrid restores
  the prior foreground process group afterward.
- Managed tab-shell tests prove that the exact generation shutdown marker
  terminates the child shell and releases its tab registration.
- Guard initialization prunes an exited or PID-reused control-client
  registration from any old scope only after its recorded process-start
  identity no longer matches; it never signals during this private-state
  cleanup.
- Immutable local E2E evidence run `20260812T145024Z-005243` passed in 161
  seconds at `tmp/2026-08-12/rungrid-headless-e2e/6` against the final
  validation tree.
- `make check`, `make lint`, `make vuln`, and `make release-snapshot` pass. The
  checks include vet, all unit/integration tests, race tests, sanitization,
  dependency licenses, native build, and Darwin/Linux amd64/arm64 cross-builds.
- The unchanged live Platform `.rungrid.yaml`, effective configuration, JSON
  plan, generated normalized manifest, and Process Compose configuration
  validate. The generated configuration contains Aquarium and the hidden guard
  but no PostgreSQL or LocalStack managed process entries.
- A graphical 30-second validation-only soak smoke passed after a same-
  generation runtime rotation with zero restarts or circuits, 0.033 percent
  average guard CPU, 29.6 MiB peak guard-plus-sampler RSS, 197.64 ms sampler
  p99, and 55.5 KiB of guard state. The Overview rendered live status/logs,
  and exact teardown left no managed tab shell, session, or attach client.
- `kit check --project` remains blocked by 11 pre-existing stale Kit-managed
  instruction findings outside this feature's source, docs, and delivery
  scope. The feature-specific Constitution changes were reviewed directly and
  the managed baseline markers remain intact.
- The required minimum 24-hour Platform soak is pending. Pull-request delivery
  stays blocked until it passes; no hosted checks or PR state are claimed.
- Soak run 6 was intentionally canceled after 30 minutes when the operator
  requested a stacked Versions full-screen correction. Its partial metrics are
  superseded and are not acceptance evidence; the soak must restart from the
  exact corrected commit.

## OUTCOME

The implementation removes all finite Process Compose CLI clients, makes
matching sessions release on teardown or runtime identity loss, and adds an
always-on generation-scoped guard for exact managed host trees and registered
streaming clients. Containment fails closed on any authority mismatch, prefers
Process Compose stop, safely escalates only captured identity-matching trees,
coordinates tab owners, persists redacted incidents, and exposes current and
inactive state through status.

Platform and Aquarium source are unchanged. The code, deterministic tests,
real Process Compose tests, repository gates, release snapshot, Platform
compatibility checks, soak runner, and upstream report draft are complete. The
feature remains in validation until the time-bound Platform soak passes.

## REPOSITORY MEMORY

Decision: created

Rationale: Runtime enforcement authority, adaptive thresholds, escalation,
session coordination, and circuit behavior are consequential cross-component
rationale that code and tests cannot preserve by themselves.

Artifacts:

- `docs/specs/runtime-resource-guard/SPEC.md`
- `docs/CONSTITUTION.md`
- `CLI_SPEC.md`

Constitution curation: the exact enforcement-authority invariant and the
managed/external ownership boundary were promoted as durable project-wide
constraints. Feature-specific thresholds, macOS discoveries, and validation
history remain in this spec and the testing reference.
