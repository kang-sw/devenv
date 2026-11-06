# whiteboard Now placement

User feedback (2026-11-06, whiteboard dogfood): the Now slot jumping between
above and below the topics is poor UX, and its summary bar shows only an icon.
User-directed changes, whiteboard items (163) and the follow-up message:

## Assignment

1. **Always on top.** Render Now above the topics on both the full page and the
   embed, every render. Today `View.NowAbove` (`wswhiteboard/view.go`, from
   `State.NowChangedThisRender`) moves it below on renders that did not change
   it; the `board` template in `wswhiteboard/assets/page.tmpl` has the two
   placements.
2. **Label.** The Now summary bar shows a "NOW" text label next to its icon
   (the icon alone looks empty).
3. **Fresh vs stale.** Now is *fresh* when the latest `whiteboard.update`
   changed it, and *stale* otherwise. Fresh: `<details open>`, styled as
   today. Stale: rendered collapsed (no `open`) and greyed out (muted summary
   colors), still expandable by click.
   - The implicit full-page refresh after todo/agenda writes
     (`refreshWhiteboardAfter` in `mcp/whiteboard_tools.go`, which calls
     `State.BeginRender` and bumps `Seq`) must not turn a fresh Now stale:
     staleness is judged against the latest `whiteboard.update`, not against
     every render. A minimal store addition (for example the seq of the latest
     update) is allowed for this; keep the JSON backward compatible (an absent
     field reads as today's state without error).
   - Keep the embed's "new since the previous embed" dot on Now as it is.
4. **Playbook text.** `agents-plugin/rsrc/whiteboard/whiteboard.md` (near line
   87) says Now sits "above the topics on the update that changed it and below
   them after". Rewrite that to the new behavior (always above; collapsed and
   greyed when the latest update did not change it). Before editing, read
   `ai-docs/manuals/shipped-surface-boundary.md`,
   `ai-docs/manuals/skill-authoring.md`, and
   `ai-docs/manuals/wsflow-mirroring.md`; mirror into
   `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` and regenerate any
   manifest the mirroring manual requires. Also read `ai-docs/manuals/ws-mcp.md`
   before editing `internal/mcp`.
5. Remove code that only served the below-placement once it is dead
   (`NowAbove` and its comments, or repurpose it as the fresh flag).

## Verification

- Render tests in `internal/wswhiteboard`: Now precedes the first topic on a
  render that did not change it (full and embed); stale Now has no `open` and
  carries the stale class; fresh Now is open; the label text is present.
- An `internal/mcp` test: after an update that sets Now, a todo or agenda write
  re-renders the full page with Now still fresh (open); the next update without
  `now` renders it stale.
- `go test ./internal/wswhiteboard -count=1`, `go test ./internal/mcp -run
  'Whiteboard' -count=1`, then the package and manifest tests the mirroring
  manual names. The full `go test ./...` is slow (internal/mcp over 10
  minutes); run it if time allows and say whether you did.
- No headless browser is installed; say the page was not visually checked.

## Return boundary

Stop and report before changing the whiteboard tool input schema, the reply
grammar, or playbook text beyond the Now sentence(s).
