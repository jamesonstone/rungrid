# Rungrid v1 implementation record

Status: v1 candidate implemented; workspace lifecycle extension locally validated

## Purpose

This document preserves the rationale and delivery history for Rungrid v1. The
authoritative product and command contract is [`CLI_SPEC.md`](../../../CLI_SPEC.md).
This record intentionally does not duplicate that contract.

## Accepted direction

Rungrid extracts a reproducible development workspace from any particular
repository. A project manifest describes infrastructure and application
services. Process Compose owns process lifecycle through a detached,
project-scoped runtime. Warp is the sole graphical adapter in v1; the same
lifecycle remains usable without a graphical terminal on macOS and Linux.

The visible workspace has three stable layers:

1. an Overview tab attached read-only to Process Compose, including selectable
   process logs;
2. a live Versions tab for lifecycle, listener, and source-control state; and
3. one tab for each configured tab-owned service, in manifest order.

Infrastructure normally uses workspace activation and starts during `up`.
Applications normally use tab activation and remain disabled until a service
tab acquires an exclusive, generation-scoped session. Process Compose still
supervises the underlying process; the tab foreground follows its logs and owns
the right to start and stop it. This separation keeps lifecycle state truthful
in Overview while preserving direct per-tab control.

## Material decisions

- The manifest uses `rungrid/v1`; machine output uses `rungrid/output/v1`.
- Project identity is a persisted slug plus random suffix. Absolute checkout
  paths are runtime inputs, never identity material.
- Structured process arguments and the interactive trigger are separate. This
  permits a familiar trigger while retaining an exact supervised command.
- Process Compose is required in the range `>=1.120.0,<2.0.0` because v1 relies
  on disabled processes, daemon lifecycle, the Unix-socket client, and remote
  read-only TUI behavior.
- Generated files live in project-scoped XDG state, carry ownership and content
  hashes, use restrictive permissions, and are replaced atomically.
- Warp tab files are derived artifacts. Regeneration may replace only artifacts
  whose ownership metadata and prior hash still match.
- External services are observed but never started or stopped by Rungrid.
- The Overview is the Process Compose TUI. Rungrid does not own a competing
  dashboard or a separate all-logs tab in v1.
- Additional graphical terminal adapters and command-free multi-pane workspaces
  are deliberately outside v1.
- Coding-agent onboarding is a read-only instruction surface, not a second
  configuration or discovery engine. `instructions` and its `agent-start`
  alias emit one self-contained brief that teaches an agent to inspect the
  selected consumer projects and author the portable manifest under each
  repository's own rules.
- Help is a first-class operator surface. It borrows Kit's presentation
  grammar—ASCII identity, workflow grouping, terminal-aware color, and stable
  plain output—while using Rungrid's own lifecycle and ownership model. Color
  is never semantic and explicit color suppression remains authoritative.
- Default-branch history rewriting is excluded from implementation lanes. The
  neutral contract is delivered normally; historical objects and archived pull
  request references may retain earlier content.

## Workspace lifecycle extension

Multi-repository workspaces are a core portability requirement. A manifest may
live in one repository while describing sibling repositories under a common
relative workspace root. Infrastructure setup and final teardown are ordered,
one-shot workspace lifecycle operations rather than long-running Process
Compose services.

This extension is delivered in two review lanes. Rungrid issue `#10` owns the
neutral manifest, runtime, recovery, command, test, and documentation changes.
Only after those prerequisites are reviewable will a separate consumer issue,
branch, specification, and ready pull request adopt the feature. Rungrid source
and fixtures remain free of consumer names, paths, and commands.

Material decisions for this extension:

- The manifest directory and workspace root are distinct. `workspace.root`
  defaults to `.`, is relative to the manifest directory, may name an ancestor,
  and is resolved with symlink-aware containment checks.
- The source manifest establishes the workspace boundary before imports are
  traversed. Imported fragments cannot redefine it; the adjacent ignored local
  overlay remains anchored to the manifest directory.
- Services, Compose files, and environment-provider paths resolve from the
  workspace root. Stable identity and deterministic generation use only the
  relative declaration; absolute resolved paths remain machine-local runtime
  data.
- `lifecycle.before_up` and `lifecycle.after_down` are sequential structured
  argument vectors. They reuse environment providers and redaction, but are
  never emitted as Process Compose processes or Warp tabs.
- The lifecycle journal records teardown intent before the first prerequisite
  mutates external state. Required teardown survives prerequisite failure,
  supervisor startup failure, signals, process crashes, and a missing runtime
  record.
- One project-scoped lifecycle lock serializes `up`, `down`, recovery, and
  uninstall. Service-level `start` and `stop` preserve their existing scope and
  never invoke global hooks.
- Cleanup attempts every teardown command, retains `cleanup-required` on any
  failure, and must complete under the recorded generation before a different
  generation may start.
- Exact PID and socket identity remain hard safety gates. A stale or ambiguous
  runtime is not permission to rerun prerequisites, signal a process, delete a
  socket, or discard teardown state.

The implementation sequence is contract and manifest loading, deterministic
planning, journal and executor, lifecycle command integration, recovery and
uninstall behavior, then generic tests and delivery validation. The consumer
lane follows only after the neutral lane is complete.

### Conclusively dead runtime recovery

Issue `#29` addresses an operator-visible crash-recovery gap discovered with a
real multi-repository Platform manifest. The lifecycle journal can correctly
retain `active` state and teardown intent after Process Compose exits, while
the dead supervisor leaves `runtime.json` behind and removes its Unix socket.
The prior reconciliation path rejected the stale PID before it could satisfy
the journal's durable cleanup obligation, so every subsequent `rungrid up`
failed without running `after_down` or starting a replacement runtime.

Accepted behavior:

- Recovery occurs only while the project lifecycle lock is held and only after
  the private runtime record matches the selected project, generation, and
  lifecycle journal identity.
- The recorded PID must not exist and the exact expected runtime socket path
  must be absent. Rungrid does not treat PID reuse, identity mismatch, a live
  Process Compose process, or any present socket as recoverable.
- Rungrid immediately re-reads the private record and repeats the PID and socket
  absence checks before removing only `runtime.json`. It never signals a PID or
  removes a socket during stale-record recovery.
- The journal runtime identity is cleared, required `after_down` commands run
  before any prerequisite is repeated, and ordinary startup may then create a
  fresh runtime. Any mismatch or cleanup failure remains visible and
  fail-closed.

Accepted implementation plan:

1. Add a supervisor operation that retires only an unchanged, project-owned
   runtime record whose recorded process and expected socket are absent.
2. Invoke that operation from lifecycle journal reconciliation after journal
   and runtime identities match, then persist a cleared journal runtime
   identity through the existing cleanup path.
3. Add focused supervisor refusal tests plus lifecycle reconciliation coverage
   proving teardown completes and startup may proceed after a dead runtime.
4. Validate the fix against the real Platform manifest without modifying the
   consumer repository or weakening live and ambiguous runtime protections.

## Declared repository roots

Status: implemented and locally validated in `GH-14`.

The common workspace root added by issue `#10` makes sibling repositories
reachable, but service paths still address that entire boundary directly. A
multi-repository manifest needs stable logical repository names so service
configuration can remain local to the repository that owns its command,
Compose file, environment providers, and source-control state.

Accepted decisions:

- `workspace.root` remains the outer import, lifecycle, and execution boundary.
  Replacing it would break the crash-safe lifecycle contract and existing
  manifests.
- `repositories` is an optional map of logical names to paths relative to the
  workspace root. The reserved implicit name `workspace` always identifies the
  workspace root and preserves existing behavior.
- A service's optional `repository` defaults to `workspace`.
  `working_directory`, Compose files, and environment-provider paths resolve
  inside the selected repository and may not escape it after symlink
  resolution.
- Declared repository roots must be relative, existing, distinct directories
  within `workspace.root`. The ignored local overlay may replace a declared
  relative path for a different checkout layout without changing service
  configuration.
- Plans and normalized manifests contain only logical names and relative
  declarations. Runtime execution resolves the selected root against the
  machine-local absolute workspace root.
- Versions derives Git state from each service's selected repository context.
  Doctor reports every valid declared root and source-control availability.
- Onboarding detects the nearest Git root for discovered services. Selected
  sibling repositories become declarations; unselected siblings do not enter
  the manifest.
- Imports and lifecycle commands retain their existing workspace-root
  semantics. Repository declarations do not expand the outer boundary, alter
  project identity, or give uninstall authority over source repositories.

Implementation proceeds through contract and schema changes, repository-root
resolution and validation, execution-path propagation, planning and operator
surfaces, onboarding discovery, focused tests, then complete repository
validation and memory curation.

## Delivery record

Issue `#12` adds the coding-agent instruction surface to the existing `GH-10`
branch and pull request at the user's direction. It retains separate issue and
commit traceability without creating a second branch or review candidate.

Issue `#13` adds the CLI help redesign to the same branch and pull request at
the user's direction, with its own scoped commits and validation evidence. Kit
is read-only design evidence; no cross-repository change is part of this lane.

Issue `#14` adds named repository roots as a separate review candidate on
`GH-14`. It refines the common workspace boundary delivered by issue `#10`
without changing import or lifecycle ownership.

The accepted plan proposed a separate issue, branch, and pull request for each
stage. The repository implementation was completed as one dependency-ordered
candidate on `GH-3`; it must not be represented as four independently reviewed
deliveries. A reviewer may require the candidate to be split before merge. The
consumer-repository cutover remains a separate delivery after a release
candidate is published.

### Stage 1: neutral contract and core

Status: implemented in `GH-3`.

- Replace the product contract with neutral version 2.
- Establish the Go command surface, manifest model and merge rules, validation,
  project identity, XDG state, ownership-safe atomic generation, planning, and
  doctor checks.
- Add sanitization and repository contract tests.

### Stage 2: runtime and manifest-owned environment

Status: implemented in `GH-3`.

- Compile Process Compose configuration and implement detached runtime identity,
  native/Compose wrappers, dependency and health semantics, environment
  providers, sessions and locks, and lifecycle commands.

### Stage 3: Warp, Versions, and service-tab ownership

Status: implemented in `GH-3`.

- Generate ordered Warp Tab Configs, implement read-only Overview, Versions,
  managed zsh trigger interception, tab registration, safe reopening, and
  uninstall.

### Stage 4: onboarding and release candidate

Status: implemented in `GH-3`; release publication is gated.

- Add resumable interactive onboarding and non-interactive initialization,
  documentation, CI, completions, release packaging, provenance, and release
  candidate validation.

### Stage 5: legacy-workspace dogfood and cutover

Status: implemented and locally validated on `GH-10`. Consumer adoption remains
a separate repository lane and is not part of this branch.

- Express the legacy workspace as a manifest, replace active wrappers without
  duplicating service inventory, retain isolated rollback material for one
  release cycle, and prove graphical/headless parity before stable release.

## Implementation discoveries

- Current Warp Tab Configs are TOML files. Generated files therefore use the
  ordered names `00_overview.toml`, `01_versions.toml`, and manifest-ordered
  service TOML files.
- Unix-domain socket path limits can be exceeded by legitimate XDG roots.
  Process Compose binds the socket through a short relative path from the
  generation directory while the runtime record retains the verified absolute
  identity.
- Process Compose commands are generated as Rungrid wrapper invocations. User
  argument vectors are never interpolated into Process Compose's shell command
  field.
- `up --headless` creates a headless effective generation even when the source
  manifest selects Warp, so graphical files are not an accidental side effect.
- The Go toolchain minimum is 1.25.12 because the earlier candidate produced
  reachable standard-library vulnerability findings.
- Bubble Tea's original transitive `go-localereader` tag predates that module's
  MIT license file. The dependency graph pins the first licensed upstream
  revision and the repository gate rejects dependency archives without license
  or notice material.
- Process Compose can write structured diagnostics to stderr before returning a
  valid JSON response on stdout, notably on a minimal Linux host without an XDG
  configuration directory. The client keeps these streams separate so
  diagnostics cannot corrupt machine-readable state while failed command
  output remains redacted.
- The source manifest must establish and validate the workspace root before an
  import path can be resolved. Imports and the ignored local overlay are then
  merged without permitting either input to change that boundary.
- A valid tab-only Process Compose generation has every process disabled. The
  detached runtime may still be ready even though no managed service is running
  until a session explicitly starts one.
- Lifecycle cleanup cannot depend on a runtime record. A durable teardown
  obligation must remain actionable after supervisor startup fails or its
  runtime identity disappears.
- Replacing an advisory-lock file while a process is waiting can split mutual
  exclusion across inodes. The lock holder therefore validates the acquired
  inode and reacquires when the path was replaced.
- Environment-provider paths receive the same execution-time, symlink-aware
  workspace boundary check as static service and lifecycle paths.
- A headless plan must derive its artifact list from the effective terminal
  mode, not from the graphical mode in source configuration.
- Process Compose 1.120 rejects the intuitive `warning` log level after
  generation. Rungrid now validates the exact accepted levels in both its
  semantic validator and published schema so this fails before lifecycle
  mutation.
- A top-level executable check is insufficient for common structured command
  vectors such as `env ... direnv exec . make dev`. Planning and Doctor now
  expose each supported wrapper layer plus the tab trigger without attempting
  to parse opaque shell command strings.
- The local command symlink recipe must fail closed when privilege elevation or
  link replacement fails. It verifies the final link target before reporting
  success, matching the existing local `kit`, `yp`, and `kp` command layout.
- An agent handoff must work before a manifest exists. The command therefore
  treats the selected manifest and project paths as inert prompt data, performs
  no discovery or mutation itself, and emits the same content through both the
  human and versioned JSON surfaces. Consumer repository rules and explicit
  lifecycle authorization remain authoritative.
- A common workspace boundary is insufficient to describe service ownership in
  a multi-repository checkout. Stable logical repository names preserve
  portable configuration while runtime path resolution remains machine-local.
- Onboarding may discover runnable sibling Git repositories, but inferred
  native sibling services require explicit selection before their repository
  declarations enter the manifest.

## Validation record

Local validation completed on macOS with Process Compose 1.120.0:

- `make check`: formatting, vet, unit/integration tests, race tests, contract
  sanitization, native build, and Darwin/Linux amd64/arm64 builds.
- `make lint`: zero `golangci-lint` findings.
- `make vuln`: no reachable vulnerabilities reported by `govulncheck`.
- Repository-root tests cover relative declarations, local overlay replacement,
  duplicate roots, unknown references, lexical and symlink escapes, repository-
  aware Compose shutdown and Git versions, Doctor checks, planning output, and
  selected sibling discovery.
- Focused command and prompt-builder tests prove that `instructions` and
  `agent-start` are equivalent, root help names the alias, JSON uses the
  versioned `AgentInstructions` envelope, and path hints remain encoded data.
- Help presentation tests pin the complete plain root output, require every
  visible command to appear in exactly one workflow group, exercise root and
  subcommand help through both invocation forms, prove terminal ANSI styling,
  and prove `--no-color` and `NO_COLOR` suppression. Kit's help implementation
  and live terminal output were inspected as design evidence without modifying
  that repository.
- `tests/end-to-end/local/run.sh`: real detached Process Compose lifecycle,
  runtime identity tamper rejection, active-generation protection, exclusive
  tab sessions, stop/restart, cleanup, and immutable ignored evidence.
- `goreleaser check` and `make release-snapshot`: release configuration and all
  four target archives validated locally. Signing and SBOM creation are covered
  by CI/release configuration; the local snapshot skips an unavailable tool.
- The real mixed-service and tab-only lifecycle suites pass with Process
  Compose 1.120.0 after the repository-root change.
- Fresh-install initialization, validation, planning, generation, and a real
  Process Compose dry run were exercised in temporary directories.
- A Linux/arm64 container reproduction with Process Compose 1.120.0 exercised
  workspace startup, tab-session ownership, `Disabled` to `Running` to
  `Completed` transitions, interrupt handling, status JSON, and shutdown.
- Workspace-root and lifecycle tests cover sibling repositories, symlink
  escapes, overlay replacement, exact argument vectors, timeouts, cancellation,
  redaction, lock replacement, journaling, rollback, missing-runtime cleanup,
  retry and no-op teardown, and uninstall refusal while cleanup is required.
- Issue `#29` adds supervisor refusal coverage for live PIDs and present socket
  paths plus lifecycle reconciliation coverage for matched and unmatched stale
  journal identities. `make check`, `make lint`, and `make vuln` pass after
  integration with current `main`.
- The real Platform `.rungrid.yaml` reproduced the stale PID with an `active`
  journal, absent recorded process, and absent socket. The candidate completed
  required teardown, reran prerequisites, started generation
  `0ee1e54fd63b6582123d`, and reported an active verified runtime; the ordinary
  installed command then reused that runtime successfully. Platform source was
  not modified, and `--no-open` avoided a graphical side effect.
- Clean-source headless lifecycle evidence for issue `#29` is recorded as run
  `20260810T202921Z-024383` at merge commit `0ad699d`; the same commit passed a
  release snapshot for all four supported OS/architecture targets.
- Real mixed-service and tab-only headless runs prove prerequisites precede the
  supervisor, teardown follows it, repeated `up` does not repeat prerequisites,
  and Process Compose remains the managed-service lifecycle authority.
- A follow-up real headless run after Process Compose log-level and structured
  executable discovery changes passed both mixed-service and tab-only suites;
  clean-source ignored evidence is recorded as run
  `20260803T152616Z-051963` at commit `5d62241`.
- The coding-agent instruction command was followed by another clean-source
  mixed-service and tab-only headless run. Ignored evidence is recorded as run
  `20260804T120123Z-084621` at commit `534cc8c`; the release snapshot also built
  all four archives from that exact commit.
- The help redesign was followed by a clean-source mixed-service and tab-only
  headless run. Ignored evidence is recorded as run
  `20260804T121733Z-014138` at commit `a4ffcef`; the release snapshot built all
  four archives from the same commit.
- `make build` was exercised under a temporary writable prefix and produced an
  exact `bin/rungrid` symlink. The existing `/usr/local/bin/rungrid` link uses
  the same canonical-repository layout as `kit`, `yp`, and `kp`; no
  administrator-authenticated link replacement was claimed from the worktree.

The pull-request workflow includes the same Process Compose version and uploads
immutable run evidence. Hosted checks exposed and now cover Linux socket-path
reporting and separation of Process Compose diagnostics from JSON responses.
The graphical Warp smoke, local SBOM generation, action-workflow linting,
signing, published release, and consumer-workspace parity are not observed and
are not passing claims. Their required local tools or controlled graphical
environment were unavailable where applicable.

## Outcome

Rungrid v1 is implemented as a review candidate with the neutral contract,
portable multi-repository workspace boundary, crash-safe one-shot lifecycle,
Process Compose runtime, Warp/headless presentation, onboarding, tests, CI, and
release packaging. A conclusively dead Process Compose runtime can now retire
only its unchanged journal-matched record, complete required teardown, and
restart without weakening live-process or socket safety. A read-only agent
instruction surface can now hand the portable integration contract and
selected path hints to a coding agent before the manifest exists. Root and
subcommand help now expose the same contract in a Rungrid-specific,
workflow-grouped terminal presentation with a stable plain fallback. The
neutral implementation is ready for a separately owned consumer cutover lane;
this outcome does not claim consumer parity.

Default-branch history was not rewritten:
repository guardrails prohibit force pushing or mutating the default branch,
and archived pull-request refs or direct object URLs could retain old content
even after a branch rewrite. Release publication remains gated on review,
merge, a license decision, hosted checks, and controlled Warp validation. The
consumer migration remains a separate post-RC change.
