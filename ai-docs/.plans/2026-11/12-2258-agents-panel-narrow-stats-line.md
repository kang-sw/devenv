# Plan: narrow agents panel shows subagent stats on a second line

## Intent

User decision (2026-11-12, whiteboard (571) a, (573) a): implement the
mockup at `ai-docs/ref/agents-panel-stats-mockup/` (variant 2, "option a",
not 2b). In the dashboard agents panel, when the panel is narrower than the
existing 560px container threshold (`.r-tree` / `atree` container query,
Decision 3 of `261110-feat-ws-dashboard-agent-panel-telemetry`), subagent
stats (model + effort, context tokens, output t/s) are no longer hidden into
the hover card; they show on a muted second line indented under the row
name, with `·` separators, as in `2-option-a-narrow.png`. Ended subagents
keep model/effort/context and no rate (unchanged data rule). The wide layout
and the lead row are unchanged. Effort stays in the stats line even when the
agent type caption (`ws:effort-high`) repeats it (2b rejected).

## Implementation notes from the mockup

- The mockup's `index.html` holds the CSS block (`.va`) inside the
  `@container atree (max-width: 559px)` query; port it into
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`.
- Pure CSS failed at 360px with a long title; the mockup wraps the subagent
  summary's mark, name, phase, and caption in a `<span class="arow">`
  (`display: contents` except in the narrow query). Apply that one-line
  markup change in the real template/JS that builds agent rows.
- The narrow-width hover card for stats becomes redundant; remove it only if
  nothing else uses it, otherwise leave it and say why.

## Verification

- Existing wsdashboard Go tests (`go test ./internal/wsdashboard/`) and any
  asset/JS tests touching the agents tree.
- Headless Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`)
  over a rendered session page at 520px, 360px with a long title, and 900px;
  compare against the mockup screenshots; no page errors.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` (dashboard asset content is T0).
- Dashboard assets only; no wire/state changes.
