# SPEC: Human Command Output

## Purpose

Rungrid's help surface was designed deliberately: emoji section labels, ANSI styling,
workflow-grouped commands. Its command output was not. `status` printed one
fixed-width line per service with `pid=0 health= session=false tab=false` trailing it,
`up` printed a single sentence after an opaque wait, and `down` printed nothing at
all. The moment an operator left `--help`, the tool stopped explaining itself.

This feature gives the workspace-facing commands one presentation vocabulary — emoji
status glyphs, box-drawn tables, and per-service announcements — without touching the
machine contract.

Scope is the surfaces an operator reads while running a workspace: `plan`, `generate`,
`doctor`, `status`, `versions`, `up`, `down`, `start`, `stop`, `uninstall`, `sync`,
`worktrees prune`, and `reconcile`. `init`, `config validate`, `config path`, and
`version` still print their single plain line; each emits one short fact rather than a
surface, and restyling them would add vocabulary without adding legibility. They remain
available to adopt the same helpers if they ever grow.

## Decisions

### Emoji is content; color is decoration

Command output emits emoji **always**, including when redirected, piped, or run with
`--no-color`. ANSI color is emitted **only** when the destination writer is a terminal
and neither `--no-color` nor a non-empty `NO_COLOR` is set.

The two are gated differently on purpose:

- An emoji is a character. It survives redirection, lands in a log file, and pastes
  into an issue intact. Gating it on terminal detection would make the same command
  produce two different documents.
- ANSI escapes are terminal control, not content. In a pipe they are noise.

This is the rule for **command** output. **Help** output keeps its existing stricter
rule — emoji and color both gated on interactivity — because its plain-text form is a
golden-tested contract (`cmd/testdata/root-help.txt`) and `CLI_SPEC.md` §11 already
promises that redirected help carries no "terminal-only heading decoration". The two
rules coexist; neither was relaxed.

### Meaning never depends on a glyph

Every glyph is paired with its plain status word in the same row: 🟢 sits next to
`running`, ❌ next to `error`. A reader whose font lacks the emoji, whose terminal
drops color, or who greps the output loses nothing. This preserves the existing
constitutional rule that color must never carry meaning, and extends it to emoji.

### Announce-only progress, not a polling loop

`up` and `down` announce what they are about to do, then report the outcome. They do
not poll Process Compose to narrate each service's transition.

Process Compose is the single authority for managed-service lifecycle, and `up` hands
off to it in one call (`supervisor.Start`). A narrating progress loop would have to
poll that authority and re-derive per-service state that `rungrid status` already
reports accurately — a second, laggier view of the same truth, and a new failure mode
when the poll disagrees with the runtime. The announce lines state intent; `status`
remains the honest view.

### The resource guard renders as its own block, not extra service columns

`rungrid status` reports the runtime resource guard added by the runtime-resource-guard
feature. Its per-service data — state, enforcement, four observed/limit pairs, circuit
state — does not fit the service table, which already carries seven columns describing
a different question (is this service running and who owns it).

The guard therefore renders as a separate block: an authority summary, its own table
keyed by service, and note lines beneath. The notes carry only what the table cannot
express and what a reader must not miss — an immature baseline, accumulated guard
restarts, a degraded reason, the latest incident — so a workspace in steady state adds
no lines at all. A dimension with no configured limit prints the observation alone
rather than an invented ceiling.

`RuntimeVerification` renders in the workspace header rather than the guard block,
because it qualifies whether the reported runtime state can be trusted at all.

### Tables measure display width, and draw their own box

The previous `%-11s` renderers computed column width in bytes. A status glyph like 🟢
occupies two terminal cells, so byte- or rune-padded columns skew every column to
their right the moment emoji enter a cell. `internal/present` measures with
`ansi.StringWidth`, which ignores escape sequences and counts wide characters as two.

The box is drawn directly rather than through `lipgloss/table`, which was the first
choice and was rejected during implementation. Every `lipgloss` render resolves its
color profile through a shared renderer bound to `os.Stdout`; on a terminal that
probes for the background color (`OSC 11`, terminated by a cursor-position report).
The probe fired even with `--no-color`, which would have broken the promise that a
colorless run writes no escape sequences, and would have written terminal queries as a
side effect of printing a table. Hand-drawing the box is roughly fifty lines, has no
global renderer state, and emits exactly the bytes the gate allows.

Note that the same probe already occurs once per invocation at `HEAD`, from the
`lipgloss` dependency pulled in by the onboarding TUI — including on the `--json`
path. That is pre-existing and out of scope here; the point is that the table must not
add more of them.

## Boundaries

- `--json` output (`rungrid/output/v1`) is unchanged in shape, field order, and
  content. Presentation is a human-path concern only.
- Exit codes, error codes (`RG####`), command names, flags, and ordering are
  unchanged.
- `--quiet` continues to suppress non-error human output, including all new lines.
- `internal/present` performs no terminal detection. Callers pass the resolved
  color decision in, which keeps the renderers pure and preserves the
  `terminalWriterCheck` test seam in `cmd/help_style.go`.

## Structure

`internal/present` holds the shared vocabulary:

- `style.go` — the color gate and ANSI helpers
- `glyph.go` — status glyphs and section emoji
- `table.go` — the box-drawn table builder
- `section.go` — headers, fields, announce steps, results, warnings

Domain renderers keep their existing home and gain a `present.Style` parameter:
`internal/planner`, `internal/versions`, `internal/reconcile`, `internal/maintenance`,
`internal/doctor`, and `internal/lifecycle` (status). `cmd` computes the style once per
command from the writer plus `--no-color`/`NO_COLOR` and passes it down.

Two details keep the announcements honest rather than decorative:

- `down` announces per service using `lifecycle.Stoppable`, the runtime's own
  shutdown predicate, so an announcement can never claim work that `stopRuntime`
  will skip. A service already stopped is reported as already stopped.
- `down` inspects state only to build its announcement. An inspection failure
  degrades the announcement and never blocks `DownProject`, because a workspace in
  a conflicted state must always remain tearable-down.

The interactive `versions --watch` surface keeps the alternate screen introduced by
the runtime-resource-guard work. `versionsWatchDisplay` owns the screen control and now
carries the resolved `present.Style`, so the restyle changed what the frame contains and
left when the frame is drawn alone. Its redirected form stays free of control sequences,
which the existing watch tests continue to assert.

## Validation

- `make fmt-check`, `make vet`, `make test`, `make test-race`, `make lint`,
  `make sanitize`
- `cmd/testdata/root-help.txt` must still match byte-for-byte, proving help output was
  not disturbed.
- `internal/planner/testdata/multi-workspace-plan.txt` regenerated; the redacted-argv
  assertion in `plan_test.go` must survive the restyle.
- Gating proof: `cmd.TestPresentStyleGatesColorButNeverEmoji` drives all four paths
  through the `terminalWriterCheck` seam — interactive, `--no-color`, `NO_COLOR`, and
  redirected — and asserts ANSI appears only on the first while emoji and box borders
  appear on all four. `internal/present` and the guard renderer assert the colorless
  case directly. A real piped run of `rungrid plan` was confirmed to carry zero SGR
  escapes, and a pty run to carry them.
- The pre-existing `OSC 11` background probe still fires once per invocation from the
  `lipgloss` dependency pulled in by the onboarding TUI, on the `--json` path included.
  It was present before this feature and no renderer here adds another.
- `--json` envelopes compared before and after for identity.
