---
title: "Whiteboard one-line view: clicking a topic switches to it without leaving the view"
---

# Whiteboard one-line view: clicking a topic switches to it without leaving the view

## Background

Owner request (relayed by MAIN, 2026-11-15): on the whiteboard, with the
board toolbar's one-line view active (the leftmost collapse toggle,
`.board-overview`, `aria-pressed="true"`), clicking a topic to switch to it
exits one-line view. The owner wants the topic switch to apply while staying
in one-line view.

Current behavior (`agents-plugin-tool/internal/wsdashboard/assets/overview.js`,
`window.WsOverview`): Overview is a picker. Its capture-phase `click`
handler swallows every click that is not on the toolbar, an open popover, or
the palette, and calls `exit(c === "item" ? e.target : null)`: an item click
leaves the view and lands on the clicked topic or card, any other click
leaves the view and returns to the entry position. Inside the view, arrow
keys already move a roving focus (`rove(next)`) and keep the focused row
visible through the shared scroll writer (`keep`), without exiting; Enter on
the roved row exits to it. `enter` marks the entry topic with
`overview-here`, and `exit(null)` (toggle, Escape) lands on the stored entry
key `entry.key` at `entry.offset`.

## Open Decision Queue
last-id: (pending MAIN mint)

(pending) [open] [policy] What a topic click inside one-line view does while the view stays on.
- Context: "switch to a topic" has no visible meaning inside the view today beyond the exit landing; the arrow-key rove is the existing in-view move primitive.
- Alternative a (select): the click moves the roving focus and the `overview-here` marker to the clicked item and makes it the view's entry key (`entry.key`, with the toolbar clearance as `entry.offset`, persisted through `save()`), so a later toggle or Escape lands on that topic expanded; the list does not scroll unless the clicked row is partly hidden (the same `keep` rule arrow keys use). Enter on the roved row still exits to it.
- Alternative b: as a, and a second click on the already-selected item exits to it (click-to-select, click-again-to-open).
- Alternative c: as a, and the clicked topic is also scrolled to the top of the one-line list.
> Recommend a: it reuses the arrow-key rove and the existing exit landing, keeps one exit model (toggle, Escape, Enter), and is a few lines in `overview.js`; b adds a second exit gesture, c moves the list under the pointer.

(pending) [open] [announced] Only the item click changes; clicks outside items (rail capsules, empty board space), palette picks (`WsOverview.pick`), Enter, Escape, Ctrl+` and the toolbar toggle keep their current exit behavior - the owner's request names the topic click only.

## Phases

### Phase 1: Stay in one-line view on topic click

Change the `click` handler's `item` branch in
`agents-plugin-tool/internal/wsdashboard/assets/overview.js` per the settled
decision, keeping the handler's swallow (no click reaches the dimmed topic).
Update the Overview JS tests in
`agents-plugin-tool/internal/wsdashboard/overview_test.go` (and any fixture
asserting that an item click exits) to the new behavior: an item click keeps
`data-overview` and `aria-pressed="true"`, moves `overview-here` and the
roving `tabindex="0"` to the clicked item, and a following toggle/Escape
exit lands on that item.

Verification: `go test ./internal/wsdashboard -run 'TestOverview|TestBoardToolbar'`
from `agents-plugin-tool/` (offline env); never the full package. No new UI
strings, so no `wsi18n` change.
