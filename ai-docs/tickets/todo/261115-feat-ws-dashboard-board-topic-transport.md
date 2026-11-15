---
title: Dashboard stream renders only what changed, per topic and per row
---

# Dashboard stream renders only what changed, per topic and per row

## Background

Owner direction (2026-11-15, relayed by MAIN): the dashboard must stop
hammering the daemon CPU on every small change. Server and client stay in
sync for the at-a-glance view; the initial reload payload is not the
problem. The trigger was the 2026-11-15 reload/spinner regression, whose
client-side causes landed separately (head A `69c7e50fa`, merged
`7622414ea`; head B `fa65f7c53`, merged `219aa6032`). The daemon's per-read
whiteboard folder reload is ticket
`261115-refactor-ws-dashboard-stream-whiteboard-reread` (worker C), which
lands before this one.

### Baseline (live, read-only, 2026-11-15)

Session `sweatradartakingoverwrite`: ~112 topics, ~4035 entries. Raw files
under `/tmp/exec5/perf/` and `/tmp/exec5/transport/` (machine-local).

| Measure | Value |
|---|---|
| Frame page `/s/<id>?frame=1` HTML | 9.7MB (board region ~8.9MB), TTFB 0.17-0.21s |
| Shell page `/s/<id>` HTML | 1.47MB, TTFB 0.34-0.47s |
| Daemon CPU, idle (no request of ours), develop 63e89802b | ~70% of one core (pprof: 4.22s of samples in 6.07s) |
| Daemon CPU, steady, develop 7622414ea (pid 91340, ~3 min after swap, one extra frame stream of ours) | ~106% of one core (`ps` cputime 31.8s over 30s) |
| Frame stream deltas after the first board, 34s | 45 region events, 3.4MB: tree 32x (~97KB each), entries 1x (149KB), panels 2x (117KB), convlive 4x (50KB); no board-topic deltas |
| Frame stream deltas, an earlier 45s window | tree 15x (1.49MB), entries 5x (621KB), panels 5x (291KB), convlive 4x |
| Shell rail stream, 32s | 27 rail regions, 7.7MB (~287KB each, ~1 per second) |

A single small board change was not measured live: that needs a write to a
shared board, which this read-only diagnosis avoids. Phase 1 records it on
a fixture before changing code (see verification).

### Where the per-change cost goes

- Every page render runs every region template: `renderSessionRegions`
  (`agents-plugin-tool/internal/wsdashboard/pages.go:698-717`) executes all
  `r-<name>` templates for the session, the ~8.9MB board included, and the
  session page source then tokenizes the whole board HTML again to split
  topics (`sse.go:328-334`, `board_topics.go` `splitBoardTopics`). A
  `tree` change (~1 per second here) therefore pays for the whole board.
  INFERRED from the code path; Phase 1 confirms it with a profile.
- The page cache can skip unchanged renders (`stream_cache.go:176`, wake
  deps plus refresh stamp) and reuse reads for live-only wakes
  (`renderLive`, `stream_cache.go:223`), but a real render still renders
  every region.
- The stream already sends only changed regions by hash, and only changed
  board topics when opened with `topics=1` (`sse.go:451`, `sse.go:615-631`,
  `boardTopicsEvent`). These save bytes on the wire, not render or read
  cost. Non-board regions (`rail`, `tree`, `entries`, `panels`) re-ship
  whole on any change in them.

## Decisions

- (D1) Target is per-change server cost, not initial payload. The initial
  frame HTML and first stream event keep carrying every topic. Source:
  owner, board (1030)=c, 2026-11-15. Rejected: first-N or viewport-driven
  initial topics (owner: the at-a-glance view must stay in sync and the
  initial load is not the problem).
- (D2) (1031) Board topics render as independent units cached by topic
  revision; a change re-renders only the changed topics and reassembles the
  board region from cached topic HTML, and the stream's topic diff consumes
  those units directly instead of re-tokenizing the whole board. Rejected:
  render the whole board, then split and hash (today's path) - it keeps the
  O(topics) render and tokenize cost on every change. Owner: ok.
- (D3) (1032) Placeholders with per-topic fetch are dropped, superseded by
  (1030)=c.
- (D4) (1033) Row-unit deltas for the other heavy regions follow the board,
  rail first (the largest steady cost: ~287KB per push per open shell at
  ~1/s), then `tree`, `entries`, `panels`. Each re-renders only changed rows
  and ships changed rows plus order, extending the existing region-hash and
  `topics=1` pattern. One surface per phase. Owner: ok.
- (D5) (1034) Verification is per-change server CPU and bytes against the
  baseline above: daemon CPU idle and under one small change, render time
  per change, bytes per change, on a ~112-topic fixture before and after,
  plus the existing stream/topic/region tests. Owner: ok.
- (D6) Compatibility per surface (MAIN, 2026-11-15: old open tabs must
  degrade to a reload, not break):
  - Page <-> daemon stream and HTML: T0. Daemon-served; the stream sends
    the assets identity before any region and an older page reloads once
    (`sse.go:466-475`; `assets/dashboard-stream.js:175`, `:249-251`). New
    row/topic events stay additive beside `region`, as `boardTopicsEvent`
    did.
  - Whiteboard on-disk store, MCP tools, `/mcp` hello: untouched, T0.
  Any phase that needs an on-disk or MCP change re-opens this decision.
- (D7) Order: heads A and B have landed; ticket
  `261115-refactor-ws-dashboard-stream-whiteboard-reread` (C) lands first
  and its cached board read is the input of the topic render cache.

## Constraints

- `AGENTS.md` Implementation Conventions: `shipped-surface-boundary.md` and
  `release-compatibility.md` apply to `agents-plugin-tool/internal/`;
  `wsflow-mirroring.md` applies to `agents-plugin/` assets if any are
  touched.
- Visible output must stay byte-identical to today's full render for the
  same state (topic HTML, region HTML, hashes may change form but the page
  shows the same thing), so search jump, deep links, history Older paging,
  panel disclosure state and reader anchoring keep working.
- Do not run the full `./internal/wsdashboard` test package; use `-run`.
  Tests run with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

## Phases

### Phase 1: Per-topic board render cache

Prerequisite: C landed. First, on a ~112-topic fixture, profile one render
triggered by a non-board change and one triggered by a single small topic
edit, and record CPU time, render time and bytes (baseline rows for D5).
Then render board topics as units cached by topic revision, reassemble the
board region from them, and feed the `topics=1` diff from the units. A
non-board change must not re-render or re-tokenize board topics; a single
topic edit re-renders that topic only. Verify: the fixture numbers before
and after, a regression test that a non-board change does not execute topic
rendering and a one-topic edit renders one topic, the existing
board-topic/stream/region tests, and a Chromium check that the page shows
the same board after a topic edit.

### Phase 2: Rail row deltas

Render rail rows as units keyed by session revision; ship changed rows plus
order instead of the whole rail region; keep head B's hover fast path
semantics. Verify bytes per rail change and daemon CPU against the 7.7MB
per 32s baseline, plus the existing rail tests and the
`TestRailHoverUpdatesBrowser` regression.

### Phase 3: Tree, entries and panels row deltas

Apply the same unit-and-delta pattern to `tree`, `entries` and `panels`,
one region at a time within the phase. Verify bytes per change against the
baseline rows and the existing region tests.
