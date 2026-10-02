---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "delivery"
feature:
  id: "terminal-resume"
  slug: "terminal-resume"
  dir: "terminal-resume"
relationships:
  - type: builds_on
    target: rungrid-v1
  - type: builds_on
    target: runtime-resource-guard
references:
  - id: cli-contract
    name: Rungrid CLI specification
    type: documentation
    target: CLI_SPEC.md
    relation: constrains
    read_policy: must
    used_for: public command, lifecycle, and output contracts
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
    name: Terminal resume issue
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/52
    relation: tracks
    read_policy: must
    used_for: delivery ownership
    status: active
skills: []
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Restore a workspace after its terminal presentation was lost, typically when
Warp restarts and closes the Rungrid windows. The detached Process Compose
runtime usually survives, so the operator needs the windows back and any
service that stopped unintentionally restarted, without a fresh start.

## CONTEXT

- `rungrid up` already reuses an active generation and opens Warp. It still
  requires every workspace service to become ready, and it always opens a new
  full window, so it is not a safe "restore" command.
- `rungrid open` opens windows but checks no services.
- Process Compose status alone cannot tell an operator-stopped service from a
  crashed one.

## REQUIREMENTS

- `rungrid resume` reuses a verified live runtime, never reruns lifecycle hooks,
  and never restarts a running service.
- It restarts managed `workspace` services that are not running, except
  services the operator stopped with `rungrid stop`.
- It reopens only windows that did not survive.
- It recovers a runtime that died without `rungrid down` through `up`, and it
  refuses a workspace that was never started or was shut down on purpose.

### Non-goals

- Restoring arbitrary user tabs or Warp session state.
- Starting `tab` services, whose lifecycle a tab owns.
- Changing `up`, `open`, or `start` semantics.

## DECISIONS

- **Stop intent is persisted.** `rungrid stop` writes
  `stopped/<generation>-<service>` in project state, `rungrid start` removes it,
  and a fresh runtime start clears the directory. Generation ids are
  content-addressed, so the same manifest after `down`/`up` reuses the id; the
  fresh-start clear prevents stale intents from surviving that cycle.
- **Window liveness uses existing registrations.** The Overview tab runs
  `attach --read-only`, which registers a resource-guard control client, and
  service tabs register in `tabs/`. A live attach client or tab registration
  means a Rungrid window survived, and resume opens only absent service tabs.
  The Versions tab has no registration, so it cannot be detected on its own.
  An operator-run `rungrid attach` in another terminal also counts as live;
  `--force-open` covers that case.
- **Recovery delegates to `up`.** A dead runtime with an active journal means
  the operator never ran `down`, so `up` reconciles the journal, reruns
  prerequisites, and opens Warp. An inactive or missing journal is refused
  with RG1151 rather than silently starting the workspace.
- **Restart failures are partial.** Each failed restart, such as an open
  resource circuit, is reported per service. Resume still opens the windows and
  exits with partial failure (RG1149). It never resets a circuit.

## VALIDATION

- `go test ./internal/lifecycle ./cmd .` covers restart selection, window
  planning, stop-intent record, clear, and fresh-start clearing, plus help and
  the CLI contract.
- `make check` and `make lint` are the required gates.
- The graphical Warp smoke stays manual (see `docs/references/testing.md`).

## OUTCOME

Operators run `rungrid resume` after Warp restarts. A live runtime keeps
running; crashed workspace services restart, operator-stopped services stay
stopped, and the Warp workspace reopens only when no Rungrid window survived.

## REPOSITORY MEMORY

Decision: created

Rationale: The stop-intent record, its fresh-start clearing, and the
window-liveness heuristic are runtime contracts whose reasons code alone would
hide.

Artifacts:

- `docs/specs/terminal-resume/SPEC.md`
- `CLI_SPEC.md`
- `README.md`
