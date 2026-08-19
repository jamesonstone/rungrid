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
| Race | `make test-race` | `CI / test` | yes | all Go packages |
| Contract sanitization | `make sanitize` | `CI / test` | yes | rejects personal/workspace identifiers and unsafe absolute paths in `CLI_SPEC.md` |
| Lint | `make lint` | `CI / lint` | yes | `golangci-lint` |
| Vulnerability | `make vuln` | `CI / vulnerability` | yes | reachable Go vulnerability scan |
| Dependency licenses | `make license` | `CI / test` | yes | every dependency module archive contains license or notice material |
| Cross-build | `make build-cross` | `CI / test` | yes | Darwin/Linux on amd64/arm64 with CGO disabled |
| Release snapshot | `make release-snapshot` | `CI / release-snapshot` | yes | GoReleaser archives and SBOM when Syft is available; signing is tag-only |

## High-Level Suites

| Suite | Type | Environment | Command | Automation | Evidence |
| --- | --- | --- | --- | --- | --- |
| Graphical Warp workspace | smoke | local macOS | controlled operator smoke after installation | manual | `tmp/<UTC-date>/rungrid-warp-smoke/<run-number>/` when implemented |
| Production | end-to-end | production | not applicable | none | Rungrid is a local CLI, not a deployed service |

## Environment Preflights

- Local code-level checks require the Go version in `go.mod`.
- Filesystem-reconciliation process tests additionally require `lsof`; they
  skip only those live-process cases when it is unavailable, while fake-runner
  ownership and refusal coverage remains mandatory.
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

- `tmp/` is ignored.
- `tests/RUN_STATUS.md` is curated at handoff milestones and is never updated
  automatically by tests or CI.

## Automation And Fallbacks

- `.github/workflows/ci.yml` maps every required code-level command above to a
  pull-request job.
- `.github/workflows/release.yml` builds archives and SBOMs and signs checksums
  for tags. A tag is not authorized until review, merge, license selection,
  hosted checks, and controlled smoke validation complete.
- For a local milestone, run `make check`, `make lint`, `make vuln`, and
  `make release-snapshot` in that order.

## Known Gaps

- A controlled Warp graphical smoke has not been run for this candidate.
- Syft and Cosign were unavailable in the local environment; CI configuration
  covers SBOM/signing but no hosted run is claimed before the PR reports it.
- No repository license has been selected, so release publication is blocked.
- The Bubble Tea dependency graph is pinned past the first licensed
  `go-localereader` revision because its tagged v0.0.1 module archive contains
  no license file.
