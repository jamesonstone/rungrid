# Project progress summary

## Project intent

Rungrid turns a portable workspace manifest into one authoritative Process
Compose lifecycle, usable through ordered Warp tabs or headlessly on macOS and
Linux. This file is the compact repository-memory index; follow its pointers
before loading broader history.

## Global constraints

- [`CONSTITUTION.md`](CONSTITUTION.md) is the durable project contract.
- [`../CLI_SPEC.md`](../CLI_SPEC.md) is the authoritative product and command
  contract.
- [`specs/rungrid-v1/SPEC.md`](specs/rungrid-v1/SPEC.md) records v1 rationale,
  implementation discoveries, validation, and delivery gates.
- Process Compose is the managed-service lifecycle authority; terminal
  presentation never maintains a competing service state. Rungrid's journal is
  authoritative for one-shot workspace prerequisites and teardown.
- Release and consumer-workspace changes remain separate mutation boundaries.

## FEATURE PROGRESS TABLE

| Feature | Source | Highest completed artifact | Status |
| --- | --- | --- | --- |
| `rungrid-v1` | `docs/specs/rungrid-v1/SPEC.md` | Integrated implementation on `GH-3` plus locally validated workspace-root and lifecycle extension on `GH-10`. | Ready for pull-request review; graphical smoke, license selection, release publication, and consumer cutover remain gated. |
| `repository-maintenance` | `docs/specs/repository-maintenance/SPEC.md` | Validated implementation on `GH-20`. | Ready for pull-request review; hosted checks remain. |
| `repository-reconcile` | `docs/specs/repository-reconcile/SPEC.md` | Locally validated implementation on `GH-23`. | Ready for stacked pull-request review against `feat/up-sync-flag`; hosted checks remain. |

## FEATURE SUMMARIES

### Rungrid v1

- **STATUS**: review candidate
- **INTENT**: Replace repository-owned development-workspace scripts with a
  neutral manifest and one truthful Process Compose lifecycle.
- **IMPLEMENTED**: Portable manifest processing, safe generated state,
  multi-repository workspace roots, crash-safe one-shot prerequisites and
  teardown, workspace/tab/external activation, detached managed-service
  lifecycle, exclusive sessions, ordered Warp tabs, headless operation,
  Versions, onboarding, tests, CI, and release packaging.
- **OPEN ITEMS**: Review and merge, choose a license, observe hosted checks,
  perform a controlled Warp smoke, publish the release candidate, then deliver
  consumer migration separately. The protected-history rewrite is excluded.
- **POINTER**: `docs/specs/rungrid-v1/SPEC.md`

### Repository maintenance

- **STATUS**: review candidate
- **INTENT**: Safely fast-forward configured repositories' local default
  branches and reclaim only independently proven obsolete linked worktrees.
- **IMPLEMENTED**: Manifest-scoped repository metadata, strict dry-run and
  typed reports, expected-OID synchronization, cooperative service
  pause/resume, CLI-authorized Process Compose maintenance jobs, exact
  GitHub/process/worktree removal proof, and real headless lifecycle coverage.
- **OPEN ITEMS**: Review the ready pull request and observe hosted checks.
- **POINTER**: `docs/specs/repository-maintenance/SPEC.md`

### Repository reconcile

- **STATUS**: review candidate
- **INTENT**: Reconcile one physical clone or a recursive repository tree while
  preserving active primary work and keeping manifest synchronization unchanged.
- **IMPLEMENTED**: Filesystem discovery and common-directory deduplication,
  live-origin default proof, expected-OID synchronization, decomposed primary
  activity evidence, guarded WIP/stash/switch recovery, native merged-worktree
  cleanup, lifecycle ownership coordination, typed reports, and four optional
  unattended coding-agent adapters.
- **OPEN ITEMS**: Review the stacked ready pull request, observe hosted checks,
  and retarget it to the default branch after the `up --sync` base pull request
  merges without rebasing or force-pushing.
- **POINTER**: `docs/specs/repository-reconcile/SPEC.md`

## Current implementation

- The Go CLI implements the complete documented command surface, strict
  manifest merge and validation, XDG state, deterministic generation,
  symlink-aware workspace boundaries, lifecycle journaling and recovery,
  Process Compose supervision, exact native and Compose execution, exclusive
  sessions, Warp/headless presentation, Versions, repository maintenance,
  onboarding, and uninstall.
- Unit, integration, race, golden, contract, fake-executable, and real
  Process Compose end-to-end suites cover the repository-owned boundaries.
- GitHub Actions covers code-level gates, verified Process Compose lifecycle
  evidence, cross-builds, vulnerability analysis, release snapshots, SBOMs,
  and tag signing.

## Validation state

- Local formatting, vet, tests, race tests, sanitization, lint,
  vulnerability analysis, Darwin/Linux cross-builds, real Process Compose
  lifecycle, project contract validation, and release snapshot are the required
  handoff gates.
- Hosted workflow results, a controlled macOS Warp smoke, checksum signing,
  SBOM publication, and release installation are observed only after the
  corresponding PR/tag actions run.
- Production validation is not applicable because Rungrid is a local CLI.

## Last updated

- 2026-08-09: Implemented and locally validated filesystem repository
  reconciliation, stale-primary safety gates, and agent adapters.
- 2026-08-06: Implemented and locally validated default-branch synchronization,
  fail-closed worktree pruning, and service-aware maintenance lifecycle.
- 2026-08-03: Implemented and locally validated portable workspace roots and
  crash-safe lifecycle hooks; kept consumer adoption as a separate lane.
- 2026-08-01: Implemented and locally validated the integrated Rungrid v1
  review candidate; recorded remaining privacy, delivery, release, graphical,
  and consumer-migration gates.
