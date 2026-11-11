# Whiteboard board scroll and idle performance

## Outcome

The user reported the dashboard whiteboard board feels janky. A headless
CDP measurement of the live board (37 topics, 218 items, ~4,600 nodes in the
board column; report and traces in
`/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/perf/`)
found:

- Scroll: p95 33 ms, max 67 ms frames. The cost is
  `agents-plugin-tool/internal/wswhiteboard/assets/rail.js` `draw()`:
  it writes `--wb-rail-top` / `--wb-rail-h` (and `--wb-rail-bot`) on the
  board host, then reads `offsetHeight`, forcing a style recalc over the
  whole board (one custom-property write+read on `#bcol` = 10.7 ms vs
  0.42 ms on a small element, because custom properties inherit into every
  node). A second forced site writes `paddingBottom` then reads
  `bar.offsetHeight`. The capsule morph animations read
  `getBoundingClientRect` right before `animate`, forcing another ~80 ms per
  full scroll.
- Idle: the `rail-orbit` "working" ring animates `stroke-dashoffset` on an
  SVG rect (`wsdashboard/assets/dashboard.css` ~L1365), forcing a main-thread
  frame and style recalc every frame (~9% of a core) whenever any rail
  session is working, which is the normal state. `dash-spin`, `rail-blink`,
  `dash-pulse` cost less but also tick.
- Minor: `dashboard.js` pane scroll listener calls `visible()`
  (`getClientRects`) on every scroll event (~43 ms per full scroll).

Implement, in this order of priority:

1. `rail.js` `draw()`: stop writing the `--wb-rail-*` custom properties on
   the board host. Put each value only on the elements that consume it
   (find every consumer of `--wb-rail-top`, `--wb-rail-h`, `--wb-rail-bot`,
   `--wb-rail-inset` across wswhiteboard and wsdashboard CSS/JS, e.g. the
   sticky topic bars, `.oq-wrap` / pill offset, board-card headers, and any
   host-provided rows), or use a non-inherited mechanism (inline
   `style.top` on the consumers, or a registered non-inheriting property via
   `CSS.registerProperty({inherits:false})` if consumers can read it), and
   batch all layout reads before any style writes in a frame. Skip writes
   whose value did not change. Keep behaviour identical: same stick
   positions, same capsule placement, same pill bottom offset.
2. `rail-orbit`: replace the `stroke-dashoffset` animation with a
   compositor-friendly one (e.g. `transform: rotate` on the ring element or a
   masked conic-gradient), keeping the look close (a short arc orbiting the
   ring at the same period) and the existing reduced-motion off switch. Give
   the remaining infinite spinners (`dash-spin`, `rail-blink`, `dash-pulse`)
   transform/opacity-only keyframes if they are not already, so they stay on
   the compositor.
3. Rail capsule morph: read all rects first, then start animations, and only
   animate on a real change of capsule membership.
4. `dashboard.js` pane scroll listener: avoid the forced `getClientRects` per
   scroll event (cache visibility, or record the offset in a rAF or on
   `scrollend`). Do NOT touch the topic-note code around L770-810 (a peer
   session edits it).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (the
  assets ship).
- Behaviour-preserving: the existing rail, pill, topic-motion and
  closed-topic-fold node tests must keep passing; add or update node tests
  where a changed contract (which element carries the offset) is pinned.
- Never run go build/test/vet locally: use
  `~/.local/bin/ws-rgo -C <worktree>/agents-plugin-tool -m 25 -- test -count=1 ./internal/wswhiteboard/... ./internal/wsdashboard/...`.
- `node --check` every touched JS file.

## Verification

- Tests above pass, node-run tests not skipped.
- Re-run the perf driver from the scratchpad (it can target a local file
  page or the live board read-only, as before; never click answer options
  or send) before and after, and report the frame-time table and forced
  recalc counts for idle and scroll. A visual check (screenshot) that the
  rail capsules, sticky bars, pill and working ring still look right.

## Return

Commit on `delegate/wb-board-perf` (this plan rides the commit). Do not merge
or push. Report commits, what changed per item, before/after numbers, and
anything unverified.
