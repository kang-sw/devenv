# Plan: throttle the Claude mod's dashboard feed so it calls ws-mcp far less often

## Intent

User decision (2026-11-12, whiteboard topic `hook-alarm`, item (616) c):
mitigate now, and design the root fix (moving the feed off `$.mcp.call`) in
a ticket.

Symptom: Orca shows idle/working alarm banners on every lead tool call.
Orca's trace (`main.trace.ndjson`) logs `agent_finish_dropped` about every
0.2 s for this pane, matching the mod's dashboard intake cadence. Working
hypothesis (not yet confirmed): each `$.mcp.call` from the mod fires Claude
Code's PreToolUse/PostToolUse command hooks, so Orca's hook runs per feed
call. Fewer feed calls means fewer hook firings.

The feed lives in `agents-plugin/hooks/claude-mod/dashboard.ts`:
`FLUSH_INTERVAL_MS = 100` (discrete changes), `TEXT_FLUSH_INTERVAL_MS = 1000`
(streamed text only). The flush is already change-driven.

## Change

- Raise the discrete-change flush gap from 100 ms to 1000 ms, and the
  streamed-text-only gap from 1000 ms to 2000 ms (text was the flicker
  source; keep it slower than discrete changes). Keep the change-driven
  behaviour: no call when nothing changed.
- Update the comments that state the cadence or its rationale.
- Mirror into `agents-plugin-wsflow/hooks/claude-mod/` if that tree carries
  a copy of the file (check `ai-docs/manuals/wsflow-mirroring.md` for how
  the mod is shared; do not hand-copy if a sync/test enforces it some other
  way).
- Do not change the drain/watcher paths, retry timing, or the intake
  protocol.

## Verification

- The mod's existing test suites for the dashboard feed
  (`dashboard.test.ts`, `dashboard-engine.test.ts`) pass; adjust any test
  that pins the old constants, without weakening what it checks.
- Any package drift test for the wsflow copy passes.
- Report the exact commands and results.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/wsflow-mirroring.md` before editing.
- Commit with `## AI Context` citing (616).
