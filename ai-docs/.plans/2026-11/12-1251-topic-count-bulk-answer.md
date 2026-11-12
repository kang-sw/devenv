# Topic header count chips answer all of their items at once

## Goal

User request 2026-11-12 (board, with a screenshot of a topic bar on the
dashboard board showing `1분 전`, an orange `? 1` chip and a blue `✓ 1`
chip): clicking a topic header's count chip should answer every item it
counts in one go, since clicking each item one by one is tedious.

- The orange `?` chip counts the topic's open policy items.
- The blue `✓` chip counts the topic's open acknowledgement items
  (technical / announced).

Locate the chips yourself: the topic bar is rendered by
`agents-plugin-tool/internal/wswhiteboard` (`render.go`, `assets/page.tmpl`,
`assets/page.css`), and the dashboard's answer staging lives in
`agents-plugin-tool/internal/wswhiteboard/assets/answers.js` and/or
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`
(`writeAnswers`, the SEND flow).

## Decisions

- Clicking the `✓` chip stages `ok` on every open technical/announced item
  of that topic, exactly as if the user had pressed each item's ok control.
- Clicking the `?` chip stages each open policy item's recommended option
  (its `wb:recommend` letter), as if the user picked it. An open policy item
  with no recommendation is left untouched (it still needs a real choice),
  and the chip then keeps counting it.
- Staging only: nothing is sent until the user presses SEND, as with
  per-item answers today, so a mistaken click is undone by changing or
  clearing the staged answers. Items the user already staged a different
  answer for are not overwritten.
- Only where answering is possible (the dashboard's live board with the
  answer form); the static/full file page and embed keep the chips as plain
  counts. The chip gets a pointer cursor, a hover state, and a tooltip
  ("모두 OK" / "추천안으로 모두 선택" style text through the existing i18n
  tables, every locale table updated like neighbouring keys), and a
  keyboard-reachable button role.
- Dashboard/whiteboard asset change: compatibility tier T0.

## Scope and verification

- Tests: extend the existing Go-driven node tests for answer staging (find
  them by `answers.js` / `writeAnswers` in `*_test.go`): ack chip stages ok
  on all open ack items of its topic only; policy chip stages the
  recommended letters, skips no-recommendation items, never overwrites an
  already-staged different answer; non-interactive pages render no button.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  before/after clicking each chip, light and dark, into
  scratchpad/topic-bulk/.

## Constraints

- Preserve the topic note float/carry-over, the sticky topic bar, the
  open-question pill, and answer carry-over across board swaps.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wswhiteboard/... ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/topic-count-bulk-answer` in this worktree; do
  not merge or push.
