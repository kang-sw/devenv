# Whiteboard: mention capsule from a text selection; recap item references resolve

## Goal

Two user requests from 2026-11-12, settled on the board (whiteboard topic
`wb-mention`, items (383)-(389)):

1. Bug: a topic recap shows `{{nn}}` item references raw instead of `(nn)`
   (a linked chip when the item is on the page), unlike item fields and topic
   bodies.
2. Feature: on the dashboard's live board, the user selects text inside a
   topic card and adds it to the composer as a "mention" capsule. The message
   sent to the lead carries it as a `wb:mention` element.

## Decisions (settled; do not reopen)

### Recap references
- The recap renders through `RecapHTML` -> `itemField`
  (`agents-plugin-tool/internal/wswhiteboard/render.go` ~538), which today
  does not resolve `{{nn}}` the way item fields / topic bodies do. Make the
  recap resolve references exactly as the whiteboard playbook already
  promises for item fields (`(12)`, linked when the item is on the page,
  verbatim when the store has no such item). Check the dashboard's own recap
  rendering paths too (full page, embed, dashboard morph). No playbook change
  is needed for this part (it already documents the behaviour).

### Mention capsule
- Gesture ((388) a): selecting text inside a topic card on the dashboard's
  live board (the page with the answer form) shows a small floating "멘션"
  button next to the selection (localised through the existing i18n tables,
  every locale like neighbouring keys). Clicking it adds a capsule to the
  composer and clears the button. No button on the static/full file page or
  embed. The button hides when the selection collapses or leaves the card.
- No content-less mentions ((389) b): an empty/whitespace-only selection
  shows no button.
- Sent format: `<wb:mention topic="<topic stem>" id="<N>">content</wb:mention>`
  - attributes quoted; `topic` is the topic's stem (or its key/name when the
    topic has no stem, as the topic note pen names it);
  - `id` only when the whole selection lies within one item card, otherwise
    omitted (topic only);
  - content escapes `&`, `<`, `>` (as `&amp;`, `&lt;`, `&gt;`).
- Placement: the mention elements go after the `</wb:answers>` block (when
  present) and before the user's typed text, in the order added.
- Capsule UI in the composer: same capsule family as the existing answer
  capsules (`dashboard.js` chips / `answers.js`), showing the topic (and
  `(N)` when present) plus a short excerpt; removable like other capsules.
- Chat history: a sent message's `wb:mention` elements are revived as
  capsules in the conversation view, the same way the `wb:answers` block is
  parsed back (`agents-plugin-tool/internal/wsinbox/answers.go` parse side and
  whatever the dashboard uses to render history capsules). Unescape the
  content for display.
- Playbook: document `wb:mention` in the whiteboard playbook's Answers section
  (what the lead receives and how to treat it: the user is pointing at that
  topic / item text as context for their message; no answer recording
  needed), in all three copies: `agents-plugin/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`; update any manifest sha /
  mirror test those files are pinned by (search tests for the path).
- Compatibility tier T0: additive message content plus dashboard assets and
  playbook text.

## Scope and verification

- Files (expected): `internal/wswhiteboard/render.go` (+ tests),
  `internal/wswhiteboard/assets/answers.js`, `internal/wsdashboard/assets/dashboard.js`
  / `dashboard.css` (or the whiteboard page.css if the button lives there),
  `internal/wsinbox/answers.go` (+ tests), i18n tables, the three playbook
  copies and their pins.
- Tests: Go test that a recap with `{{nn}}` renders `(nn)` (linked when the
  item exists, verbatim when not); node-driven test (existing answers.js /
  writeAnswers harness in `*_test.go`) for building the mention string
  (escaping, id only within one item, ordering after `</wb:answers>`, no
  empty mention); wsinbox parse test for reviving mentions from a message.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  select text in a topic body and in an item card, the floating button, the
  composer capsule, a recap with a resolved reference; light and dark; shots
  into scratchpad/wb-mention/.

## Constraints

- Read `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing the files they
  cover (AGENTS.md Implementation Conventions).
- Preserve: answer capsules and their order, topic note pen float/carry-over,
  answer carry-over across board swaps, the `wb:answers` format.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test -count=1 ./internal/wswhiteboard/... ./internal/wsdashboard/... ./internal/wsinbox/... ./internal/wsi18n/... ./internal/mcp/...`
  (and the package tests that pin the playbook copies); never run go
  build/test/vet locally. Note: `internal/mcp` TestDashboardReadPageServesTheBoard
  already fails on develop; do not chase it unless your change touches it.
- Commit on branch `delegate/wb-mention-recap-refs` in this worktree; do not
  merge or push.
