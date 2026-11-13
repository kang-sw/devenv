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
- Implementer is a Claude Opus worker: the owner directs frontend adjustments
  to Opus agents and the change needs LOOK visual judgment. Rejected: Pi worker.

## Constraints

- T0 only (additive routes/params). See AGENTS.md `### Compatibility Tiers`.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing, per AGENTS.md
  `### Implementation Conventions`.
- Builds and tests run on the sys_lock remote w5090.

## Phases

Draft; may change after the queue settles.

### Phase 1: Shell, sidebar, flicker fix, session frames

Shell plus sidebar in the shell; flicker fix (collapse/pin/order state
reflected in the server render); session frames with LRU and `pushState` URLs.

### Phase 2: Remaining pages, throttling, render fan-out

Remaining pages as frames, hidden-frame throttling, and the render fan-out
cache.

Verification: LOOK captures wide and narrow; measured session-switch time
before/after.

## Open Decision Queue

Last ID: (423)

(418) [open] [policy] What the persistent shell holds: a sidebar only; b sidebar + top header (global search, notifications, session title/badges); c b + agent panel. Recommendation: b - global search and notifications are session-independent; the agent panel is per-session content.

(419) [open] [policy] Non-session pages (tracker, wiki, etc.) also open as frames inside the shell: a session pages only; b all dashboard pages. Recommendation: b - the sidebar then never re-renders on any navigation; tracker.js is ~6.4k lines; framing is page-agnostic.

(420) [open] [policy] Narrow/mobile sidebar: a overlay as today; b replace with a top dropdown session picker. Recommendation: a - no change, keeps four-state and tri-state radio sidebar features.

(421) [open] [technical] Keep the 5 most recent frames (LRU, no config key) over unlimited - each frame holds its own JS heap; 5 covers normal back-and-forth; tune after measurement.

(422) [open] [technical] Address bar keeps /s/<id>; frames load the same path with ?frame=1; the shell updates the URL with history.pushState - over a new URL scheme, so existing links, bookmarks and embeds keep working.

(423) [open] [announced] Sidebar flicker fix (collapse/pin/order state in server render) ships in shell Phase 1 - citation: owner (42) answer, flicker fix goes together with the new structure after the design discussion.
