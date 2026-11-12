# Plan: mockup of whiteboard `check` items rendered as a checklist under the recap

## Intent

User decision (2026-11-12, whiteboard (570) a): the whiteboard gains a fourth
item class `check` for a request that the user confirm or look at something
("review this draft", "verify on your machine", "OK to proceed?"). It is a
gate, not a decision, so it must not be bundled with announced/technical
acknowledgements. The user asked for a visual mockup only (no product code):
`check` items render as a checklist directly under the topic's recap, one
checkbox per item, ticked one at a time.

## Deliverable

A static mockup under `ai-docs/ref/whiteboard-mockup/r6-check-items/`
(follow the sibling `r3-outline`, `r4-dashboard`, `r5-status-band` layout):
an `index.html` plus a PNG screenshot of it, committed.

- Base the look on the real dashboard whiteboard styles
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`, the
  topic/recap/item markup in the dashboard templates and JS), so the mockup
  reads as the existing board with one addition. Copy what you need into the
  mockup; do not edit product assets.
- One topic with: a recap line (the existing inset recap block), then the
  check list right under it, then the topic body, then ordinary items
  (one announced, one policy) as today.
- Check list states to show, in Korean UI text: two unchecked items
  (waiting on the user), one checked (answered ok), one marked "not ok" with
  a short note (the user's `no` plus note via the existing note pen idea).
  Each row: checkbox, `(nn)` ID, one-line title, optional link chip.
- Show that ticking stages an answer the same way other board answers are
  staged before the send button (do not invent a new send flow).
- Make the unchecked checks count as "waiting on the user" in whatever topic
  header/badge the board already uses for open items.

## Constraints

- Mockup only: no changes to `agents-plugin*/` sources, playbooks, or Go.
- Verify the screenshot with headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright` works on this machine).
- Commit the mockup folder and this plan; `## AI Context` cites (570).
