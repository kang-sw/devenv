# Plan: board sticky topic headers with jump capsules

## Request

User, 2026-11-10 whiteboard (topic "보드 토픽 점프 (sticky 헤더)"): in the
dashboard's right board column (the topic card stack), the header of the
topic being read should stick and follow while scrolling through its body,
and compact jump rows for the neighbouring topics should sit above and below
it; clicking one jumps to that topic's header. The user approved mockup v2
("이렇게 구현하는 걸로 해주세요") and asked to delegate it in parallel with the
ws 0.48.14 release (this branch merges after the release tag).

## Confirmed design (mockup v2)

- **Current topic header sticks at its original size**: the same `.tbar`
  look (30px min-height, 14px bold name, ago text, chips), top rounded corners
  matching the card, a light drop shadow while stuck. "Current" = the topic
  whose card spans the top edge of the scroll viewport.
- **Jump capsules**: the neighbouring topics' headers, squashed vertically
  into long capsules in the topic header colour (`--wb-topic` /
  `--wb-on-topic`, `border-radius: 999px`, about 15-17px tall, 10-10.5px
  text, single line with ellipsis).
  - Up to **2 above** the stuck header (the previous topics) and up to **2 at
    the bottom** of the viewport (the next topics not yet fully in view).
  - The capsule nearest the current topic is wider and stronger; the farther
    one is narrower (larger side margin) and fainter (lower opacity), giving
    a stacked-depth look.
  - The outermost capsule shows `+N` at its right end when more topics lie
    beyond it in that direction.
  - A capsule shows the topic's waiting-policy count as a small pill in the
    policy colour (`--wb-policy` / `--wb-on-policy`) when nonzero (the topic's
    `.chip.policy` count).
  - Bottom capsules float over the content with a gradient fade to
    `--wb-bg` behind them.
  - Clicking a capsule scrolls that topic's header to the top of the
    viewport (below the top capsules).
- At the top of the list (nothing above) there are no top capsules; at the
  end, no bottom capsules.
- Styling uses only existing `--wb-*` tokens; works in light and dark themes.

## Scope and constraints

- Frontend only. The board column is `#bcol` (`.bcol`, `overflow: auto`) in
  `internal/wsdashboard/assets/dashboard.tmpl` / `dashboard.css` (~:473);
  topics are `section.topic[data-topic]` with `.tbar` from
  `internal/wswhiteboard/assets/page.tmpl` (styles in `page.css`).
- Board swaps replace or morph the board region (`dashboard.js` morphBoard /
  region swap); the rail must rebuild after a swap and must not accumulate
  listeners, observers, or DOM across swaps (the 261110 perf ticket just
  removed that class of growth; keep scroll work rAF-throttled and cheap).
- Quiet / closed topics: include them as capsules only if it reads well;
  your call, state it in the commit's AI Context. Collapsed closed topics past
  the cap ("show more") need not appear.
- The standalone whiteboard file page (page.tmpl without the dashboard) is
  optional: add it only if the same code applies with no extra structure.
- Keyboard/a11y: capsules are buttons with the topic name as label.
- Do not change server-side data or Go rendering unless a tiny template hook
  is strictly needed; if so, keep it to markup the script reads.

## Verification

- `node --check` on every touched JS file (nice'd); existing node-run asset
  tests, if any touch these files, run headless under `nice -n 19`.
- Go tests only remotely: `~/.local/bin/ws-rgo -C
  /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool
  -m 30 -- test ./internal/wsdashboard/ ./internal/wswhiteboard/ -count=1
  -timeout 30m` (asset embedding / template tests). Never run go
  build/test/vet locally (it freezes the machine).
- Visual check: if you can render the dashboard page headlessly with an
  isolated dev daemon (separate WS_CACHE_HOME / WS_CONFIG_HOME, its own
  port), screenshot both the mid-scroll and top-of-list states. Never touch
  processes or ports 29417 / 45743 (the user's daemons, including Windows
  PID 40472).
- Do not push. Commit on branch `feat/board-sticky-topic-headers`.
