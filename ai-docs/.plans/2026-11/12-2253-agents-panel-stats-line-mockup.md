# Plan: mockup of subagent stats on a second row line in a narrow agents panel

## Intent

User request (2026-11-12, whiteboard (571) "a안 목업 좀 해봐주세요"): in the
dashboard agents panel, subagent telemetry (model, effort, context tokens,
output rate t/s; `261110-feat-ws-dashboard-agent-panel-telemetry`) is hidden
into a hover card when the panel is narrower than 560px (Decision 3 of that
ticket; `.r-tree` / `atree` inline-size container query). Option (a) under
review: in a narrow panel, keep the stats visible on a **second line** under
the subagent row instead of hiding them. Mockup only, no product change.

## Deliverable

A static mockup under `ai-docs/ref/agents-panel-stats-mockup/` (new folder,
following sibling `ai-docs/ref/*-mockup/` layout): `index.html` plus PNG
screenshots, committed.

- Base it on the real agents panel markup and CSS
  (`agents-plugin-tool/internal/wsdashboard/assets/` — dashboard.css and the
  JS/template that builds the agents tree rows and the stats/hover card), so
  it reads as today's panel with one change. Copy what you need; do not edit
  product assets.
- Reference row today (narrow): caret, status dot, title "Whiteboard
  watching-output rule", kind badge "도구", agent type `ws:effort-high`,
  `✓ 13`, `✕ 0`, `2m 24s`, and a nested running tool line
  (`Bash  command: cd /Users/...  1m 37s`).
- Show, side by side or stacked: (1) current narrow layout (stats hidden),
  (2) option (a) narrow layout with a muted second line such as
  `claude-sonnet-5-5 · high · 48.2k · 62 t/s`, (3) the wide (>=560px) layout
  unchanged for comparison. Include a running subagent (with t/s) and an
  ended one (no rate). Consider whether `ws:effort-high` duplicates the
  effort shown on the stats line and show the de-duplicated variant if it
  reads cleaner; note the choice.
- Narrow width ~520px CSS, wide ~900px. Match the dark theme the screenshot
  shows.

## Constraints

- Mockup only: no changes to `agents-plugin*/` sources.
- Screenshots via headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`).
- Commit the mockup folder and this plan; `## AI Context` cites (571).
