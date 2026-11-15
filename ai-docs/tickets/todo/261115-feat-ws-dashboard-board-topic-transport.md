---
title: Dashboard board transport renders and ships per topic
---

# Dashboard board transport renders and ships per topic

## Background

Owner direction (2026-11-15, relayed by MAIN): the board transport needs a
fundamental optimization, "even if server-rendered, render per topic and
fetch only changed topics". The trigger was the 2026-11-15 reload/spinner
regression; its client-side causes landed as separate heads (A
`exec5/perf-history-init`, B `exec5/perf-rail-hover`), and the daemon's idle
re-read cost is `261115-refactor-ws-dashboard-stream-whiteboard-reread`
(worker C). This ticket is the structural follow-up after A/B/C land.

### Baseline (live, develop ce22992e6..c51439783, 2026-11-15)

Session `sweatradartakingoverwrite`, ~112 topics, ~4035 entries, read-only
measurements against the shared daemon (raw: `/tmp/exec5/perf/`,
`/tmp/exec5/transport/`, machine-local):

| Measure | Value |
|---|---|
| Frame page `/s/<id>?frame=1` HTML | 9.7MB (board region ~8.1-8.9MB) |
| Frame server TTFB | 0.17-0.21s |
| Shell page `/s/<id>` HTML | 1.47MB, TTFB 0.34-0.47s |
| Frame reload in Chromium (before head A) | load 1.8-7.9s, longest task up to 4.3s |
| Frame reload, offline fixture, after head A | load ~1.76s, longest task ~0.85s, init ~0.2s |
| Fresh frame stream, first 32.5s (`?h=&frame=1&topics=1`) | 34 events, 10.8MB: board 1x8.9MB, tree 13x (1.24MB total), entries 4x (477KB), panels 3x (175KB), convlive 3x (24KB) |
| Shell rail stream, first 32.5s (`/rail/events?h=`) | 27 rail regions, 7.7MB (~287KB each, ~1 per second) |
| Daemon idle CPU | ~70% of one core (worker C's ticket) |

### What already exists

- Region streaming: each page region carries a content hash; the stream
  sends only regions whose hash the page does not hold
  (`agents-plugin-tool/internal/wsdashboard/sse.go:16-39`, `sendRegions`
  at `sse.go:615`).
- Topic deltas for the board region: a session stream opened with
  `topics=1` splits the rendered board into topic sections with per-topic
  hashes and sends only changed topics plus order
  (`board_topics.go` `splitBoardTopics`, `boardTopicsEvent`;
  `sse.go:451`, `sse.go:618-631`; the client asks for it in
  `assets/dashboard-stream.js:182`).
- Old-tab safety: the stream sends the daemon's assets identity before any
  region, and a page whose script is older reloads once
  (`sse.go:468-475`; `assets/dashboard-stream.js:175`, `:249-251`).

### Gaps

1. The server still renders the whole board (all topics) for every board
   change and then splits it; per-topic deltas save wire bytes, not render
   or read cost.
2. The initial frame HTML and the first stream board event ship every topic
   (8-9MB) regardless of what is visible.
3. Client init (whiteboard `stream.js`, `wbBindBoard`) runs over every
   topic at load; head A reduced the cost but not the O(topics) shape.
4. Non-board regions re-ship whole: `tree` (~95KB per push, 13 per 30s),
   `entries` (~119KB), `panels` (~58KB), and the shell `rail` (~287KB per
   push at ~1/s; head B stops the DOM remount, not the bytes).

## Decisions

- (D1) Scope follows the owner direction: per-topic render units with a
  per-topic version/hash; the initial load ships the shell plus the visible
  or first-N topics and lazily fetches the rest; the stream sends only
  changed topic fragments, extending the existing region-hash and
  `topics=1` mechanism down to topic-granular rendering (not only
  topic-granular diffing). Source: owner via MAIN, 2026-11-15.
- (D2) Compatibility, per surface (MAIN 2026-11-15: old open tabs must
  degrade to a reload, not break):
  - Page <-> daemon stream and HTML: T0. Daemon-served; the assets-identity
    event reloads an old page before it sees new markup
    (`sse.go:468-475`, `dashboard-stream.js:249-251`). New events stay
    additive beside `region`, like `boardTopicsEvent`.
  - Whiteboard on-disk store: untouched (no shape change). T0.
  - MCP tools and `/mcp` hello: untouched. T0.
  Any design that needs an on-disk or MCP change re-opens this decision.
- (D3) Ordering: heads A (client init batching) and B (rail hover fast
  path) and ticket `261115-refactor-ws-dashboard-stream-whiteboard-reread`
  (C, server read caching) land before this ticket starts; this ticket
  does not preempt them. A's batched init becomes the per-topic init path;
  C's cached board read is the input the per-topic renderer consumes.

## Open Decision Queue

Last ID: (1034)

(1031) [open] [technical] Per-topic render cache keyed by topic revision (from C's cached board read) over re-rendering the whole board and splitting - only the per-topic render removes the O(topics) server cost that splitting keeps.
(1032) [open] [technical] Lazy topics as server placeholders with key+hash, fetched by a per-topic GET (or a stream request) when near the viewport, over shipping all topics as inert templates - templates still cost the 8-9MB transfer and parse.
(1030) [open] [policy] How many topics the initial load renders eagerly.
- Context: the reader usually looks at the newest topics; deep links and search jumps can target any topic.
- Alternative: a fixed first-N (e.g. 12) plus the deep-link target; or viewport-driven only (measure after first paint).
> Recommend first-N plus any deep-link/search target, N as a code constant tuned by measurement; viewport-only delays first paint of anything below the fold.
(1033) [open] [technical] Extend the same hash-per-unit approach to `tree`, `entries`, `panels` and the shell `rail` (row-level units) in later phases rather than in Phase 1 - the board is ~80% of first-load bytes; one surface at a time keeps each phase verifiable. MAIN note: the rail stream (7.7MB per 32s per open shell) is the largest steady cost, so rail row units are the first follow-up phase candidate.
(1034) [open] [technical] Verification bar: before/after on a ~112-topic fixture for frame HTML bytes, first-stream bytes, per-change bytes, server render time per change, and Chromium reload time and longest task, plus the existing stream/topic tests - these are the baseline rows above.

## Constraints

- Shipped-surface and wsflow-mirroring manuals apply to any dashboard asset
  or template change (`AGENTS.md` Implementation Conventions).
- Search jump, deep links, history "Older" paging, panel disclosure state
  and reader scroll anchoring must keep working with lazily loaded topics.

## Phases

### Phase 1: Per-topic board render and lazy initial topics

Render board topics as independent units cached by topic revision; the
frame HTML and the first `topics=1` stream event carry only the eager
topics plus placeholders; the rest load on demand; board changes render and
ship only the changed topics. Verify against the baseline table above with
the (1034) bar.
