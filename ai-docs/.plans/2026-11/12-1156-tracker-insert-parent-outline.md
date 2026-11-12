# Tracker hover insert outlines the would-be parent row

## Goal

In the dashboard tracker list, while the hover insert indicator (the `+`
badge between rows, with gray reachable-depth ticks and red blocked-depth
x marks, aea06dcc3) is showing, draw an outline on the row that would become
the parent of the item inserted at the badge's current depth. The outline
follows the badge as the pointer changes depth; no outline when the badge
depth is the root (no parent). User request, 2026-11-12, with a screenshot of
the indicator under "발표 자료 정리 > 슬라이드 구성".

## Scope

- `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`: where the
  insert indicator picks its depth (insOpts / insPick), resolve the parent
  row for the picked depth (the nearest row above the gap at depth
  `pick - 1`) and toggle a class on it; clear it whenever the indicator
  hides, moves, or the list re-renders.
- `agents-plugin-tool/internal/wsdashboard/assets/tracker.css`: the outline
  style - quiet, uses existing `--wb-*` tokens (primary at reduced strength,
  e.g. a 1px inset outline / box-shadow with small radius), light and dark,
  no layout shift.
- Tests: extend `tracker_quickadd_test.go` (node-driven) for: the parent
  class lands on the right row for each picked depth, none at depth 0, and
  is cleared when the indicator hides.
- Headless check against the real CSS (as the aea06dcc3 delegate did):
  screenshots light and dark at two depths into
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/tracker-parent/
  (playwright-core under that scratchpad's `pw/node_modules`; chromium at
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`).

## Constraints

- insOpts/insPick/mvSlot semantics unchanged; drag-move behavior unchanged.
- Dashboard asset change: compatibility tier T0.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/tracker-insert-parent-outline` in this
  worktree; do not merge or push.
