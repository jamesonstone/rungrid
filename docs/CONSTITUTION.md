# CONSTITUTION

## PRINCIPLES

- A Rungrid workspace is declared by a portable manifest, not by scripts owned
  by a particular consumer repository.
- Process Compose is the single authority for managed-service lifecycle. Every
  Rungrid view and service command must report or change that same runtime
  rather than infer a parallel service state.
- Interactive terminal ownership and process supervision are separate: a tab
  may own the right to start and stop a service while Process Compose remains
  authoritative for its lifecycle and logs.
- One-shot workspace prerequisites and teardown are Rungrid operations. Their
  journal, not Process Compose, is authoritative for ordering and recovery.
- Repository maintenance is fail-closed. Ordinary `sync` remains
  manifest-scoped; explicitly invoked filesystem `reconcile` is the sole
  exception and still requires independent Git, GitHub, path, activity,
  process-ownership, cleanliness, and exact-OID proof.

## CONSTRAINTS

- Manifest and output contracts use `rungrid/v1` and `rungrid/output/v1`.
- Project identity must not encode or hash an absolute developer path.
- The manifest directory and workspace root are distinct. The portable root is
  relative to the manifest, while resolved paths are machine-local and must
  remain inside the symlink-aware workspace boundary.
- Services may select a declared logical repository within the workspace.
  Service working directories, Compose files, and environment-provider paths
  must remain inside that repository's symlink-aware boundary.
- Subprocesses use argument vectors. User commands, environment values, and
  paths must not be interpolated into shell command strings.
- Secrets resolve only at execution time and must be redacted from errors,
  plans, generated artifacts, registrations, and evidence.
- External services are observed but never started or stopped by Rungrid.
- Runtime state is project-scoped, private, atomic, and fail-closed. PID,
  process-start, socket, generation, owner, and content-hash checks protect
  every mutation boundary they identify. Automatic crash recovery may retire
  only an unchanged runtime record that matches the lifecycle journal after
  both its recorded PID and expected socket are absent; that path never signals
  a PID or removes a socket.
- Every enforcement action is scoped by project ID, generation ID,
  effective-manifest hash, verified Process Compose runtime identity, and
  proven process ancestry. Filesystem location, executable name, port
  ownership, and service name are supporting observations only and never
  confer termination authority. If any scope or ownership proof is missing or
  changes, enforcement fails closed.
- Resource enforcement owns only managed native and Compose host process trees
  and registered Rungrid control clients from the exact generated scope.
  External services, lifecycle hooks, manual processes, other projects, and
  stale generations remain outside termination authority.
- A project-scoped lock serializes global lifecycle mutation. Once startup may
  have changed external state, its journal retains teardown intent until every
  required cleanup command succeeds, even when the runtime record is missing.
- Global lifecycle hooks are exact, sequential argument vectors. They are not
  managed services or terminal tabs and never run for individual service
  `start` or `stop` commands.
- Generated terminal files may be replaced or removed only when their ownership
  marker and last recorded content hash match.
- A generation shutdown marker quiesces only that generation's owning sessions
  and managed tab shells. Each releases its private registration and child
  process before teardown; another project or generation remains untouched.
- Headless operation must not require or generate graphical terminal state.
- Coding-agent instructions are read-only guidance. They must not inspect or
  execute supplied project paths, replace the manifest as configuration
  authority, or override a consumer repository's rules and user authorization.
- Human help output may add color and terminal-only decoration only when the
  output is interactive and color is not disabled. Color must never carry
  meaning; redirected and explicitly colorless help remains complete and
  stable.
- Human command output treats emoji as content and color as decoration. Emoji
  are emitted unconditionally so redirected output is the same document;
  ANSI color is emitted only when the writer is interactive and color is not
  disabled. Every glyph is accompanied by its plain status word, so neither
  color nor a glyph ever carries meaning alone. Printing human output never
  writes terminal query or control sequences beyond that decoration and the
  alternate-screen watch surfaces each command specifies, and never alters
  machine-readable output or exit status.
- Linked feature worktrees are never implicitly checked out, merged, rebased,
  reset, stopped, or rewritten by repository maintenance. Filesystem
  reconciliation may commit, stash, or switch only the primary checkout under
  its explicit stale-root gates; ordinary manifest maintenance retains the
  no-feature-checkout and no-stash rule. A checked-out default worktree advances
  only after clean-state and expected-OID revalidation, with exact affected
  services paused and resumed.
- Process Compose may present disabled repository-maintenance jobs and their
  logs, but only a short-lived generation-scoped CLI request authorizes one.
  The read-only Overview never grants mutation authority.
- Worktree removal never uses force or direct recursive deletion. It requires
  a canonical clean inactive lane, one same-repository merged pull request
  whose head OID equals the worktree HEAD, a deleted remote branch, and no
  process whose working directory is inside the lane.

### Kit-Managed Baseline Rules

<!-- BEGIN KIT-MANAGED BASELINE RULES -->
- Treat `docs/CONSTITUTION.md` as the canonical project contract.
- Keep `AGENTS.md`, `CLAUDE.md`, and `.github/copilot-instructions.md` aligned with the repo-local docs tree.
- Treat `docs/notes/<feature>` as optional source material, not canonical truth; promote durable decisions into `SPEC.md`, `docs/CONSTITUTION.md`, or durable references.
- Use native agent planning for research, clarification, design, and implementation planning.
- Before implementation, inspect code and repository memory; create or adopt `SPEC.md` when material rationale exists.
- After validation, curate feature rationale, project invariants, reusable practices, and domain knowledge into their scope-appropriate canonical documents.
- Allow a justified `not required` repository-memory decision when code and tests preserve the complete durable truth.
- Keep every version-control-eligible handwritten implementation/source and test file at 300 physical lines or less.
- Before delivery, audit the complete affected source/test scope; whole-project reconcile and scheduled maintenance audit the entire repository.
- Exclude documentation files, all `docs/**`, all `.kit/**`, `.kit.yaml`, ignored files, vendored dependencies, and proven generated files.
- Split oversized files by semantic responsibility while preserving stable public entry points and behavior; never use minification or arbitrary numbered chunks to claim compliance.
<!-- END KIT-MANAGED BASELINE RULES -->

## CHANGE CLASSIFICATION

<!-- all work falls into one of two tracks — classify before acting -->

### Repository-Memory Work

<!-- use when: consequential product rationale, architecture, cross-component behavior, or historical decisions must survive -->
<!-- workflow: native plan → create/adopt SPEC.md before code → implement → validate → curate repository memory -->
<!-- legacy staged documents: BRAINSTORM.md, legacy SPEC.md, PLAN.md, TASKS.md only when explicitly chosen -->

### Ad Hoc (Lightweight)

<!-- use when: bug fixes, security reviews, refactors, dependency updates, config changes, small refinements -->
<!-- workflow: understand → implement → verify -->
<!-- docs: update practical canonical docs when behavior changes -->
<!-- do not create feature SPEC.md solely for ceremony; report a justified not-required memory decision -->

### Ad Hoc with Existing Specs

<!-- if change touches code with existing spec docs: update them when rationale, behavior, requirements, or approach changes -->
<!-- leave them unchanged when code and tests communicate the complete durable truth -->

## NON-GOALS

- Rungrid v1 does not implement a product-owned dashboard, an all-logs tab,
  additional graphical terminal adapters, Windows support, or command-free
  multi-pane workspaces.
- Rungrid does not own consumer-repository rollback material or rewrite
  repository history as part of normal implementation delivery.

## DEFINITIONS

- **Workspace-owned service:** a managed service started during `rungrid up`.
- **Tab-owned service:** a disabled managed process started only while an
  exclusive generation-scoped service session owns it.
- **External service:** a readiness dependency observed without lifecycle
  mutation.
- **Generation:** an immutable, content-addressed set of derived runtime and
  terminal artifacts for a validated manifest.
- **Workspace root:** the relative manifest declaration whose resolved,
  symlink-aware directory bounds all workspace-owned execution paths.
- **Repository root:** a stable logical service boundary declared relative to
  the workspace root; `workspace` is the implicit backward-compatible root.
- **Lifecycle command:** an ordered one-shot prerequisite or teardown command
  owned by Rungrid rather than Process Compose.
- **Lifecycle journal:** the crash-safe project record that proves the active
  generation, completed prerequisites, teardown obligation, and cleanup result.
- **Overview:** the read-only remote Process Compose TUI and its selectable
  service logs.
- **Versions:** the live service, listener, Git branch, commit, and worktree
  state view.
- **Repository sync:** a manifest-scoped fetch followed by an expected-state
  fast-forward of only each local remote-default branch.
- **Repository reconcile:** an explicitly invoked filesystem scan that
  deduplicates physical clones, synchronizes each proven remote default,
  conservatively repairs only eligible stale primary checkouts, and applies
  native linked-worktree cleanup proof.
- **Worktree prune:** a confirmed, immediately revalidated, non-force removal
  of only linked worktrees whose independent safety proofs all succeed.
