---
title: ws dashboard iframe shell
related:
  261113-feat-ws-dashboard-session-four-state-lifecycle: sidebar contents
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: perf ticket; session-switch baseline
---

# ws dashboard iframe shell

## Background

Current behavior (from the lead's earlier investigation):

- Every dashboard URL change reloads the whole document. Sidebar links are
  plain links; 8 scripts (~16k lines) are re-executed and the WebSocket
  reconnects. The owner observes 500-1000 ms per session switch.
- Sidebar collapse state lives only in browser storage and the server always
  renders the sidebar expanded, so the last script collapses it after paint,
  producing a visible flicker. Pin and order apply at the same late point. The
  "ended" collapse is not persisted at all.

Options considered earlier (board dash-nav):

- (a) Flicker fix only: 0.5-1 day.
- (b) View Transitions: 1 h; smooth white flash, reload cost unchanged.
- Iframe shell: 3-5 days; the shell persists and recent sessions switch
  instantly via LRU.
- (c) Swap body only: 1-2 weeks; needs splitting the ~6.9k-line `dashboard.js`.
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
- (423) Sidebar flicker fix (collapse/pin/order state in the server render)
  ships in shell Phase 1. Owner ok 2026-11-13.
- Wide-view layout rule (owner, 2026-11-13, from the dash-nav note): in wide
  view all panels are left-aligned and the pinned panel always fills the
  remaining width to the right. This lands first as a separate quick change on
  branch `impl/develop/wide-left-align` (in progress); the shell must preserve
  that layout.
- Implementer is a Claude Opus worker: the owner directs frontend adjustments
  to Opus agents and the change needs LOOK visual judgment. Rejected: Pi worker.

## Constraints

- T0 only (additive routes/params). See AGENTS.md `### Compatibility Tiers`.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing, per AGENTS.md
  `### Implementation Conventions`.
- Builds and tests run on the sys_lock remote w5090.

## Phases

Preserve the wide-view layout rule from `## Decisions` (left-aligned panels,
pinned panel fills the remaining right width) throughout.

### Phase 1: Shell, sidebar, header, flicker fix, session frames

Shell holding the sidebar and top header (global search, notifications,
session title/badges); flicker fix (collapse/pin/order state reflected in the
server render); session frames (agent panel stays in the frame) with a 5-frame
LRU and `pushState` URLs (`/s/<id>`, frames load `?frame=1`). Narrow/mobile
keeps the overlay sidebar.

### Phase 2: Remaining pages as frames, throttling, render fan-out

All remaining dashboard pages (tracker, wiki, ...) open as frames inside the
shell, plus hidden-frame throttling and the render fan-out cache.

Verification: LOOK captures wide and narrow; measured session-switch time
before/after.
