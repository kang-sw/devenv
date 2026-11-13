---
title: ws dashboard iframe shell
related:
  261113-feat-ws-dashboard-session-four-state-lifecycle: sidebar contents
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: perf ticket; session-switch baseline
  261113-feat-ws-dashboard-browser-pane: also turns sessionPageCSP into a per-request policy (frame-src for the proxy origin)
  261113-feat-ws-dashboard-worktree-delete: its rail row action and modal live in the rail this ticket moves into the shell
  261113-bug-ws-dashboard-pinned-panel-sticky-header: blocked by this ticket
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1ad6563f62410a0a
sage-review-completeness-reviewed: 1ad6563f62410a0a
---

# ws dashboard iframe shell

## Background

Current behavior (from the lead's earlier investigation):

- Every dashboard URL change reloads the whole document. Sidebar links are
  plain links; 13 script tags (12 without the optional terminal script; about
  16.9k lines across the asset scripts, plus the whiteboard answers/viewer and
  i18n scripts) are re-executed and the WebSocket reconnects
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L25-L42`;
  `new WebSocket` at `assets/dashboard.js#L5321`). The owner observes 500-1000 ms per session switch.
- Sidebar collapse state lives only in browser storage and the server always
  renders the sidebar expanded, so the last script collapses it after paint,
  producing a visible flicker. Pin and order apply at the same late point. The
  "ended" collapse is not persisted at all.

Options considered earlier (board dash-nav):

- (a) Flicker fix only: 0.5-1 day.
- (b) View Transitions: 1 h; smooth white flash, reload cost unchanged.
- Iframe shell: 3-5 days; the shell persists and recent sessions switch
  instantly via LRU.
- (c) Swap body only: 1-2 weeks; needs splitting the ~6.9k-line `dashboard.js` (6926 lines, `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`).
- (d) Full SPA: 1-2 months+; permanently splits rendering between server and
  client.

Chosen direction: iframe shell as the final structure. It fits the
server-rendered, no-build-step, single-exe, no-node-on-remote design and does
not block a later SPA. Rejected: (c) and (d) for cost; (b) alone because it
leaves the reload cost.

Owner (42) answer earlier: hold implementation until the UX design discussion
(e.g. in wide mode the sidebar pinned to the far left edge). That discussion is
now open.

## Decisions

- Architecture: all data goes through the daemon. The shell and each frame open
  their own WebSocket to the daemon; `postMessage` is only for UI control
  (navigate, title, badges, notifications).
- Costs to handle: (1) per-connection duplicate region rendering - render once
  per session and fan out by hash cache; the sidebar is subscribed only by the
  shell. (2) Hidden frames get no `visibilitychange` - the shell tells them so
  they lower update frequency. (3) Per-frame JS memory - LRU cap.
- (418) Shell contents: sidebar plus top header (global search, notifications,
  session title/badges); the agent panel stays in the frame. Owner answer
  2026-11-13. Rejected: a sidebar only; c agent panel in the shell (it is
  per-session content).
- (419) All dashboard pages (tracker, wiki, ...) open as frames inside the
  shell, so the sidebar never re-renders on any navigation. Owner answer
  2026-11-13. Rejected: a session pages only.
- (420) Narrow/mobile keeps today's overlay sidebar. Owner answer 2026-11-13.
  Rejected: b top dropdown session picker.
- (421) Keep the 5 most recent frames (LRU), no config key. Owner ok
  2026-11-13. Rejected: unlimited (each frame holds its own JS heap).
- (422) Address bar keeps `/s/<id>`; frames load the same path with
  `?frame=1`; the shell updates the URL with `history.pushState`. Owner ok
  2026-11-13. Rejected: a new URL scheme (breaks links, bookmarks, embeds).
- (423) Sidebar flicker fix ships in shell Phase 1. Owner ok 2026-11-13.
- (501) How the flicker fix applies the sidebar state before first paint is
  the implementer's call (owner, 2026-11-13: the question was dropped and left
  to the shell implementation). Fact: the collapse/pin/order state lives only
  in browser localStorage today (`rail.js`: "nothing reaches the server"), so
  (423)'s original wording "in the server render" is not binding; candidates
  are an inline pre-paint script in the shell head, a cookie the server render
  reads (watch cookie size for order lists), or another mechanism. Any new
  store shape or protocol surface stays out (T0 constraint). Name the chosen
  mechanism in the Result.
- (503) Framing headers (technical, owner-acknowledged 2026-11-13): responses
  to `?frame=1` requests send `Content-Security-Policy: frame-ancestors
  'self'` and `X-Frame-Options: SAMEORIGIN`; every other response keeps
  `frame-ancestors 'none'` / `DENY` (today every page sends DENY,
  `agents-plugin-tool/internal/wsdashboard/pages.go:1265-1291`, which blocks
  the shell outright). Foreign origins still cannot frame any page.
  Rejected: relaxing framing on every response (widens clickjacking surface).
- **Cross-document input and links** (design review 2026-11-13, autonomous):
  the postMessage set also carries key forwarding from a focused frame to the
  shell for the shell-owned hotkeys (rail hotkey Ctrl+` on `#railbtn`, global
  search focus), preserving the fd4fc2be rail hotkey handover; a click inside
  a frame on a link to another dashboard page that the shell frames is
  intercepted and sent as a navigate message, never loaded as a plain page
  in the frame (which (503) refuses). The message shapes are the
  implementer's, recorded in the Result. Rejected: leaving hotkeys
  frame-local (focus inside a frame would swallow the rail hotkey).
- **Phase 1 non-session links** (design review 2026-11-13, autonomous): until
  Phase 2, links to non-session pages (tracker, wiki, ...) navigate the top
  level, leaving the shell. Rejected: framing them before they get
  `?frame=1` headers (the frame would be refused).
- **Composed session-page CSP** (coherence review 2026-11-13, autonomous):
  this ticket and `261113-feat-ws-dashboard-browser-pane` both turn the
  constant `sessionPageCSP` (`pages.go:1276`) into a per-request policy in
  one function and both touch `TestSessionPageFramesOnlyItsOwnOrigin`. After
  this ticket, `/s/<id>` without `?frame=1` serves the shell; the `?frame=1`
  response is the document that will host browser-pane cards. Whichever
  ticket lands second carries the other's directive into that function: the
  `?frame=1` response ends as `frame-src 'self' <proxy origin(s)>;
  frame-ancestors 'self'` plus `X-Frame-Options: SAMEORIGIN`; the shell and
  plain responses keep `frame-ancestors 'none'` and `DENY`.
- **Rail-mounted features relocate** (coherence review 2026-11-13,
  autonomous): rail UI that lands before this ticket (the worktree-delete
  row action and modal, the rail-usage gutter footer) moves into the shell
  document with the rail; the shell is same-origin, so their route gates are
  unaffected.
- Wide-view layout rule (owner, 2026-11-13, from the dash-nav note): in wide
  view all panels are left-aligned and the pinned panel always fills the
  remaining width to the right. This landed first as a separate quick change:
  branch `impl/develop/wide-left-align` is merged into `develop` (merge
  2306fb085, CSS-only in `dashboard.css` and `tracker.css`); the shell must
  preserve that layout.
- Implementer is a Claude Opus worker: the owner directs frontend adjustments
  to Opus agents and the change needs LOOK visual judgment. Rejected: Pi worker.

## Constraints

- T0 only (additive routes/params). See AGENTS.md `### Compatibility Tiers`.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing, per AGENTS.md
  `### Implementation Conventions`.
- Builds and tests run on the sys_lock remote w5090.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 939a2c59 (2026-11-12, commit): "One flat JSON array per kind (ws.dashboard.railorder by reply-id, ws.dashboard.wsorder by path) instead of a per-group map" - rail order, stars, and collapse state are browser-storage only (`assets/rail.js#L1-L15`, "nothing reaches the server") - bearing: constrains
- 2306fb08 (2026-11-13, commit): "Owner request 2026-11-13 (board dash-nav): wide view panels left-aligned, pinned panel always fills remaining width; asked to land quickly ahead of the iframe shell." - bearing: supports
- 2306fb08 (2026-11-13, commit): "Known follow-up: pin width drag handles mostly inert now that --pinw is a minimum; owner decision (429) pending." - bearing: constrains
- a8747f16 (2026-11-13, commit): "Session-switch cost is structural (full document reload) and belongs to the iframe shell ticket; this ticket only measures (d)." - bearing: supports
- fd4fc2be (2026-11-13, commit): "Preserve rail hotkey handover and use one opening state for overlay rail plus memos or memos beside a permanent rail." - bearing: constrains
- 93ac7559 (2026-11-10, commit): "The rail region is re-rendered on every stream update, so the popovers are created once outside the region, positioned fixed, and re-anchored after each swap" - bearing: constrains
- 05ddba10 (2026-11-13, commit): "Owner 2026-11-13: a stopped (child_exited) session showed live colored text in the sidebar; fix now instead of the weekend, drop the moon icon, dim the harness badge of sleeping rows." - bearing: constrains
- 6e0e6928 (2026-11-12, commit): "Sticky unit: a `.thead` wrapper around bar + recap + check summary sticks at the bar's existing stick line." (superseded by `.tophead` in 261112-feat-ws-whiteboard-topic-tickets-sticky-head Decision 12; `.thead` is also the session header class) - bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | assets/dashboard.tmpl, assets/dashboard.js, assets/dashboard.css, assets/rail.js, pageview.go, pageview_rail.go, pages.go under agents-plugin-tool/internal/wsdashboard |
| scope.surface | cross-module | Go page rendering and headers plus browser assets and a shell-to-frame postMessage contract; new `?frame=1` query param |
| scope.new_public_symbol | yes | `?frame=1` frame mode, the shell page, postMessage control messages |
| scope.new_type_contract | yes | postMessage UI-control message shapes (navigate, title, badges, notifications, visibility) between shell and frames |
| scope.test_surface | existing | pageview_test.go, rail_test.go, pincol_width_test.go, pages_test.go under agents-plugin-tool/internal/wsdashboard; the ticket names no test file |
| complexity.reuse_points | unconfirmed | existing rail region render (pageview_rail.go), WsRail state, and region hash swap in dashboard.js; the ticket does not name the reused components and no hash-fan-out cache is confirmed in the tree |
| complexity.side_effect_risk | high | every dashboard navigation, the per-page WebSocket, rail state, and the session-page script's single-document assumptions change; 13 scripts load per page |
| risk.correctness | high | frame LRU lifecycle, hidden-frame throttling, history.pushState sync, and focus and keyboard routing across documents |
| risk.fit | high | the ticket needs state the server cannot see today (rail collapse/pin/order are localStorage-only) and a framing policy the daemon currently denies |
| risk.test | high | LOOK visual judgment and measured switch time; cross-document behavior is not covered by the node-run script tests |
| risk.security_or_contract | high | every daemon page sends frame-ancestors none and X-Frame-Options DENY (pages.go#L1265-L1291) to stop clickjacked write routes; framing session pages inside the shell relaxes that guard |

## Phases

Preserve the wide-view layout rule from `## Decisions` (left-aligned panels,
pinned panel fills the remaining right width) throughout.

### Phase 1: Shell, sidebar, header, flicker fix, session frames

Shell holding the sidebar and top header (global search, notifications,
session title/badges); flicker fix (collapse/pin/order state applied before
first paint, mechanism per (501)); framing headers per (503); session frames (agent panel stays in the frame) with a 5-frame
LRU and `pushState` URLs (`/s/<id>`, frames load `?frame=1`). Narrow/mobile
keeps the overlay sidebar. Hotkey forwarding and in-frame link interception
per `## Decisions`; non-session links navigate the top level until Phase 2.

Verification: Go tests that `?frame=1` responses carry `frame-ancestors
'self'` / `SAMEORIGIN` and other responses still carry `'none'` / `DENY`; LOOK
captures of the shell wide and narrow, and a reload with a collapsed and
reordered sidebar showing no flicker (state correct on the first captured
frame); a headless check that with focus inside a session frame the rail
hotkey toggles the shell rail and global search takes focus, and that a
click on an in-frame `/s/<id>` link switches frames through the shell (URL
updated by `pushState`, no plain-page load in the frame); browser back and
forward switch frames.

### Phase 2: Remaining pages as frames, throttling, render fan-out

All remaining dashboard pages open as frames inside the shell: every
dashboard route except session pages (already framed in Phase 1); popout
and embed routes that are opened outside the dashboard stay unframed, and
the worker lists any other exception in the Result. Plus hidden-frame
throttling and the render fan-out cache.

Verification: LOOK captures wide and narrow; a test that a hidden frame is
told it is hidden and lowers its update rate, and resumes when shown; a test
that two frames of the same session cause one region render per update
(fan-out by hash cache); measured session-switch time before/after, baseline
today's full reload (owner-observed 500-1000 ms), pass when switching to a
session still in the LRU completes under 100 ms and a cold switch is no
slower than the baseline.
