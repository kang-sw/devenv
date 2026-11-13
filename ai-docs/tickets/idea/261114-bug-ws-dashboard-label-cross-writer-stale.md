---
title: "Dashboard: a label saved on one page goes stale when another writer sets it back before the render"
related:
  261113-feat-ws-dashboard-session-rail-tree: "found in its Phase 4 harness run; Forward note in 4d760277c and b710d81ce"
---

# Dashboard: a label saved on one page goes stale when another writer sets it back before the render

## Problem

A page that saves a session label (the rail row pencil, the row menu's
rename, or the head rename) shows the new label at once, before any render
of it arrives.

If another writer sets the label back before the daemon sends a render
between the two writes, the next render matches the hash the page still
holds for the old content. The page therefore skips that render. The other
writer can be another browser, a second rename, or an MCP or API caller.

The page keeps showing the stale label until something else changes the
rail.

## Evidence

- Phase 4's `menu` scenario in `scripts/rail-headless.mjs` hit this reliably
  when it reset a label right after renaming it. The scenario now waits for
  the rename's render before it resets the label (commit 4d760277c, Forward
  note).
- A client-only fix was tried: after a local save, forget the rail and head
  `data-h` so that the next render always lands.
  - It failed because the daemon dedupes each stream against the render it
    last sent there. It never re-sends a render equal to that one, whatever
    the page holds now.
  - The fix was reverted (commit b710d81ce, `## AI Context`).
- Reproduced on the rail. The head is inferred to behave the same from the
  same code pattern (`labelSave` in `assets/dashboard.js`), but this was not
  reproduced.

## Directions to weigh

- After a local save, the page asks its stream for a fresh render: a resync
  message, or a reconnect carrying an empty `held()` hash for the region.
- The label route's answer carries the new render generation, and the page
  refuses to treat a render with an older hash as current. The rail layout
  ops already do this with `data-lgen` and `staleRail`.
- A label revision joins the rail and head stamps, so setting the label back
  produces a new hash even when the content is the same as before.

Impact is low: it needs two writers racing within one render interval, and
the next rail change fixes it on its own.
