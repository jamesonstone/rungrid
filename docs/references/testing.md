# Testing Reference

## Purpose

- Record the project's durable commands, suites, environments, automation, and evidence expectations
- Follow `rules/testing-and-environment-validation.md` for the mandatory cross-project testing and production-safety contract
- Keep feature-specific testing details in the current feature's `SPEC.md` VALIDATION and OUTCOME sections; legacy staged flows may still use `PLAN.md` or `TASKS.md`

## Code-Level Validation

| Layer | Command | PR workflow or check | Required | Notes |
| --- | --- | --- | --- | --- |
| Formatting | `make fmt-check` | `CI / test` | yes | `gofmt` cleanliness without mutation |
| Static analysis | `make vet` | `CI / test` | yes | all Go packages |
| Unit and integration | `make test` | `CI / test` | yes | schema, merge, workspace boundaries, state, argv, environment, generation, lifecycle journal and recovery, resource-guard authority/limits/circuits, repository-maintenance and filesystem-reconciliation proof, onboarding, and golden contracts |
| Race | `make test-race` | `CI / test` | yes | all Go packages, with the opt-in E2E skipped |
| Contract sanitization | `make sanitize` | `CI / test` | yes | rejects personal/workspace identifiers and unsafe absolute paths in `CLI_SPEC.md` |
| Lint | `make lint` | `CI / lint` | yes | `golangci-lint` |
| Vulnerability | `make vuln` | `CI / vulnerability` | yes | reachable Go vulnerability scan |
| Dependency licenses | `make license` | `CI / test` | yes | every dependency module archive contains license or notice material |
| Cross-build | `make build-cross` | `CI / test` | yes | Darwin/Linux on amd64/arm64 with CGO disabled |
| Release snapshot | `make release-snapshot` | `CI / release-snapshot` | yes | GoReleaser archives and SBOM when Syft is available; signing is tag-only |

## High-Level Suites

| Suite | Type | Environment | Command | Automation | Evidence |
| --- | --- | --- | --- | --- | --- |
| Headless lifecycle | end-to-end | local macOS or Linux | `tests/end-to-end/local/run.sh` | `CI / end-to-end` and local milestone | `tmp/<UTC-date>/rungrid-headless-e2e/<run-number>/` and a 14-day Actions artifact; mixed-service and tab-only workspaces exercise one-shot hook ordering, while repository maintenance proves default-worktree fast-forward with service pause/resume |
| Resource guard soak | live integration | local macOS or Linux | `tests/live-integration/resource-guard-soak/run.sh <workspace> <duration>` | manual before guard delivery | bounded redacted decision/overhead samples plus final acceptance JSON; minimum 24 hours, preferably 72, with no competing Rungrid runtime |
| Graphical Warp workspace | end-to-end | local macOS | controlled operator smoke after installation | manual | `tmp/<UTC-date>/rungrid-warp-smoke/<run-number>/` when implemented |
| Production | end-to-end | production | not applicable | none | Rungrid is a local CLI, not a deployed service |

## Environment Preflights

- Local code-level checks require the Go version in `go.mod`.
- Filesystem-reconciliation process tests additionally require `lsof`; they
  skip only those live-process cases when it is unavailable, while fake-runner
  ownership and refusal coverage remains mandatory.
- The headless suite additionally requires Process Compose
  `>=1.120.0,<2.0.0`, a Unix-like host, and permission to create temporary Unix
  sockets and child processes. It uses temporary workspace and XDG roots and a
  10-second service-state deadline.
- Resource-guard E2E uses temporary projects and deliberately low validated
  thresholds. It exercises CPU, memory, process, and thread containment without
  approaching host saturation, three restarts, fourth-breach circuit opening,
  explicit reset, exact cleanup, and survival of manual/external processes.
- The Platform soak uses Platform only as a compatibility and normal-load
  fixture; it does not edit Platform or Aquarium sources. Acceptance requires
  zero healthy-service restarts/circuits, emergency containment within five
  seconds, sustained containment at 60 seconds plus or minus two samples,
  guard and sampler below one percent of one core on average and five percent
  at p99, less than 64 MiB RSS, sampler p99 below 250 milliseconds, state below
  10 MiB for 14 services, and no mutation outside exact authority.
- The Warp smoke requires macOS, Warp, zsh, Process Compose, and explicit
  acceptance of user-visible windows and tabs.
- No deployed or production environment exists for this CLI.

## Credentials And Test Data

- No credentials or remote test data are required.
- Tests use only generic multi-repository fixtures and temporary local
  processes. The local E2E test shuts down Process Compose and relies on the Go
  test temporary-directory cleanup after asserting the runtime marker is
  removed and the lifecycle journal is inactive.

## Evidence And Retention

- `tmp/` is ignored. The headless runner atomically reserves a positive run
  number and writes redacted `output.txt` plus `result.json`.
- `output.txt` is the sole full-output artifact. Evidence runners never replay
  it unboundedly to stdout or stderr; terminal output contains only bounded
  status metadata and the exact evidence path.
- Evidence output has a 64 MiB hard maximum with no unlimited mode. The limit
  is enforced before bytes reach disk. Overflow terminates and waits for the
  child process group, preserves the bounded artifact, records output-limit
  metadata in `result.json`, and fails with exit code 74.
- `tests/RUN_STATUS.md` is curated at handoff milestones and is never updated
  automatically by tests or CI.
- The CI end-to-end job installs the pinned Process Compose 1.120.0 Linux
  archive after checking its published SHA-256 digest, then uploads the run
  directory for 14 days even when the suite fails.

## Automation And Fallbacks

- `.github/workflows/ci.yml` maps every required code-level command above to a
  pull-request job.
- `.github/workflows/release.yml` builds archives and SBOMs and signs checksums
  for tags. A tag is not authorized until review, merge, license selection,
  hosted checks, and controlled smoke validation complete.
- For a local milestone, run `make check`, `make lint`, `make vuln`,
  `tests/end-to-end/local/run.sh`, `make release-snapshot`, Platform validation,
  and the resource-guard soak in that order.

## Known Gaps

- A controlled Warp graphical smoke has not been run for this candidate.
- Syft and Cosign were unavailable in the local environment; CI configuration
  covers SBOM/signing but no hosted run is claimed before the PR reports it.
- No repository license has been selected, so release publication is blocked.
- The Bubble Tea dependency graph is pinned past the first licensed
  `go-localereader` revision because its tagged v0.0.1 module archive contains
  no license file.
