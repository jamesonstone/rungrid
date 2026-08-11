---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "complete"
feature:
  id: "repository-maintenance"
  slug: "repository-maintenance"
  dir: "repository-maintenance"
relationships:
  - type: builds_on
    target: rungrid-v1
references:
  - id: cli-contract
    name: Rungrid CLI specification
    type: documentation
    target: CLI_SPEC.md
    relation: constrains
    read_policy: must
    used_for: public command, manifest, output, and lifecycle contracts
    status: active
  - id: worktree-policy
    name: Git worktree policy
    type: documentation
    target: docs/references/worktrees.md
    relation: constrains
    read_policy: must
    used_for: fail-closed worktree inspection and removal
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
    name: Repository maintenance issue
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/20
    relation: tracks
    read_policy: must
    used_for: delivery ownership
    status: active
  - id: workspace-discovery-fix
    name: Implicit workspace discovery fix
    type: issue
    target: https://github.com/jamesonstone/rungrid/issues/31
    relation: tracks
    read_policy: must
    used_for: service-bounded discovery when the workspace root is not a Git worktree
    status: active
skills: []
delivery_intent: issue_branch_pr_ready
---
# SPEC

## PURPOSE

Give one Rungrid workspace a safe, observable way to fast-forward every
configured repository's local default branch and reclaim obsolete linked
worktrees without changing active feature branches or bypassing service-session
ownership.

## CONTEXT

- A multi-repository workspace becomes stale after pull requests merge even
  when its services and terminal layout remain healthy.
- Updating repositories manually repeats remote/default-branch discovery and
  can accidentally switch, reset, merge, or rebase the feature worktree a user
  is actively testing.
- Merged and remotely deleted branches leave linked worktrees and build output
  behind. Their removal is destructive and therefore requires exact Git,
  GitHub, cleanliness, path, process, and commit-identity proof.
- Rungrid already declares the logical repository set and maps every service to
  one repository. The manifest remains the portable inventory; no consumer
  duplicates it in maintenance scripts.
- Process Compose is authoritative for managed-service lifecycle and the
  Overview is read-only. Maintenance may publish lifecycle and logs there, but
  the CLI remains the authorization surface.

## REQUIREMENTS

- Add `rungrid sync` and `rungrid worktrees prune` under a `Maintain` help
  section with human and `rungrid/output/v1` JSON output.
- Both commands act on all unique declared Git common directories or a
  repeatable `--repository` selection.
- When the implicit `workspace` repository is not itself a Git worktree,
  discover only the Git top-levels containing workspace-owned services'
  resolved working directories. Deduplicate those top-levels by Git common
  directory, report them by stable workspace-relative path, and never turn
  this compatibility path into a recursive filesystem scan.
- Keep `--repository workspace` as the aggregate selector for that inferred
  compatibility inventory. Workspace-relative report names do not become new
  logical selectors without matching manifest repository declarations.
- Both commands apply by default. `--dry-run` is strictly non-mutating: it may
  query GitHub and the remote but must not fetch, update refs, write state,
  stop processes, remove files, or prune metadata.
- Discover each repository's remote default branch from its live symbolic HEAD.
  Default the remote to `origin`; permit a repository-local override for a
  different remote and a default-branch fallback when symbolic discovery is
  unavailable.
- `sync` fetches and prunes only the selected remote, then fast-forwards only
  the local default branch. It never checks out, merges into, rebases, resets,
  or otherwise changes a feature branch.
- A missing, ahead, diverged, dirty, unavailable, or concurrently changed
  local default branch is preserved with an exact reason.
- If the default branch is not checked out, advance its ref with expected-old
  OID protection. If it is checked out, require a clean worktree and use an
  exact fast-forward-only merge.
- Services running from feature worktrees remain running and unchanged.
  Services running from the default-branch worktree pause before its files
  change and resume afterward. Tab sessions retain their registration and
  exclusive lock throughout a cooperative maintenance pause.
- If pause acknowledgement fails, make no repository change. If resume fails
  after a successful fast-forward, retain the updated branch, record a partial
  result, and give exact recovery guidance rather than moving the branch back.
- `worktrees prune` removes only exact registered linked worktrees that are not
  primary, current, detached, locked, manifest-declared, internally owned, or
  used as the working directory of any live process, including a Rungrid
  service, session, shell, or coding agent.
- A removal candidate must be clean, contain no unpushed commits, have exactly
  one same-repository pull request merged into the discovered default branch,
  match that pull request's head OID, and have no live remote head branch.
- Verified expected `.env` and `.envrc` symlinks targeting the primary
  checkout are the sole ignored/untracked exception. Remove only those links
  immediately before ordinary worktree removal and restore them if removal
  fails.
- Interactive prune shows every candidate and refusal reason, then requires
  confirmation. Non-interactive prune refuses to remove anything without
  `--yes`. Every candidate is revalidated immediately before mutation. A
  failure for one repository or candidate does not block an independently
  proven candidate, but the complete command returns a typed partial result.
- Removal uses ordinary non-force `git worktree remove`, `git branch -d`, and
  worktree metadata pruning. Never delete a remote branch or use reset, stash,
  clean, direct recursive deletion, force removal, or force branch deletion.
- When an active supervisor exists, expose the authorized maintenance run as a
  disabled Process Compose job in a `maintenance` namespace so Overview shows
  the same running/completed lifecycle and selectable logs. A job started
  without a valid generation-scoped request fails closed.
- Headless operation executes the same maintenance service directly without
  requiring Process Compose or terminal artifacts.
- Keep every affected handwritten source and test file at or below 300
  physical lines.

### Non-goals

- Keeping every feature branch rebased or merged with the default branch.
- Removing closed-unmerged, fork-backed, ambiguous, detached, legacy,
  non-canonical, dirty, active, or unverifiable worktrees.
- Scanning arbitrary filesystem trees or repositories absent from the Rungrid
  manifest.
- Making the read-only Process Compose Overview an authorization surface.
- Deleting remote branches or GitHub pull requests.

## ACCEPTED PLAN

1. Extend repository declarations with optional remote/default-branch metadata
   while preserving existing manifest defaults and workspace boundaries.
2. Build one maintenance inventory keyed by physical Git common directory,
   with focused argument-vector Git and GitHub gateways and typed plans.
3. Implement strict read-only planning for default synchronization and
   worktree pruning, including stable human/JSON reports and refusal reasons.
4. Implement serialized, revalidated application with expected-OID ref
   updates, fast-forward-only checked-out updates, confirmation, non-force
   removal, and crash-visible result journals.
5. Refactor tab sessions into a cooperative pause/resume state machine and
   coordinate only services whose actual worktree will change.
6. Generate disabled internal maintenance jobs that accept only an atomic,
   generation-scoped request created by the CLI; retain direct headless
   execution when no runtime is active.
7. Update help, agent instructions, README, CLI specification, schema, planner,
   golden files, and reusable testing guidance.
8. Validate fake-executable arguments, real temporary remotes/worktrees, PTY
   session continuity, Process Compose visibility, complete repository checks,
   source-size compliance, sanitization, and headless lifecycle compatibility.
9. Preserve sibling-workspace manifests created before named repository roots
   by deriving the implicit maintenance inventory from workspace-owned service
   working directories only when `workspace.root` is not a Git worktree.

## DECISIONS

- `sync` means fetch remote state and fast-forward the local default branch;
  feature branches are report-only. This preserves active development intent
  and avoids implicit history rewriting.
- Commands apply by default because synchronization is an operator action, but
  destructive worktree removal still requires explicit interactive
  confirmation or `--yes`.
- A feature-worktree service does not pause merely because shared remote or
  default refs change. Only a service executing from the checked-out default
  worktree whose files will advance is coordinated.
- The session owner, not the maintenance caller, performs a tab-owned pause and
  resume. This retains exclusive ownership and prevents `stop` from returning
  the managed shell to an ordinary prompt during maintenance.
- Process Compose presents maintenance lifecycle and logs but does not grant
  authorization. The generation-scoped request journal remains authoritative
  for the operation and recovery.
- Worktree proof follows the repository's fail-closed native Git contract and
  does not depend on Kit, a Git alias, or direct filesystem deletion.
- A declared logical repository may identify a directory inside a Git
  worktree. Maintenance deduplicates and protects the physical Git top-level,
  while service coordination maps nested declared roots back to that worktree.
- The implicit `workspace` repository remains the first maintenance candidate.
  If its root is not a Git worktree, configured workspace-owned service paths
  are the only fallback inventory. This preserves existing sibling-workspace
  manifests without weakening named repository validation or scanning
  undeclared directories.

## DISCOVERIES

- The existing tab session exits and releases its lock when Process Compose
  reports a stopped process. Cooperative maintenance therefore requires an
  explicit pause state that suppresses the ordinary terminal-state exit path,
  rotates log following, and acknowledges pause and resume transitions.
- Repository declarations currently contain only `path`; remote metadata can
  be added without a second repository inventory or a new top-level
  maintenance section.
- Process Compose disabled processes can represent maintenance jobs, while the
  existing read-only attachment prevents operators from starting them through
  Overview. A request check is still required because mutable attachments and
  direct Process Compose clients exist.
- Git ordinary worktree removal does not protect a clean directory merely
  because another process has its current working directory inside it. Rungrid
  therefore treats bounded `lsof` cwd inspection as required removal proof and
  preserves the candidate when that proof is unavailable.
- Apply-time pruning must relist registered worktrees and rediscover the remote
  default branch for every candidate. Reusing the preview's worktree OID would
  leave a race in which a newly advanced clean branch could be removed using
  stale proof.
- A checked-out default branch also needs expected-OID revalidation after
  services pause. `git merge --ff-only` alone is insufficient because a
  concurrent local advance can make it return successfully without reaching
  the expected remote OID.
- Service recovery cannot inherit the canceled maintenance request context.
  After ownership has paused a process, Rungrid uses a fresh bounded recovery
  context so interrupting a Git operation still attempts to restore every
  paused workspace and tab service.
- Local branch deletion can legitimately fail after a squash-merged pull
  request because ordinary `git branch -d` cannot prove commit ancestry. The
  worktree removal remains truthful and the local branch is preserved as a
  partial result.
- Sibling-workspace manifests can validly use a non-Git container as
  `workspace.root` while leaving services on the implicit `workspace`
  repository. Treating that container as the sole maintenance repository
  prevents synchronization even though every service working directory is an
  explicit, validated path into a Git worktree.

## VALIDATION

- `make fmt-check`, `make vet`, `make test`, `make test-race`, and
  `make sanitize` passed.
- `make lint` reported zero issues; `make vuln` found no called
  vulnerabilities; `make license` found license or notice material for every
  dependency module.
- `make build-cross` produced CGO-free Darwin and Linux builds for amd64 and
  arm64. `make release-snapshot` validated GoReleaser and built all four
  snapshot archives.
- `tests/end-to-end/local/run.sh` passed with bounded evidence at
  `tmp/2026-08-06/rungrid-headless-e2e/3`. Its real Process Compose maintenance
  case proved strict dry-run behavior, authorized job visibility, exact
  default-branch advancement, and both workspace-owned and tab-owned service
  pause/resume back to `Running`. Rejecting a second tab session after the
  operation proved the original session retained exclusive ownership.
- Real temporary bare remotes and linked worktrees proved deduplication,
  expected-OID synchronization, dirty/diverged preservation, merged-PR and
  deleted-remote proof, process-in-use refusal, safe environment-link
  restoration, and truthful partial branch cleanup.
- One initial package-parallel `go test ./...` run hit the existing one-second
  lifecycle fixture timeout under host load. The isolated test passed three
  consecutive runs, the complete serial suite passed, and the subsequent
  ordinary `make test` and full race suite passed.
- The complete handwritten source/test audit found no file above 300 physical
  lines.
- The focused maintenance regression passed against a non-Git workspace root
  containing two real child repositories and bare remotes. It proved that
  nested service directories deduplicate to two Git common directories and
  that `Sync` completes without discovery failures.
- The fixed CLI completed a live `sync --dry-run` against the sibling-workspace
  manifest that exposed the defect. It reported all 13 configured repository
  roots with no recursive discovery or Platform mutation.
- `make check`, `make lint`, `make vuln`, the local headless end-to-end suite,
  and `make release-snapshot` passed. The first `make check` attempt hit the
  unchanged one-second workspace execution fixture under load; that isolated
  test passed five consecutive runs and the complete rerun passed, including
  race tests and all cross-builds.
- The headless suite recorded PASS evidence at
  `tmp/2026-08-11/rungrid-headless-e2e/2` with run ID
  `20260811T153441Z-071409`, 345 output bytes, and asserted cleanup.
- The current Kit CLI did not discover the repository's legacy nonnumeric spec
  directories for feature or `--all` validation. `kit check --project`
  separately retained ten pre-existing Kit-managed instruction findings; this
  fix did not modify those support documents or claim that project check as a
  pass.

## OUTCOME

Rungrid now provides `sync` and `worktrees prune` with human and typed JSON
reports, repeatable logical-repository selection, and strictly read-only
planning. Repository declarations carry portable remote and default-branch
fallback metadata without persisting developer paths.

An active runtime executes apply operations as disabled `maintenance`
namespace Process Compose jobs authorized by single-use generation-scoped
requests. Default-worktree services pause and resume through the same
supervisor; tab sessions retain their registration and exclusive lock. Direct
headless operation uses the same maintenance engine and workspace lock.

Pruning is conservative and partial-result aware: it deduplicates Git common
directories, protects declared/current/internal/non-canonical lanes, inspects
all cwd processes, verifies exact GitHub merge identity and remote deletion,
revalidates live Git state immediately before ordinary removal, restores
verified environment links on failure, and never force-deletes local or remote
state.

Manifest-scoped maintenance now also supports sibling workspaces whose root is
an intentional non-Git container. Rungrid first preserves the implicit
workspace-repository behavior, then falls back only to workspace-owned
services' validated working directories when that root is not a Git worktree.
It reports the inferred Git top-levels by workspace-relative path, deduplicates
them by common directory, and never scans undeclared workspace content.

## REPOSITORY MEMORY

Decision: updated

Rationale: The service-bounded compatibility behavior is a material extension
of repository-maintenance inventory semantics. The existing feature spec and
CLI contract now explain why non-Git sibling-workspace containers are supported
without weakening declared repository validation or allowing recursive scans.

Artifacts:

- `docs/specs/repository-maintenance/SPEC.md`
- `CLI_SPEC.md`
- `tests/RUN_STATUS.md`
