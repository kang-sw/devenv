# Plan: dim a topic body's change bars once the topic moved on

## Intent

User request (2026-11-12, whiteboard topic `chg-dim`, items (582), (583)):
a topic body's change bars (green new / blue mod / red del, drawn by
`agents-plugin-tool/internal/wswhiteboard/assets/chg.js` from the marks of
the body's latest edit, `TopicDoc.Marks`) stay fully bright until the next
body edit. When the topic's other content changes after that edit, the bars
should stay but be dimmed, reading as "changed, but not just now".

- (582) Stale rule: the marks are stale when any item of the same topic has
  `Item.Changed` greater than the body's `TopicDoc.Changed` (an item added,
  patched, answered, or closed/withdrawn after the body edit). A recap change
  (`RecapChanged`) does not count. Time-based fading was rejected.
- (583) Dimmed = the bars at roughly 35% opacity; never removed. The next
  body edit produces new marks and they render at full strength again
  (naturally, since the body's `Changed` then exceeds the items').

## Shape

- View-only: compute a boolean beside `DocMarks` in `view.go` (topic block
  building near `block.DocMarks = doc.Marks`) and carry it to rendering
  (`render.go` where `t.DocMarks` is emitted) as a class or attribute that
  `chg.js` / `page.css` use to dim the `.wb-chg-layer` bars. No store or
  wire change (T0). The dashboard embeds the wswhiteboard assets, so it
  follows; check that the dashboard's own topic rendering path (if it builds
  the body markup separately) gets the same flag.
- Live updates: when the page re-renders or receives a live patch, the flag
  must follow (check `live.js` / how the dashboard swaps topic bodies).

## Verification

- Go tests in `internal/wswhiteboard` for the flag: body edit then no item
  change -> fresh; item added/answered/patched after the body edit -> stale;
  recap-only change -> fresh; a later body edit -> fresh again.
- Existing `go test ./internal/wswhiteboard/ ./internal/wsdashboard/`.
- Headless Chromium screenshot of a rendered board with one fresh and one
  stale topic (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`);
  no page errors. Save the PNG under `/tmp` and report its path.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.
- Do not touch the item classes or the recap/check work
  (`261112-feat-ws-whiteboard-check-items`, separate ticket).
