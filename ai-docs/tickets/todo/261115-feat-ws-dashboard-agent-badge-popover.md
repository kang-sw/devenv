---
title: "Dashboard agent count badge opens a popover that replaces the top-left agent list"
related:
  261112-feat-ws-dashboard-agent-panel-autocollapse: "idea ticket, likely superseded: removing the top-left list makes panel auto-collapse moot; confirm and drop at pickup"
  261109-feat-ws-dashboard-subagent-badge: "done; landed the agent count badge in the strip between conversation and composer that this ticket extends"
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
