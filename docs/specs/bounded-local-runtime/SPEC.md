---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "complete"
feature:
  id: "bounded-local-runtime"
  slug: "bounded-local-runtime"
  dir: "bounded-local-runtime"
relationships:
  - type: builds_on
    target: rungrid-v1
references:
  - id: process-compose-compiler
    name: Process Compose compiler
    type: code
    target: internal/processcompose/compile.go
    relation: implements
    read_policy: must
    used_for: bounded application logs
    status: active
  - id: versions-monitor
    name: Versions monitor
    type: code
    target: internal/versions/versions.go
    relation: implements
    read_policy: must
    used_for: bounded monitoring overhead
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

Make a many-service local Rungrid workspace safe to leave running indefinitely
by bounding persisted logs and removing avoidable steady-state monitoring work.

## CONTEXT

- Process Compose application logs are persisted per process without rotation.
- Process Compose internal server and client diagnostics are written through
  its `-L` flag, which exposes a path but no rotation policy.
- `rungrid versions --watch` currently performs one Process Compose query,
  one `lsof` query per active service, and four Git commands per service every
  second, then redraws the terminal even when nothing changed.
- The Platform consumer runs twelve native applications alongside two shared
  external services and needs an indefinitely sustainable idle state.

## REQUIREMENTS

- Compile every managed process log with a 10 MB rotation threshold, one
  retained compressed rollover, and age retention derived from
  `runtime.log_retention`.
- Apply the same log policy to Rungrid maintenance processes.
- Prevent Process Compose internal diagnostic logs from growing on disk.
- Keep process-state polling responsive at one second.
- Run at most one batched listener query every five seconds.
- Refresh repository source metadata no more than every ten seconds, using one
  Git status query per repository after its worktree root is known.
- Redraw the human Versions table only when runtime or service state changes.
- Preserve one-shot and JSON Versions behavior, manifest compatibility,
  lifecycle behavior, and interactive log streaming.
- Keep LabCore and LabCore UI ownership outside Rungrid; their commands and
  watcher configuration are not changed here.
- Keep every affected handwritten source and test file at or below 300 lines.

### Non-goals

- Limiting intentional interactive `logs --follow`, sessions, or TUI streams.
- Changing service commands, health probes, restart policy, or port ownership.
- Persisting Process Compose internal diagnostics through a new daemon.
- Changing deployed or cloud runtimes.

## ACCEPTED PLAN

1. Activate the existing manifest log-retention contract in generated Process
   Compose configuration with fixed safe rotation defaults.
2. Route non-rotatable Process Compose internal diagnostic output to the OS
   null device while preserving application logs and command error reporting.
3. Add a Versions collector that caches repository state, batches listener
   discovery, and exposes material snapshot comparison for redraw suppression.
4. Add focused compiler, supervisor, source parser, listener parser, cache,
   and snapshot equality coverage.
5. Update CLI and manifest documentation, run the complete Rungrid validation
   contract, and record exact results here.

## DECISIONS

- Use a 10 MB threshold with one compressed rollover. Each individual log file
  stays at or below 10 MB and older rollovers are discarded deterministically.
- Send Process Compose `-L` diagnostics to the OS null device because the
  installed supported interface has no rotation settings for that stream.
  Application stdout/stderr remains available through rotated process logs.
- Keep the one-second Process Compose state query for responsive operator
  feedback; expensive listener and source discovery use independent caches.

## DISCOVERIES

- Process Compose applies rotation to managed process logs, but its `-L`
  internal diagnostic stream accepts only a path. Rungrid therefore preserves
  application output in rotated logs and sends the non-rotatable diagnostic
  stream to the operating system null device.
- A single `git status --porcelain=v2 --branch` supplies branch, abbreviated
  commit, and dirty state. After the worktree root is known, each refresh no
  longer needs separate branch, commit, root, and status processes.
- Listener discovery accepts a comma-separated PID set, so one `lsof` process
  can replace one subprocess per running service while retaining per-PID port
  attribution.
- Platform validation must preserve the manifest's sibling-workspace topology;
  a direct sibling validation directory proved all fourteen service entries
  without mutating the active Platform checkout.

## VALIDATION

- PASS: focused manifest, Process Compose compiler, supervisor, Versions cache,
  Git parser, listener parser, and material-snapshot tests.
- PASS: `make check`, including formatting, vet, all Go tests, the race suite,
  CLI sanitization, dependency licenses, native build, and four cross-builds.
  An initial package-parallel race run hit an existing one-second lifecycle
  fixture timeout under concurrent host load; the isolated test passed five
  consecutive race runs and the complete isolated `make check` passed.
- PASS: `make lint` reported zero findings and `make vuln` reported no
  reachable vulnerabilities.
- PASS: local headless end-to-end run
  `20260810T181126Z-091618` against Process Compose 1.120.0, with evidence at
  `tmp/2026-08-10/rungrid-headless-e2e/1`; all lifecycle assertions passed and
  the test recorded successful cleanup.
- PASS: `make release-snapshot` built Darwin and Linux archives for amd64 and
  arm64.
- PASS: the Platform consumer manifest compiled fourteen services and emitted
  a 10 MB, seven-day, one-backup compressed rotation policy for every native
  and maintenance process. Docker Compose rendered all profiles with all 27
  services on the `local` driver at `max-size=10m` and `max-file=1`.
- PASS: Platform's development workspace contract, Tab Config lifecycle,
  supervisor ownership validation, Compose rendering, and exact Rungrid plan
  generation.
- PASS: `git diff --check`, changed-diff Gitleaks scans in every affected
  repository, and `kit reconcile --all --output-only`; Kit audited 150
  eligible handwritten Rungrid source/test files with none above 300 physical
  lines.
- BASELINE: `kit check --project` reports nine pre-existing instruction-sync
  findings. The installed Kit currently ignores this repository's established
  non-numeric feature directories in `kit check --all`, so the living spec is
  reviewed against the existing V3 examples but not claimed as Kit-validated.

## OUTCOME

Every Rungrid-managed application and maintenance log now rotates at 10 MB,
retains one compressed rollover, and expires according to the manifest's
retention period. Process Compose internal diagnostics are not persisted
because that stream has no rotation contract; command failures remain visible
through bounded terminal and process output.

Versions watch mode retains one-second process-state responsiveness while Git
metadata refreshes at most every ten seconds, listener discovery is batched at
most every five seconds, and external health checks refresh at most every five
seconds. The table redraws only when material runtime or service state changes.
One-shot and JSON capture, lifecycle behavior, service commands, interactive
streams, and LabCore/LabCore UI watcher ownership remain unchanged.

## REPOSITORY MEMORY

Decision: created

Rationale: The split between rotated application logs, discarded
non-rotatable internal diagnostics, and independently cached monitoring data
is consequential operational rationale that code alone does not explain.

Artifacts:

- `docs/specs/bounded-local-runtime/SPEC.md`
- `docs/PROJECT_PROGRESS_SUMMARY.md`
- `CLI_SPEC.md`

Constitution curation: not required. The change is a feature-local operational
policy already captured by the manifest contract, CLI specification, and this
living spec; it does not establish a new project-wide constitutional boundary.
