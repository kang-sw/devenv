---
title: Dashboard stream renders only what changed, per topic and per row
related:
  261115-refactor-ws-dashboard-stream-whiteboard-reread: prerequisite
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 57b4cb2aadc86f84
sage-review-completeness-reviewed: 57b4cb2aadc86f84
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
`261115-refactor-ws-dashboard-stream-whiteboard-reread` (worker C), whose
Phase 1 read cache landed via `aa07206c0` (including `8145ec935`). C has
since completed its metadata-write elision phase too and is in `.done/`
(`ai-docs/tickets/.done/261115-refactor-ws-dashboard-stream-whiteboard-reread.md`).

### Baseline (live, read-only, 2026-11-15)

Session `sweatradartakingoverwrite`: ~112 topics, ~4035 entries. Raw files
under `/tmp/exec5/perf/` and `/tmp/exec5/transport/` (machine-local).

| Measure | Value |
|---|---|
| Frame page `/s/<id>?frame=1` HTML | 9.7MB (board region ~8.9MB), TTFB 0.17-0.21s |
| Shell page `/s/<id>` HTML | 1.47MB, TTFB 0.34-0.47s |
| Daemon CPU, idle (no request of ours), develop 63e89802b | ~70% of one core (pprof: 4.22s of samples in 6.07s) |
| Daemon CPU, steady, develop 7622414ea (pid 91340, ~3 min after swap, one extra frame stream of ours) | ~106% of one core (`ps` cputime 31.8s over 30s) |
| Frame stream deltas after the first board, 34s | 44 region events, 3.4MB (3,416,177 bytes), plus one tracker event: tree 32x (~97KB each), entries 1x (149KB), panels 2x (117KB), convlive 4x (50KB); no board-topic deltas (`/tmp/exec5/transport/delta2.out`) |
| Frame stream deltas, an earlier 45s window | tree 15x (1.49MB), entries 5x (621KB), panels 5x (291KB), convlive 4x |
| Shell rail stream, 32s | 27 rail regions, 7.7MB (~287KB each, ~1 per second) |

A single small board change was not measured live: that needs a write to a
shared board, which this read-only diagnosis avoids. Phase 1 records it on
a fixture before changing code (see verification).

### Where the per-change cost goes

- Each streamed session render executes every session-region template:
  `renderSessionRegions` (`agents-plugin-tool/internal/wsdashboard/pages.go#L706-L729`)
  inserts already-rendered board HTML through `SessionView.ThemedBoard`
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L166-L167`). The
  MCP source already memoizes the board's topic rendering, so a non-board
  change need not execute topic templates
  (`agents-plugin-tool/internal/mcp/dashboard_page.go#L133-L172`), but the
  session render still processes the whole board HTML and then tokenizes
  it again with `splitBoardTopics`
  (`agents-plugin-tool/internal/wsdashboard/sse.go#L328-L334`,
  `agents-plugin-tool/internal/wsdashboard/board_topics.go#L34-L92`).
  The CPU attribution is INFERRED from this code path; Phase 1 confirms
  it with a profile.
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
  - Topic revision: a topic unit's cache key is a hash of that topic's
    complete render input - the per-topic view data handed to the topic
    template, including fields derived from other topics such as reverse
    waiters (`agents-plugin-tool/internal/wswhiteboard/view.go#L767-L791`),
    not only its own decoded document and entries - plus the shared render inputs the existing board memo tracks
    (language, theme, watched ticket/files state, mailbox and process
    inputs; `agents-plugin-tool/internal/mcp/dashboard_page.go#L83-L205`).
    Folder-file identity may serve only as a hint that a topic might have
    changed, never as the key: the folder writer rewrites every topic's
    `meta.json` on each board commit (`agents-plugin-tool/internal/wswhiteboard/folders.go`
    `plan()`), so identity alone would invalidate every topic. A
    shared-input change invalidates every topic unit. Rejected:
    `TopicDoc.Changed` as the revision - ticket changes do not move it
    (`agents-plugin-tool/internal/wswhiteboard/store.go#L234-L246`).
  - Unit API: the page source hands wsdashboard an ordered list of topic
    units `{id, hash, html}` beside the assembled board region; the
    worker names the symbols. The memo stays in mcp, per `b0e680e2`.
- (D3) (1032) Placeholders with per-topic fetch are dropped, superseded by
  (1030)=c.
- (D4) (1033) Row-unit deltas for the other heavy regions follow the board,
  rail first (the largest steady cost: ~287KB per push per open shell at
  ~1/s), then `tree`, `entries`, `panels`. Each re-renders only changed rows
  and ships changed rows plus order, extending the existing region-hash and
  `topics=1` pattern. One surface per phase. Owner: ok.
  - Rail rows: a row's identity is its `data-rid`; a row ships when the
    hash of its rendered HTML changes. Rows still render on each rail
    wake, because liveness is clock-dependent and layout/worktree inputs
    feed the markup; the saving is the diff, the bytes and the client
    work. Rejected: keying rows by session revision - it misses the
    clock, layout and worktree inputs and would ship stale rows.
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
  - Row and topic delta events are opt-in per stream connection by a
    query flag, as `topics=1` is; a page that does not send the flag gets
    full regions exactly as today. The first event on a connection, and
    any event whose row baseline the server cannot vouch for, is the full
    region, matching the existing first-full/non-topic fallback
    (`agents-plugin-tool/internal/wsdashboard/board_topics.go#L102-L119`).
    So an old tab that banners instead of reloading
    (`assets/dashboard-stream.js:248-252`, drafts or a repeated identity)
    keeps working on full regions.
  - Whiteboard on-disk store, MCP tools, `/mcp` hello: untouched, T0.
  Any phase that needs an on-disk or MCP change re-opens this decision.
- (D7) Order: heads A and B have landed; ticket
  `261115-refactor-ws-dashboard-stream-whiteboard-reread` (C) lands first
  and its cached board read is the input of the topic render cache.

## Constraints

- `AGENTS.md` Implementation Conventions: `shipped-surface-boundary.md` and
  `release-compatibility.md` apply to `agents-plugin-tool/internal/`.
  `wsflow-mirroring.md` does not apply: every phase stays under
  `agents-plugin-tool/`; a phase that touches `agents-plugin/` or
  `agents-plugin-wsflow/` reads it first.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Visible output must stay byte-identical to today's full render for the
  same state (topic HTML, region HTML, hashes may change form but the page
  shows the same thing), so search jump, deep links, history Older paging,
  panel disclosure state and reader anchoring keep working.
- Do not run the full `./internal/wsdashboard` test package; use `-run`.
  Tests run with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

## Prior Decisions

- 261115-refactor-ws-dashboard-stream-whiteboard-reread (2026-11-15, Result): "one 112-topic stream and 40 non-board snapshot wakes at 4 Hz, sample **1.24 -> 0.37 CPU seconds**" — bearing: supports
- 1f4b3ff4 (2026-11-15, commit): "Keep last-good shared render state only for open subscribers, never after confirmed absence/mismatch; no wire event, persisted field or lock boundary changes (T0)." — bearing: constrains
- 3ec591d8 (2026-11-14, commit): "This is the ticket's T0 additive topics=1 opt-in; old streams retain full region events and the first opted-in board send is full even if h matches." — bearing: constrains
- edb6f8a2 (2026-11-11, commit): "The live-only wake uses distinct broker tokens (live:<id>, ring:<id>) so the stream cache can tell a snapshot hand-off from a store change without a new wake channel" — bearing: constrains
- b0e680e2 (2026-11-10, commit): "The memo lives in mcp, where the render inputs are known, rather than in wsdashboard's page cache, which cannot tell board inputs from live ones." — bearing: constrains
- dc8d5566 (2026-11-09, commit): "Theme went into the cache key rather than a theme-neutral board with per-stream ThemeFrames: simpler, and a second theme is rare." — bearing: constrains
- 30485943 (2026-11-07, commit): "One parsed template set per language because a parsed set's FuncMap is fixed; the entry cache key is prefixed with the language so two viewers in different languages never share a cached entry." — bearing: constrains
- b150fe83 (2026-11-07, commit): "The entries region keeps its title bar (user/mail counts) outside the keyed container; the page replaces the bar and merges entries." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Session rendering, shared page cache, topic decomposition and client dispatch span agents-plugin-tool/internal/wsdashboard/pages.go, stream_cache.go, board_topics.go, sse.go and assets/dashboard-stream.js; actual topic rendering is in agents-plugin-tool/internal/mcp/dashboard_page.go and agents-plugin-tool/internal/wswhiteboard/render.go. |
| scope.surface | cross-module | D2 crosses the MCP PageContent source, wswhiteboard rendering and wsdashboard transport; D4 changes the browser stream contract for rail/tree/entries/panels. |
| scope.new_public_symbol | yes | D2 settles an ordered topic-unit list on the page source; before it, current PageContent.Board carries wswhiteboard.Regions containing HTML, not structured topic units (agents-plugin-tool/internal/wsdashboard/page.go#L46-L61; agents-plugin-tool/internal/wswhiteboard/render.go#L310-L326). |
| scope.new_type_contract | yes | D4 requires changed rows plus order for rail/tree/entries/panels; current regionEvent carries only name/hash/html (agents-plugin-tool/internal/wsdashboard/sse.go#L213-L218), and boardTopicsEvent is board-specific (agents-plugin-tool/internal/wsdashboard/board_topics.go#L26-L32). |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/board_topics_test.go, board_topics_stream_test.go, board_topics_client_test.go, stream_cache_test.go, pages_test.go, rail_test.go and rail_hover_browser_test.go cover reusable parser, stream, shared-render and hover contracts; new regression cases are required by the phases. |
| complexity.reuse_points | confirmed | Read the existing board memo in agents-plugin-tool/internal/mcp/dashboard_page.go#L83-L205, topic hash rendering in agents-plugin-tool/internal/wswhiteboard/render.go#L783-L806, shared page cache in agents-plugin-tool/internal/wsdashboard/stream_cache.go#L176-L235 and opt-in diff/fallback in board_topics.go#L102-L119. |
| complexity.side_effect_risk | high | Shared cached units and per-connection row baselines alter every open dashboard page's update path; missed invalidation or accumulated hidden updates can leave stale state. |
| risk.correctness | high | Topic output depends on more than body recency: tickets do not move TopicDoc.Changed (agents-plugin-tool/internal/wswhiteboard/store.go#L234-L246), and the existing board memo also tracks watched files, language and mailbox/process inputs. |
| risk.fit | moderate | Preserve the source-owned board memo, shared language/theme cache, live-only reads, transient-read retention, first-full/non-topic fallbacks and head B's hover fast path while replacing their render/diff granularity. |
| risk.test | high | Phase 1 needs paired CPU/render/byte measurements on a 112-topic fixture and Chromium parity; current board_topics_stream_test.go uses pre-rendered two-topic HTML and cannot prove real topic render counts. |
| risk.security_or_contract | high | D6 requires old tabs to remain usable, but assets/dashboard-stream.js#L248-L252 banners rather than reloads with drafts or a repeated identity; new row events need an explicit compatibility/fallback contract without changing persisted stores or MCP calls. |

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
rendering, a one-topic edit renders one topic, and declaring topic A as
waiting on topic B re-renders B (its reverse-waiter markup) without
re-rendering unrelated topics, the existing
board-topic/stream/region tests, and a Chromium check that the page shows
the same board after a topic edit.

### Result (3a6a3f3d2) - 2026-11-15

Implemented Phase 1 only. The source-owned MCP memo now caches each topic by
its complete projected render input, including reverse waiters and board-wide
reference membership, plus shared language/mailbox/process and external-file
inputs. Cache hits retain image watches. Folder metadata identity and sequence
alone do not invalidate topics. Body/panel markdown and topic templates run
only on misses; the dashboard consumes immutable ordered units and caches the
assembled region, hash and decomposition per theme.

Each unit is exactly the section span returned by `splitBoardTopics`:
inter-section whitespace stays in its original position in the full board and
in the parser-equivalent shell, never in a wire unit. Regression tests compare
every unit, shell, full-region HTML and hash on 112 topics in auto/light/dark
themes. Unfamiliar layouts retain the parser/full-region fallback. D6 remains
T0: `PageContent.BoardUnits` is in-process only (`json:"-"`), the existing
`topics=1` flag/event payload and first-full/legacy-region behavior are unchanged,
and neither stores nor MCP calls/hello changed.

**Measurements.** All measurements used disposable in-process fixtures, not
the shared daemon. Baseline: `cf6d129b9`; candidate: `3a6a3f3d2`. The main
fixture has 112 topics and 4,032 entries, with 200 measured operations per row.
Source and session-transport stages were measured separately; these are not
end-to-end request latencies. CPU/op is total sampled process CPU divided by
200, including runtime/GC; wall time is sensitive to host scheduling.

| Stage / change | CPU ms/op, before -> after | Wall ms/op, before -> after | Board delta JSON bytes, before -> after |
|---|---:|---:|---:|
| Source, non-board read | 5.35 -> 5.25 | 11.048 -> 12.706 | n/a |
| Source, one small body edit | 270.45 -> 25.75 | 473.160 -> 49.222 | n/a |
| Session render/diff, non-board change | 23.80 -> 0.20 | 17.935 -> 0.191 | 0 -> 0 |
| Session render/diff, one small body edit | 25.05 -> 2.30 | 23.038 -> 1.708 | 26,406 -> 26,406 |

The initial assembled board remains 2,214,962 bytes; the edited state remains
2,214,297 bytes, before and after. This phase saves render/tokenizer work,
not additional wire bytes: the old `topics=1` path already sent one topic.
Baseline transport profiles attribute cost to `html.Tokenizer`; the new
transport profiles contain no tokenizer samples. Unchanged source reads are
essentially CPU-flat, as expected from the pre-existing whole-board memo.

The existing isolated idle-stream fixture (112 topics, 40 non-board snapshot
wakes at 4 Hz, about 10.4 seconds) sampled **0.55 -> 0.44 CPU seconds**
(**5.33% -> 4.22%** of one core over the profile windows). This is synthetic
snapshot churn, not a new measurement of the live daemon's idle percentage.
Raw logs, profiles, top reports and saved scratch harnesses are under
`/tmp/exec5/btp1/{before,after}/` and `/tmp/exec5/btp1/*-scratch.go`; scratch
tests were removed from the worktree before committing.

**Verification.** Every Go command used `GOPROXY=off GOSUMDB=off
GOTOOLCHAIN=local`, `-count=1`, and focused `-run` patterns; no full dashboard
package run was performed.

- `./internal/mcp`: dashboard board/memo/read-page/read-cache/closed-topic
  cases and `WhiteboardStream` passed, including non-board zero topic renders,
  one-topic edits, A waiting on B rendering A+B only, sequence/file-identity
  independence, external invalidation and retained image watches.
- `./internal/wsdashboard`: board-topic parser/client/SSE/WebSocket, unit
  parity, stream/page-cache/region and transient-read cases passed. Node
  fragments ran through their Go tests, not as standalone source files.
- `./internal/wswhiteboard`: topic-unit/hash, stream/history, panel reference
  and waiting-view cases passed. The eager full-render parity test also covers
  deferred history and English/Korean output.
- `go test -race ./internal/mcp -run '^TestDashboardBoardTopicCache'` and
  `go test -race ./internal/wsdashboard -run
  '^TestBoardTopicUnits(ReuseAndParity|Streams)$'` passed.
- `TestBoardTopicUnitsBrowser` passed with the supplied Playwright module and
  Chromium cache: one-topic WebSocket delta, unchanged topic node retained,
  existing reader-anchor hold/release preserved, streamed/fresh board signatures
  equal, no page errors. Compared `/tmp/exec5/btp1/browser-after.png` vs
  `/tmp/exec5/btp1/browser-fresh.png`: match.

The lead authorized one retry after stop (f). The server-side whitespace defect
was fixed without relaxing `dashboard-board.js`'s single-element validator.
The subsequent Chromium timeout was diagnosed from captured events/DOM as the
existing visible-reader gate, and the fixture now uses its apply-latest control
instead of assuming immediate replacement. Direct author-performed lite review
covered the source and tests because subagents were prohibited; this was not an
independent review. Evidence: `/tmp/exec5/btp1/self-review.md` and the final
`{dashboard,whiteboard,mcp}-final.log` files. No remaining finding from that pass.
Phases 2-5 remain unimplemented; the ticket stays in `ready/`.

### Phase 2: Rail row deltas

Per D4: identify rail rows by `data-rid`, render rows on each rail wake,
and ship only rows whose rendered-HTML hash changed, plus order, instead of
the whole rail region; keep head B's hover fast path semantics. Verify
bytes per rail change and daemon CPU against the 7.7MB per 32s baseline,
a regression test that a clock-only liveness change ships that row, plus
the existing rail tests and the
`TestRailHoverUpdatesBrowser` regression.

### Result (af2b5b889) - 2026-11-15

Implemented Phase 2 only. Each shared rail render decomposes the unchanged
full HTML into `data-rid` rows, complete rendered-HTML hashes and inter-row
shell fragments. Rows still render on every rail wake, including wakes with
unchanged session revisions. An opted-in connection (`railrows=1`) receives
`rail-rows` with changed rows plus order. Exact shell equality proves the slot
baseline; grouping, layout-generation, row-count or other shell changes use
the full region. First sends remain full even with `h=current`, failed writes
do not advance baselines, and uncertain decompositions reset by a full send.

The client retains immutable canonical inputs separately from marked DOM,
accumulates hidden/interaction-gated changes and parses only changed rows on
ordinary deltas. Unchanged rows retain their nodes. Hover-only changes replace
only inert sources, without remounting spinners, changing focus or reapplying
marks. Existing drag/rename/layout gates, generation checks, rail metadata,
folds and scroll/focus restoration remain in place.

**Compatibility, directional T0.** New daemon -> old page: only the explicit
query flag allocates row-event state (`wsdashboard/sse.go:455-462`); without
it, full region delivery is unchanged (`wsdashboard/sse.go:627-655`). This also preserves
old tabs that banner instead of reloading after the assets identity. New page
-> old daemon: the additive query flag is ignored by the old stream, while the
new page still consumes full rail regions
(`wsdashboard/assets/dashboard-stream.js:182-211`). A missing or invalid client
baseline closes the connection and reconnects for the guaranteed first full
(`wsdashboard/assets/dashboard-stream.js:213-224`; `wsdashboard/sse.go:632-650`). No persisted data, MCP tool,
hello, manifest or Pi harness surface changes, so reopening needs no migration
or recovery. Paths in this paragraph are under
`agents-plugin-tool/internal/`.

**Measurements.** Baseline `59a283670`; candidate `af2b5b889`. Both used the
saved disposable in-process daemon fixture: 112 rail rows, one rail SSE
subscriber, separate cache/config/state directories and an ephemeral loopback
port. Each window is 32 seconds; the change window makes 27 one-row
snapshot/status changes at 1 Hz, with 27 rail renders and 27 events before and
after. No shared daemon or board was contacted or modified.

| Measure | Before -> after |
|---|---:|
| Initial stream event bytes | 409,126 -> 409,126 |
| Change SSE bytes, total including framing | 11,044,503 -> 130,590 (**98.82% less**) |
| SSE bytes per change, alternating status | 409,034 / 409,079 -> 4,815 / 4,860 |
| Idle process CPU seconds / 32s | 0.048773 -> 0.076003 (0.152% -> 0.238% of one core) |
| Change process CPU seconds / 32s | 1.138686 -> 1.062296 (3.558% -> 3.320% of one core) |
| Sampled cumulative stream CPU | 0.41s -> 0.49s |
| Sampled cumulative rail template CPU | 0.24s -> 0.27s |

VERIFIED: the fixture saves wire bytes, while total CPU is roughly flat and
the coarse stream/render CPU samples increase; row decomposition accounts for
0.11s in the candidate profile. Process CPU includes the fixture driver,
source/index processing, runtime/GC and profiling. These single samples do
not establish a daemon CPU improvement. Render wall latency was not separately
instrumented. INFERRED: this removes the same whole-rail wire waste as the
live baseline, but the live 7.7MB/32s (27 pushes of about 287KB) and 70-106%
daemon CPU are different workloads, not remeasured here. This rail-only
fixture has about 409KB full events and runs no board/topic templates.

Raw evidence: `/tmp/rail-phase2-evidence/summary.md`; baseline logs/profiles in
`before/`, final candidate logs/profiles in `after/final/`, local profile top
reports beside them, and the removed scratch harness retained as
`measure-scratch.go`. The earlier candidate measurement is retained too.

**Verification.** All Go tests used `GOPROXY=off GOSUMDB=off
GOTOOLCHAIN=local`, `-count=1` and focused `-run` filters; no entire dashboard
or MCP package, `./...`, network/provider, installation, shared-runtime
operation, process signal, push or merge ran.

- `TestRail*`, shared-rail and shell-stream tests passed, including exact
  `TestRailHoverUpdatesBrowser` and `TestRailDragLiveBrowser` (20/20 drops
  under churn). Cached Playwright/Chromium also passed rail metadata,
  lifecycle, worktree-admission, child-busy and static-drag cases.
- New regressions cover clock-only liveness shipping its row, rendering on an
  unchanged-revision wake, complete HTML/hash parity, changed-row-only SSE and
  WebSocket events, legacy full regions, side rails, first-full reconnect,
  failed writes, uncertain baselines, hidden accumulation, keyed order and
  invalid client input/recovery. Focused rail-row race tests passed.
- Board-topic/unit, page stream/reload, region filter, shared-render and assets
  identity regressions passed. Complete successful output is in
  `after/focused-green.log` and `after/race.log`.
- The first broader verification command exposed obsolete exact stream-URL
  expectations in two existing Node tests. Both expected no new opt-in flag;
  the production URL correctly added the D6-approved `railrows=1`. Updated
  those contracts and added shell-opt-in/frame-exclusion and recovery cases.
  Failed output and diagnosis remain in `after/focused-final.log` and
  `failure-diagnosis.md`; the subsequent focused runs passed without skipped
  or removed tests or raised timeouts.
- Independent one-pass lite review was clean, with no findings. Report:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@a93cfe66/review-paths/aa2d6db2-01-rail-phase2-lite.md`.

Baseline screenshots were captured before source edits. Compared
`/tmp/rail-phase2-evidence/after/real-expanded-dark-crop.png` vs
`/tmp/rail-phase2-evidence/before/real-expanded-dark-crop.png`: match.
Compared `/tmp/rail-phase2-evidence/after/real-expanded-light-crop.png` vs
`/tmp/rail-phase2-evidence/before/real-expanded-light-crop.png`: match.
Expanded and collapsed rail-row crops are byte-identical PNGs in both themes;
checksums are in `visual-sha256.txt`. Full viewport captures are preserved as
`{before,after}/real-{expanded,collapsed}-{dark,light}.png` for MAIN's premerge
comparison; disposable workspace labels differ outside the row crops.

Phase 1's Result and all phase plans are unchanged. Phases 3-5 remain
unimplemented; the ticket stays in `ready/`, with branch and lease retained
for MAIN. No unresolved implementation or review finding.

### Phase 3: Tree row deltas

Apply the Phase 2 unit-and-delta pattern to `tree`. Verify bytes per tree
change against the baseline rows (32 tree pushes of ~97KB in 34s) and the
existing tree/region tests.

### Phase 4: Entries row deltas

Apply the same pattern to `entries`, keeping its title bar outside the
keyed container (`b150fe83`). Verify bytes per entries change against the
baseline rows and the existing entries/region tests.

### Phase 5: Panels row deltas

Apply the same pattern to `panels`, keeping panel disclosure state.
Verify bytes per panels change against the baseline rows and the existing
panels/region tests.
