---
title: "Dashboard agent count badge opens a popover that replaces the top-left agent list"
related:
  261112-feat-ws-dashboard-agent-panel-autocollapse: "idea ticket, likely superseded: removing the top-left list makes panel auto-collapse moot; confirm and drop at pickup"
  261109-feat-ws-dashboard-subagent-badge: "done; landed the agent count badge in the strip between conversation and composer that this ticket extends"
completed: 2026-11-16
---

# Dashboard agent count badge opens a popover that replaces the top-left agent list

## Background

Owner request on board item 1048 (2026-11-15): the conversation's agent count
badge, shown in the thin strip between the conversation and the composer, is
already implemented. Pressing it should open a popover that replaces the
top-left "agent list" panel, and the conversation gains the freed vertical
space. Owner quote, verbatim: "네 지금 대화 입력창이랑 대화창 사이의 얇은 공간 위에 뜨는 에이전트 숫자 뱃지까지는 구현이 되어있는데, 그 숫자 뱃지를 눌렀을 때 뜨는 팝업으로 지금의 좌상단 "에이전트 리스트"를 대체하자는 취지로 기억합니다."

Approved mockup images are in
`ai-docs/tickets/assets/261115-feat-ws-dashboard-agent-badge-popover/`:
`before.png`, `after-closed.png`, `after-open.png`,
`after-open-expanded.png`, `after-open-light.png`.

- Closed: the top-left list is gone; the badge reads "2 실행 중".
- Open: popover above the badge with a sticky header (running / done /
  failed / mail / background counts, pin, close); only the body scrolls.
  Row order: lead, running, ended (done/failed), background tasks.
- Esc or an outside click closes the popover unless pinned.
- Clicking a row expands its last few output lines with a "전체 출력 열기"
  action.
- Colours and popover shape follow the dashboard values and the existing left
  session-list popover.

## Decisions

- (1053)=a: a pinned popover stays floating over the conversation, covering
  part of it; the pin state is remembered in the browser.
- (1054): the badge shows the running count only.
- (1055) announced + ok: narrow (tab) screens use the same popover, shrunk to
  the viewport width; the board tab keeps the badge and popover.
- Constraint from MAIN: every row the top-left list shows today must be
  reachable in the popover (no information loss). Nothing is removed beyond
  relocating the list.
- (1123)=a: implement after v0.50.0.
- Owner rule: the UI is built by a Claude agent and verified by screenshots
  against the approved mockup; GPT-built UI gets redone.
- Compatibility tier: T0. Frontend-only; no tool, protocol, store or
  manifest change. Pin state is browser-local storage with a new key.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`). Shipped dashboard text must be downstream-neutral.
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`).
- Implement after v0.50.0 ships; do not pull into the release.
- UI work by a Claude agent; acceptance by screenshot comparison to the mockup.

## Phases

### Phase 1: Badge popover replaces the top-left agent list

Make the existing agent count badge open the popover described above and
remove the top-left agent list panel, giving its height to the conversation.
Every row the old list showed must be present in the popover, in the order
lead, running, ended, background tasks. Implement close on Esc and outside
click (suppressed while pinned), the pin toggle persisted in the browser,
row expansion with recent output lines and the full-output action, and
the shrink-to-viewport behaviour on narrow screens and in the board tab. New
user-visible strings go through the dashboard i18n tables.

Acceptance:

- Screenshots at 1440 and 700 px wide, light and dark, compared with the
  mockup images (closed, open, expanded).
- Browser tests for open, close, pin persistence, Esc, outside click, row
  expand, and narrow-width fit.
- i18n entries for all new strings in every shipped locale.
- No row from the previous top-left list is missing; T0 compatibility holds.

### Result

The agents region (`r-tree`) moved, unchanged as a live region, into a
`#agpop` popover inside the composer. The running-agent badge became a
button that toggles it. The grid lost the tree panel and its splitter, so
the conversation column now starts at the top. Because the region is
server-rendered and only relocated, every row the old list showed stays
reachable. The browser test checks this against the server's own row keys
and peer count. Live swaps, keyed open state, tool-permission rows and the
board drawer wait work as before.

- Order: lead, then `Running · N`, `Ended · N` and `Background tasks · N`
  groups, then peer leads. Child agents stay nested under their parent.
- Close: Esc, an outside pointer-down, the badge itself, and an `×`
  button. Esc and outside clicks are ignored while pinned. The pin is
  stored in localStorage (`ws.dashboard.agpop.pin`) and the popover reopens
  on reload. The `×` button unpins and closes.
- Expansion: an open row shows its newest four batches in a tinted box,
  with an "Open full output" action that reveals every batch. Both states
  survive region swaps.
- Narrow screens and the board tab: the same popover, capped at
  `100% - 20px` of the composer width. The board tab keeps the badge and
  the popover.
- Badge: `N(+M) running`, the running count only. It stays visible but
  muted at zero, so the popover is always reachable, including for ended
  sessions.
- Deviations:
  - Background tasks are counts only. The snapshot carries no per-task
    rows, and adding them would leave T0.
  - The agents container query dropped from 559px to 439px so that rows
    stay on one line in the 500px popover.
- i18n: eight new keys in all 11 tables.
- Tests: `TestAgentBadgePopoverBrowser` covers open/close, the pin across
  reload, order, no row loss, expansion with full output, the narrow fit and
  the board tab. Existing session-page, files, worktree and explorer tests
  now reflect the tree living in the popover.

#### Edition (hover preview) - 2026-11-16

Owner (1306): the badge also opens the same `agpop` on hover. `dashboard-composer.js`
adds pointerenter/pointerleave on badge and panel: 150 ms intent delay opens,
200 ms grace after leaving both closes, only a hover-opened panel auto-closes;
a click on a hover-opened panel makes it click-owned (stays open), otherwise
click toggles as before; pin, Esc and outside press unchanged. Touch pointers
and `(hover: hover)`-less devices never open it. No second popover. T0.
Timing/flow checked with a node stub simulation; browser test not run.

