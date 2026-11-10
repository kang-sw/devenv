---
title: "Dashboard agent host: a session rail card on the session page, always shown when wide and sliding out from the left otherwise, with per-session draft restore"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-workspaces: "later regroups the rail from project to workspace"
  261110-feat-ws-dashboard-agent-panel-telemetry: "sibling multi-session UX child; independent"
---

# Dashboard session rail

## Background

Track A of the epic, for the sessions that exist today: the user runs many
sessions at once and wants switching between them to be smooth, independent
of the terminal and the session holder (2026-11-10 whiteboard, topic
"멀티 세션 관리 UX", (28) b assumed from the user's request).

Today the arrow at the session page's top left (`assets/dashboard.tmpl:9`,
`<a class="back" href="/">`) is a plain link to the sessions index page,
not a dropdown or a drawer. The index (`handleIndex`, `pages.go:804`;
`buildIndexView`, `pageview.go:766`; templates `index` and `r-sessions`,
`assets/dashboard.tmpl:175`, `:194`) already groups sessions by registration
project path (`ProjectGroup`, `pageview.go:732`) with live sessions first,
an ended subsection, and rows (`IndexRow`, `pageview.go:742`) showing the
reply ID, harness, phase pill, current tool, running-agent count, policy
item count, and last seen. It is server-rendered and refreshed over the
`/events` WebSocket.

The session grid is one column with tabs below 760px and two columns
(`5fr | 7fr`, conversation and board) from 760px (`assets/dashboard.css`
around lines 468-496); the layout menu (auto, split, tabs) is saved in
localStorage `ws.dashboard.layout` (`assets/dashboard.js:152`). A
three-column mode exists only in the tracker drawer, from 1600px
(`layoutOf`, `assets/tracker.js:2037`).

Mockup the user reviewed: https://claude.ai/artifact/TboGt3LPM37sSVSknTMZbE
(three widths, the rail sliding out, sample data).

## Decisions

1. **The session list becomes a card, the session rail, on the session
   page (user's request; technical, (29) ok).** It carries the index's
   project groups and row content into a card at the page's left. From a
   1600px viewport the page has three columns, rail | conversation |
   board, and the rail is always shown. Below 1600px the top-left arrow
   toggles the rail as an overlay that slides in from the left over the
   page, with a scrim; Esc, a scrim click, or picking a session closes it.
   The index page `/` stays, linked from the rail's foot (it also lists
   the ended sessions in full). 1600px is the tracker drawer's
   three-column threshold, so the page has one notion of "wide".
   Rejected: inserting the rail as a grid column below 1600px, which
   narrows the conversation column.
2. **Picking a session is a full page navigation to `/s/<id>` (technical,
   (30) ok).** Rejected: an in-page (SPA) swap, which would have to swap the
   region stream, the composer draft, scroll, and board state inside one
   page. Every page renders the rail as the same region, so a navigation
   costs only a short repaint.
3. **Per-session draft restore in localStorage (user, (30) note; technical,
   (39) ok).** Keyed by session ID: the composer text with its mentions,
   whiteboard answer and note inputs in progress, and open, collapsed, tab,
   and selection states. Sending a message clears that session's draft;
   drafts beyond a fixed cap are dropped oldest first. Pasted images are not
   stored; a restored draft that had images says that the images were not
   restored. This reuses the page's existing `save`/`load` localStorage
   helpers (`assets/dashboard.js:17-20`); the composer draft is held in
   memory only today. Rejected: storing images in IndexedDB (a new storage
   layer for one case); relying on localStorage for images (its quota,
   around 5MB, fills quickly).
4. **Leaving with unrestorable input asks first (user, (39) note).** While
   the current page holds input that the draft store cannot restore (pasted
   images), navigating away, including picking another session in the rail,
   raises the browser's leave-page confirmation (`beforeunload`), the way an
   editor guards unsaved edits.
5. **The rail's visibility depends only on the viewport width (technical,
   (40) ok).** From 1600px it is always shown whatever the layout menu
   (auto, split, tabs) says; the menu keeps governing the conversation and
   board columns only. Rejected: hiding the rail under the "tabs" layout,
   which would make it vanish when the user only wanted tabs for
   conversation and board.

## Constraints

- Independent of the session holder, the hosted-session flag, daemon lead
  sessions, and the terminal ((36) ok); it can land now.
- `261110-feat-ws-dashboard-workspaces` later changes the rail's grouping
  from project path to workspace ((36) ok).
- The rail shows the index's content to the viewers the index already
  serves; it exposes nothing the index withholds.
- Browser storage can be absent or throw (private windows, blocked site
  data); the page must work without it, as the existing helpers already
  guard.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`. Heavy Go builds
  and tests follow the machine's remote-build note when one applies.

## Phases

### Phase 1: rail, responsive overlay, draft restore

Implement Decisions 1-5.

Verification:

- Go tests: the session page renders the rail region with the index's
  project groups and the current session marked; the region refreshes over
  the stream like other regions.
- Node-run JS cases in the existing style (`runNode`, `toast_test.go`):
  draft save, restore, clear on send, cap eviction, the images-not-restored
  notice, and the leave guard armed only while unrestorable input is held.
- A browser check at three widths (at least 1600px, between 760 and 1600px,
  below 760px): the rail always shown, then sliding in and closing on Esc,
  scrim, and selection; a typed draft survives switching sessions and back.
