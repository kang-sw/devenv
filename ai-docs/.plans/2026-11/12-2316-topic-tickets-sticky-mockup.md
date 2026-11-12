# Plan: mockup of topic-linked tickets (live glow) and a sticky topic header carrying recap, ticket badges, and a collapsed check list

## Intent

User decisions (2026-11-12, whiteboard topic `topic-tickets`):

- (595) a: a lead links ticket stems to a whiteboard topic explicitly (a
  `tickets` attribute on `wb:topic`); no auto-detection.
- (596) a: mockup first, then a ticket.
- The topic shows its linked tickets as status-coloured chips; a topic with
  at least one live ticket (idea, todo, or ready; not .done/.dropped) gets a
  soft glow on its card.
- User note: the recap, the ticket list, and the check list should follow
  the topic's sticky header while scrolling through a long topic. The check
  list follows **collapsed** (e.g. a one-line "check 2 / 4 open" summary that
  expands on click). The ticket list renders as **badges stacked to the
  right, wrapping**, so the sticky header gains as few rows as possible.

Mockup only, no product change.

## Deliverable

`ai-docs/ref/whiteboard-mockup/r7-topic-tickets-sticky/` (follow the sibling
`r6-check-items` layout): `index.html` plus PNG screenshots, committed.

- Base it on the real board assets (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`,
  `page.css`, and how the dashboard embeds them) and on the r6 mockup's
  check list (`ai-docs/ref/whiteboard-mockup/r6-check-items/index.html`),
  so it reads as today's board plus r6 plus this change. Find how the topic
  bar is sticky today and extend that.
- Show: (1) a topic at rest (top of the page): topic bar, recap, ticket
  badges right-aligned in the bar or recap row, check list expanded under
  the recap, body, items; (2) the same topic scrolled so its header is stuck:
  bar + recap + badges + the check list collapsed to one line; (3) a narrow
  width (~520px) with 3-4 badges wrapping; (4) glow on a topic with a live
  ticket next to one whose tickets are all done (no glow), and one with no
  tickets. Badge text: short stem tail or `#<date>-…` form plus a status
  colour (idea / todo / ready / done / dropped); decide what reads best and
  note it.
- Dark and light theme if the real CSS supports both (light-dark()).
- Keep the glow subtle; note the CSS used.

## Constraints

- Mockup only: no changes to `agents-plugin*/` sources.
- Screenshots via headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`), including a scrolled screenshot
  for the stuck state; no page errors.
- Commit the mockup folder and this plan; `## AI Context` cites (595), (596).
- Return the PNG paths and any choice you made that the user should confirm.
