---
title: "Dashboard conversation: incremental updates and on-demand older history"
parent: 261107-epic-ws-dashboard-messenger-round2
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8a7315100dbe69a5
sage-review-completeness-reviewed: 8a7315100dbe69a5
completed: 2026-11-07
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
  streaming reply nudges the daemon about ten times a second and each stream
  renders at most once per 250 ms, so the region changes up to four times a
  second (`agents-plugin-tool/internal/wsdashboard/sse.go#L27-L30`,
  `sseMinInterval = 250ms`), and lead replies now render as markdown (merge
  ddb721de1), so each tick re-runs markdown for every entry.

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
  name is a stable paging cursor. This supersedes the "last N, about 50"
  retention of 261106-feat-ws-dashboard-live-snapshot-pi and the round-1
  epic 261106-epic-ws-dashboard-serve-daemon; the history stays bounded, so
  their rejection of an unbounded log still holds.
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
- **Cache key covers every render input** (lead, 2026-11-07, auto-confirmed
  from design review): a ring entry file never changes, but a mail line's
  rendered HTML also depends on whether its body is still in the mailbox
  history ring and whether this daemon serves the linked peer, and a user
  line's thumbnails depend on whether its attachment files still exist. The
  cache key is the entry name plus those inputs. The page still never
  re-renders an entry it already shows (Two regions), so a changed link or
  a removed attachment appears after a reload. A missing attachment renders
  as path text, not a broken image. Rejected: keying on the entry name alone
  - it would freeze a dead peer link or a broken image into every later
  render.
- **Cache bound** (lead, 2026-11-07, auto-confirmed from design review): the
  daemon drops a session's cached entries when the session leaves the index,
  and bounds each session's cache (the live 50 plus recently fetched history
  pages, least recently used evicted first). Rejected: an unbounded cache - a
  long-lived daemon serving many 2000-entry sessions would grow without
  limit.
- **History route shape** (lead, 2026-11-07, auto-confirmed from review):
  `before` is exclusive; the response is JSON with the rendered entries' HTML
  fragment, the next `before` cursor, and an end flag that is true when no
  older entry remains, so the page stops asking. The route reads the ring
  whenever the session page itself is served, adapter present or not; an
  omitted `before` returns the newest page, so a page that drew no entries
  can still start loading.
  Rejected: an HTML-only response - the page would have to parse the cursor
  and the end state out of markup.
- **Scroll owner and merge scope** (lead, 2026-11-07, auto-confirmed from
  design review): a wrapper outside both regions owns scrolling and the
  scroll pin, since `#r-conversation` is the scroll element today and the
  split makes it two regions. The keyed merge looks for `data-entry` only
  inside the entries container: held bubbles move to the live region and
  already carry `data-entry="<inbox id>"`, a nearly identical name grammar,
  so they must never be read as ring entries. The steer-promotion sibling
  attaches its button to held bubbles; whichever ticket lands second targets
  the live region's selectors.

## Constraints

- The conversation keeps showing a waiting approval after the split (epic
  cross-child decision).
- Page view state (composer text, scroll pin, the agents/conversation split)
  survives updates, as it does today.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Prior Decisions

- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, Decisions): "Conversation history is a per-session ring. ws-mcp writes each finished conversation message ... as one file in a per-session ring that keeps the last N entries (N a child constant, about 50)" — bearing: contradiction-candidate
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "adds a per-session conversation history ring (last N, about 50) beside the bounded snapshot, so reload and daemon replacement keep recent conversation; rejected no-history and an unbounded log" — bearing: contradiction-candidate
- 261106-feat-mailbox-history-ring (2026-11-06, Decisions): "Rejected: appending to one file, where concurrent appends interleave or overwrite depending on the OS." — bearing: supports
- 261107-epic-ws-dashboard-messenger-round2 (2026-11-07, Cross-Child Decisions): the page keeps showing a waiting approval (conversation and agents tree) — bearing: constrains
- 026a2d62 (2026-11-07, commit): "Branch already merged develop (5cf17805b) to resolve the one-line r-conversation conflict with the markdown merge, keeping both sides" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wslive/ring.go, agents-plugin-tool/internal/wslive/snapshot.go, agents-plugin-tool/internal/wsdashboard/sse.go, pages.go, assets/dashboard.js, assets/dashboard.tmpl |
| scope.surface | cross-module | wslive retention and cursor read consumed by wsdashboard region render, a new HTTP route, and page JS |
| scope.new_public_symbol | yes | GET /s/<rid>/history route and a history-cap constant beside wslive.RingCap |
| scope.new_type_contract | yes | history route query contract (before, n) and rendered-entries response; entries/live region split |
| scope.test_surface | existing | agents-plugin-tool/internal/wslive/ring_test.go, agents-plugin-tool/internal/wsdashboard/pages_test.go, daemon_test.go; history route and cache tests may add files |
| complexity.reuse_points | confirmed | region hash and SSE mechanism (sse.go regionEvent), ring entry files and ringEntryNames in ring.go, swap in dashboard.js |
| complexity.side_effect_risk | high | the conversation region markup, swap logic, scroll pin, composer, applySent and held bubbles all depend on r-conversation being replaced whole |
| risk.correctness | high | keyed merge must keep ordering, held/sent state, scroll position and approval display across regions; trim race across processes |
| risk.fit | moderate | follows the existing region hash and ring directory patterns but splits one region into two |
| risk.test | high | page-side merge and scroll-up loading have no automated check; only a live check covers them |
| risk.security_or_contract | moderate | new route must apply the session page access rules and treat the before cursor as a file name without path traversal |

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

### Result (b150fe83a) - 2026-11-07

Landed as specified: the ring keeps `wslive.RingHistoryCap` (2000) entries
on disk, `ReadRing` is the newest `RingCap` (50) view, and `ReadRingPage`
pages older entries by an exclusive entry-name cursor (`ValidRingName`
guards it). The conversation is two regions, `entries` (merged by
`data-entry` inside `.centries`, never re-rendered) and `convlive`
(streaming reply, approval/activity line, held messages, sent marks), in a
`#convo` wrapper that owns scrolling and the pin. A per-session LRU entry
cache (250 entries, dropped when the session leaves the index) renders each
entry once. `GET /s/<rid>/history?before=&n=` answers
`{html, before, end}`, and the page prepends older pages near the top with
the scroll position kept.

Verification: `go test ./internal/wslive/ ./internal/wsdashboard/
./internal/wsinbox/ ./internal/mcp/` pass, and `-race` passes for wslive
and wsdashboard. One test, `TestMessageRouteRefusesBadImages`, times out
under `-race` on pristine develop too. Live check, headless Chromium
against a 310-entry fixture with 300 ms streaming ticks:
- 50 entries drawn;
- entry nodes survive the streaming ticks;
- scrolling up loaded all 310 entries in 6 requests, in order, with the
  reading position kept;
- a new message appended while scrolled up left `scrollTop` unchanged;
- no console errors.

Decisions:
- The entries region keeps its title bar (the counts) outside the keyed
  container. The page replaces the bar and merges the entries.
- Both regions are `display: contents`, so the sticky bar spans `#convo`.
- Develop was merged forward (29c81ad6e) to pick up composer image
  attachments.
  - Thumbnails now require the attachment file to exist; otherwise the path
    shows as text, per the cache-key decision.
  - The cache key includes each image's src.
  - The history route extracts user-line images the same way the live view
    does.
