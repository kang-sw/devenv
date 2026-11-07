---
title: "Dashboard conversation: incremental updates and on-demand older history"
parent: 261107-epic-ws-dashboard-messenger-round2
---

# Dashboard conversation: incremental updates and on-demand older history

## Background

The user runs very long lead sessions and wants the live session page to
replace the terminal (2026-11-07: "제가 초장기 세션을 많이 쓰는 편이라 아마
성능 이슈가 있을 거에요. 온디멘드 로드라던지 이 채팅창 성능 개선도 꼭
스코프로 넣어주고요"). Two facts shape the work:

- The conversation keeps only the last 50 finished messages: the ring
  (`wslive.RingCap = 50`, `agents-plugin-tool/internal/wslive/ring.go`)
  deletes older entry files, so a long session loses its history on disk.
- Every change to the `r-conversation` region re-renders all of its entries
  on the daemon and replaces the region's `innerHTML` on the page
  (`wsdashboard/sse.go` region events, `assets/dashboard.js` `swap`). The
  streaming reply changes the region up to ten times a second, and lead
  replies now render as markdown (merge ddb721de1), so each tick re-runs
  markdown for every entry.

## Decisions

- **Incremental conversation updates** (user, 2026-11-07, "(27) ok"): a new
  finished message is added to what the page shows and the streaming reply
  updates only its own bubble; the page no longer replaces the whole
  conversation on every change. Rejected: keeping the whole-region swap.
- **History depth** (user, 2026-11-07, option (a) of (28)): the page draws the
  recent 50 messages; a longer history stays on disk, and scrolling to the
  top loads older messages a page at a time. Rejected: keeping only the
  recent 50 with incremental updates alone (option b) - a long session could
  not look back.
- **History lives in the existing ring dir** (user, 2026-11-07, "(35) ok"):
  the live view reads the newest 50 entries and the trim prunes beyond a
  history cap of 2000 entries. Rejected: a separate append-only log file -
  entries are already immutable, stamp-named files written lock-free by
  several processes, so one store needs no new writer protocol, and an entry
  name is a stable paging cursor.
- **Two regions** (user, 2026-11-07, "(36) ok"): an entries region the page
  merges by key (it inserts entries whose `data-entry` name it lacks and never
  re-renders an existing one) and a small live region (streaming reply,
  approval/activity line, held messages). Rejected: a new append-only event
  protocol - the split reuses the region hash and SSE mechanism, and the
  10-per-second streaming ticks resend only the small live region.
- **Rendered-entry cache** (user, 2026-11-07, "(37) ok"): the daemon caches
  each entry's rendered HTML by entry name. Rejected: rendering every entry
  on every render - entries never change once written, so markdown runs once
  per entry instead of once per tick.
- **History route** (user, 2026-11-07, "(38) ok"):
  `GET /s/<rid>/history?before=<entry name>&n=50` returns rendered entries
  under the session page's access rules; the page fetches it when scrolled to
  the top and prepends, keeping the scroll position, and evicts nothing.
  Rejected: embedding the full history in the region - the region stays
  bounded at 50, and older pages cost nothing until asked for.

## Constraints

- The conversation keeps showing a waiting approval after the split (epic
  cross-child decision).
- Page view state (composer text, scroll pin, the agents/conversation split)
  survives updates, as it does today.

## Phases

### Phase 1: Incremental conversation and paged history

One behavior slice: the ring retention change, the region split and keyed
merge, the rendered-entry cache, the history route, and the page's
scroll-up loading land together.

Verification: Go tests for the ring (newest-50 view, trim at the history
cap, paging by cursor), the region split (a streaming tick changes only the
live region's hash; a new entry changes the entries region), the cache (an
entry renders once), and the history route (cursor paging, end of history,
access rules); a live check on a long session that scrolling up loads older
messages and that streaming no longer re-renders the entries.
