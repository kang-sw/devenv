# Delegation plan: whiteboard topic motion

Source: user request, 2026-11-10 whiteboard topic "멀티 세션 관리 UX", item (35) ok:
"새로 등장하는 화이트보드는 매끄럽게 끼어들어서 늘어나고, 토픽이 갱신되면 새 넓이로
매끄럽게 보간되고요" (a newly appearing topic slides in and grows; an updated topic
interpolates smoothly to its new height).

## Outcome

On the session page's board region (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`,
`swap()` -> `morphBoard()`):

1. A topic section (`section.topic[data-topic]`) that appears on a board swap and was not on
   the board before grows from height 0 to its natural height (an "insert" slide-in), so the
   topics below it are pushed down smoothly instead of jumping.
2. A topic whose section is replaced because its HTML changed (morphBoard replaces the node;
   it never morphs in place) animates from the old section's rendered height to the new
   section's natural height.
3. A topic that leaves the board may simply disappear (no exit animation required); a topic
   that only moves position needs no FLIP animation in this change.
4. No animation:
   - on the initial page load (the board as served);
   - when the swap was deferred because the user was typing (`editing()` / `deferred`) - when
     the deferred swap lands it applies without animation, so the board never moves under a
     just-blurred input;
   - under `prefers-reduced-motion: reduce`;
   - for closed-page reconciliation (topics moved between the capped closed section and the
     fetched closed pages).
5. After the animation the section returns to its natural, unconstrained height
   (no fixed `height` or `overflow` left behind), so later content changes (iframe body frames
   resizing via `setFrameHeights`, details toggles, images loading) are not clipped.
6. A swap that arrives while a previous animation on the same topic is still running starts
   from the currently rendered height, not from the stale target.

## Constraints

- Keep morphBoard's existing invariants: kept nodes are never moved (moving a body iframe
  reloads it); `boardSeen` comparison semantics unchanged; the returned `ins` list unchanged
  in content and order (callers prepare these nodes).
- Topic body iframes restore their height through `frameHeights`/`setFrameHeights` after
  insertion; measure the target height after the caller has prepared the inserted nodes
  (after the `ins.forEach(...)` block in `swap`), or animate in a way that tolerates the
  natural height changing during the animation.
- Prefer the Web Animations API (`el.animate`) over CSS class juggling; keep durations short
  (around 180-240ms, ease-out). Any CSS needed goes in `assets/dashboard.css`, with a
  `prefers-reduced-motion` guard in the existing style.
- Do not touch the topic-note code (`dashboard.js` `topicNotes` / note pen handlers, roughly
  L770-810 and L1097 on develop): another session's ticket
  (261110-bug-ws-whiteboard-query-output-noise) may edit it; overlapping hunks there would
  conflict.
- Surgical change; follow the file's existing style (var, function expressions).
- Manual: `ai-docs/manuals/shipped-surface-boundary.md` (the dashboard is shipped surface).

## Verification

- JS behavior test run under node in the existing style (`runNode` helper; see
  `toast_test.go`, `topicnote_chip_test.go`, `stream_page_test.go` for how dashboard.js logic
  is exercised): cover that the animation decision is skipped on initial load, on a deferred
  (editing) swap, and under reduced motion, and taken for a new topic and a changed topic
  with the right start height. If exercising `animate` without a DOM is impractical, factor
  the decision ("which inserted topic nodes animate, and from what start height") into a
  small pure function and test that.
- Heavy Go tests run remotely only:
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- test ./internal/wsdashboard/...`
  (never run `go test` / `go build` locally on this machine).
- The browser check is the lead's; report what a reviewer should look at.

## Return boundary

Commit on branch `feat/wb-topic-motion` in this worktree (do not merge, do not push). Stop
and report instead if the change needs server-side template or Go changes beyond tests, or
touches the topic-note code.
