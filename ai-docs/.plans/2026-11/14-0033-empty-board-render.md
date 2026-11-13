# Empty-board render glitch (ticketless UX fix, (527)=a)

Source: owner report relayed by MAIN, 2026-11-14 (board topic
`empty-board-render`). Screenshot:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/2af10a694d73bbff-1.png`.
It shows a session page with no whiteboard, in the wide layout where the idle
pin drop zone (`.bpzone`) is visible on the right.

## Symptoms

1. The left empty-board box ("이 세션에는 아직 화이트보드가 없습니다.") starts
   about 20px lower than the right `.bpzone` box ("여기에 놓으면 고정").
2. The left box's bottom is clipped: its dashed sides run to the viewport
   bottom with no bottom border and no rounded corners. The right box is
   intact.
3. The header buttons at the top right are pushed up and clipped; only their
   bottom edges show. The page scrolls slightly.

## Likely cause (verify; do not trust)

- `dashboard.css` around lines 1488-1490 sets `.r-board` and its `.empty` to
  `height: 100%`.
- That rule came from 05447f96e. 9b2ab99bb (fix/ux-postswap-css-regressions,
  merged at 65a917d1b) made it match again when an empty `#bpins` is the
  first child, so this glitch is the now-live form of that fill.
- `height: 100%` of a container that has a top offset (padding, header, or
  rail inset) overflows by that offset. The column clips the bottom, and the
  document grows, which pushes the header up.
- `.bpzone` is `position: fixed; top: 50px; bottom: 12px`
  (dashboard.css ~1547/1552), so its geometry is independent of the board's.

## Required outcome

- In the wide layout with `.bpzone` visible, the empty box's top and bottom
  edges line up with `.bpzone`'s (within 1px). Both boxes have all four
  dashed sides and rounded corners.
- The page does not scroll: `document.scrollingElement.scrollHeight` equals
  the viewport height, and the header buttons are fully visible.
- Layouts where `.bpzone` is not shown (narrow, a pinned column present, the
  tabs layout, `.dash.live`): the empty box fills the free height of its
  column without overflow. If the box cannot fill there, it must at least not
  overflow; report which case you chose.
- Make the empty-board selector tolerant of a future first child: a planned
  board-toolbar card will become the board's first child. The rule should key
  on "the board has no topics or cards", not on child position. Keep
  `.bpins:empty` hidden.

## Boundary

- Prefer a CSS-only fix in `wsdashboard/assets/dashboard.css`.
  - The fill should come from flex or grid sizing of the board, or from a
    viewport-based height that subtracts the real offsets, not from bare
    `height: 100%`.
  - Touch JS only if CSS cannot express it, and then return to the lead
    first with the reason.
- Do not change topic or pin behavior. A perf worker is editing the board
  assets in parallel, so keep the change surgical.
- Do not touch the shared daemon:
  - No `scripts/dev.py`.
  - No restart, shutdown or kill of anything.
  - For headless checks, either run your own daemon or use the live one
    (127.0.0.1:29417) GET-only with every non-GET aborted, injecting the
    branch CSS with `addStyleTag`.
  - Your own daemon: build `ws-mcp` to `/tmp` and run
    `WS_CACHE_HOME=<tmp> <bin> dashboard --port 0`. Stop it by PID when done.

## Verify

- Headless Chromium, before and after, at 2560x1400 and 1900x1000. At 1400x900
  and 420x800 check that the narrow-layout fallback does not overflow. Use a
  session page with no whiteboard topics (e.g. a session that has none).
  - Report each box's top, bottom and height, the page scrollHeight against
    the viewport, and whether the header buttons are clipped.
- A board with topics is unchanged: same layout metrics before and after.
- `go test ./internal/wsdashboard/... ./internal/wswhiteboard/...` from
  `agents-plugin-tool`. The CSS collision guard must stay green.
