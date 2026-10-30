---
kit_metadata_version: 1
artifact: "spec"
workflow_version: 3
phase: "delivery"
feature:
  id: "warp-window-lifecycle"
  slug: "warp-window-lifecycle"
  dir: "warp-window-lifecycle"
relationships:
  - type: builds_on
    target: terminal-resume
---
# SPEC

## PURPOSE

Stop repeated restarts from stacking live Warp windows (GH-76). After one overnight run that restarted a workspace after each merge, the human found over 20 Warp terminals and the GPU pinned at 100%.

## CONTEXT

Warp's own records for that run showed 19 window-opening `rungrid up` runs and 20 surviving Overview tabs, with one Versions tab and one of each service tab. Three defects combined:

- `up` opened a full window on every call, even for a reused runtime with windows open.
- `attach` had no link to its runtime. The Process Compose client retries its socket, and the next runtime reuses that path, so after `down` then `up` the old client reconnected and repainted indefinitely at about 25 KB/s. Each restart added one live client. Versions and service tabs already watched their runtime, which is why only Overview tabs survived.
- The resource guard deleted live client registrations. It compared them with a process snapshot taken before the client started, so `overviewAttached` was usually false and `resume` could not see a live window either.

## REQUIREMENTS

- `up` on a reused runtime opens no window when a window is live or opened in the last 90 seconds. It opens only requested service tabs that are absent, and says so. `rungrid open` still opens another on request.
- `attach` ends within seconds when the exact runtime it attached to stops, restores the terminal, exits 0, and never reconnects to a later runtime.
- A client registration is removed only when that exact process is gone.
- A fresh runtime always opens its window: a live signal cannot apply to a runtime that was just started.

## OPEN QUESTIONS

- Warp's `new_window=true` Tab Config URI creates a window with an untitled default tab beside the Overview tab. That tab has no Rungrid process and outlives `down`, so each fresh start still leaves one idle window. Warp offers no close API; closing it would need Accessibility automation, or opening the workspace another way.

## ACCEPTED PLAN

1. `planUpOpen` and `windowsLive` decide what `up` opens, from the same evidence `resume` uses.
2. `watchRuntimeEnd` polls `RuntimeActive` and stops the attach client with SIGTERM, so its TUI restores the terminal.
3. `controlClientRegistrationStale` asks about the exact process.
4. `terminal-opened.json` records the open, for the grace period.

## DECISIONS

- The watchdog compares the exact runtime record, so a new runtime on the same socket path and generation still ends the old client.
- SIGTERM with a three-second `WaitDelay`, not the default kill: a killed TUI leaves an interactive terminal in raw mode.
- A runtime that crashes without shutdown keeps its clients until the identity changes, so the operator can still see the failure.
- A 90-second grace period over tracking window state: nothing can observe a Warp window before its shell starts the client, and starts took 6 to 30 seconds.
- Rejected: exiting `attach` with a success code alone. The first diagnosis blamed the exit code, from a test that never connected; a corrected test showed the exit code was already 0.

## DISCOVERIES

- Warp runs a tab's command only after its shell starts, 6 to 30 seconds after the URI opens.
- `resume`'s window detection was affected by the same stale-registration defect and now works.

## VALIDATION

Unit: `TestPlanUpOpen`, `TestAttachResultTreatsAStoppedRuntimeAsACleanEnd`, `TestWatchRuntimeEnd`, `TestWindowStartingCoversOnlyThisRuntimesRecentOpen`, `TestControlClientRegistrationStaleAsksTheProcess`.

Manual, real pseudo-terminal: before the fix, `attach` was still running 53 seconds after `down` then `up`, streaming about 25 KB/s; after it, the client exits 0 within 3 seconds. `resume --no-open` reports `windows_live: true` with a client attached.

Manual, real Warp with a throwaway project: first `up` opened one window; an immediate second `up` and a third after the client attached opened none; `down` ended the client and closed the Overview tab. The Warp checks ran before the final commit's documentation changes only.

## OUTCOME

Shipped except the idle default tab, recorded as an open question.

## REPOSITORY MEMORY

- Created: this spec. Updated: `CLI_SPEC.md`.
