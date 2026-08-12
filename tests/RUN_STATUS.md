# Validation run status

| Suite | Environment | Current status | Latest attempt | Latest pass | Source/deployment | Run ID | Evidence | Active finding |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Headless lifecycle | local | PASS | 2026-08-12T14:53:06Z | 2026-08-12T14:53:06Z | dirty `6777102` / no deployment | `20260812T145024Z-005243` | `tmp/2026-08-12/rungrid-headless-e2e/6/` (ignored local evidence) | Lifecycle, session quiescence, emergency and sustained containment, circuit reset, external-process survival, and exact cleanup passed against Process Compose 1.120.0. |
| Resource guard soak | local macOS | PARTIAL | 2026-08-12T14:40:45Z | none | dirty `6777102` / no deployment | local smoke | `tmp/2026-08-12/rungrid-resource-guard-soak/5/` (ignored local evidence) | Graphical 30-second smoke passed after same-generation runtime rotation; the required uninterrupted 24-hour acceptance run remains pending. |
| Graphical Warp smoke | local macOS | PARTIAL | 2026-08-12T14:40:45Z | none | dirty `6777102` / no deployment | local operator observation | `tmp/2026-08-12/rungrid-resource-guard-soak/5/` (supporting ignored evidence) | Overview rendered status and logs with a foreground attach client, and down quiesced managed tabs; a dedicated graphical smoke evidence runner is not implemented. |
| Production | production | NOT_APPLICABLE | not applicable | not applicable | non-deployed CLI | none | none | Rungrid has no production service environment. |
