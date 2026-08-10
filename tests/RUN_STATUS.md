# Validation run status

| Suite | Environment | Current status | Latest attempt | Latest pass | Source/deployment | Run ID | Evidence | Active finding |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Headless lifecycle | local | PASS | 2026-08-10T20:29:34Z | 2026-08-10T20:29:34Z | clean `0ad699d` / no deployment | `20260810T202921Z-024383` | `tmp/2026-08-10/rungrid-headless-e2e/1/` (ignored local evidence) | Mixed-service, tab-only, and repository-maintenance workspaces passed against Process Compose 1.120.0; output was 343 bytes of the 64 MiB limit and cleanup was asserted. |
| Graphical Warp smoke | local macOS | BLOCKED | not run | none | local source / no deployment | none | none | Requires a controlled user-visible Warp session. |
| Production | production | NOT_APPLICABLE | not applicable | not applicable | non-deployed CLI | none | none | Rungrid has no production service environment. |
