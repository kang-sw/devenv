# Fresh session load: code-level audit (2026-11-16)

Baseline: develop `e6e0ebf44`. This is source/byte accounting, not a browser
trace, latency measurement, compressed transfer measurement or CPU benchmark.

## Request and work breakdown

1. A shell rail pick keeps the outer document/rail stream, saves the old
   frame's draft synchronously, navigates it through native beforeunload, removes
   it, then creates `/s/<id>?frame=1`. No cached session DOM survives. A cancelled
   leave keeps the old frame. Ordinary switches do not fetch another shell page.
2. `pages.go handleSessionPage` calls `readSessionWith(...nil,true,true)`:
   session/source page, snapshot and liveness, conversation ring and, with an
   adapter, held/drained inbox records and attachment metadata. File/entry caches
   can avoid repeated disk/markdown work; this request still builds a view and
   renders all session regions. `renderSessionRegions`/`renderSessionPageData`
   generate fresh HTML including the initial board and newest conversation
   window. No client board-HTML GET is needed before first paint. These are
   source-backed operations, not measured per-request milliseconds.
3. Baseline session HTML inlines CSS, sprites and client code. Raw source bytes:
   dashboard.js ordered concatenation **591,982 B**, tracker.js **521,616 B**,
   dashboard CSS **254,253 B** (excluding extra memo/usage styles), cards.js
   **74,667 B**, files.js **85,494 B**, terminal.js **29,069 B** when offered.
   Each new document parses these again; inline bytes are retransmitted in the
   no-store HTML. Language strings and template conditionals remain per-page.
4. The main script restores folds, constructs card/toolbar state, binds forms,
   performs initial board/frame setup and time formatting, restores drafts,
   mounts tracker hooks, and connects streams. Topic documents already use
   inert srcdoc placeholders, near/far viewport bands and paced mount slices;
   Mermaid is demand-loaded. Exposure checks and frame-height handling are
   explicitly outside this phase and unchanged.
5. `dashboard-stream.js connect` opens **one WebSocket** for the frame's session
   events, with rendered-region hashes, `frame=1` and keyed-row opt-ins. It is
   the SSE-compatible event protocol over WebSocket, not a second SSE request.
   `sse.go`/`stream_cache.go` share stream renders, send only changed region
   hashes, coalesce wakes at 250ms, and recheck freshness on a 15s tick. Opening a
   fresh document does not require a duplicate full client region fetch; a stale
   region may legitimately arrive between initial HTML and stream connect.
6. Conditional requests: hosted/handed session lead state; tracker reads on its
   initial revision; Files roots/tree only when shown; conversation history near
   its top; images/attachments and near-view topic-body resources. Counts depend
   on session contents and viewport. No constant whole-session request count is
   claimed for these content-dependent branches.
7. `terminal.js mount/reconcile` requests `/s/<id>/terminals` once per mount;
   visibility, drawer and folder-popover actions can overlap that request. The
   server's `listTerminals` enumerates terminal state and folders, not output.
   Baseline every restored terminal opens xterm, measures/fits it and connects
   `/s/<id>/terminals/<tid>/ws?from=0`, even off-screen or folded. **Backlog is
   replayed over that WebSocket; there is no separate backlog HTTP GET.** The
   holder's ring replay/JSON/base64 decode and emulator writes are real repeat
   work. A fresh emulator cannot resume at an old cursor without restoring its
   screen/buffer; doing so would silently omit output. This phase retains full
   replay when the card is actually needed.
8. Xterm/addon JS, symbols WOFF2, Mermaid and body-frame assets already have
   content-addressed URLs and immutable caching (`terminal_routes.go`,
   `serveMermaid`, `serveFrameAsset`). Those do not need a second cache layer.
9. `restoreDraft` reads local storage synchronously, validates references,
   removes unavailable pasted-image markers, restores composer/answer/chip and
   fold/tab state, then lays out the visible document. It performs no draft
   network request. Before Phase 2 no per-session scroll position was retained
   outside the removed document. The shell temporarily shows an empty own-header
   until the new frame posts its header; that handshake is unchanged.

## Contained changes selected

- Make only the framed session's shared main script a content-addressed static
  asset, preserving its position after its prerequisite inline modules. Shell,
  index and standalone fixture pages retain their existing inline path. No
  bundled-module reorder or deferred main initialization. Cache saves transfer
  and enables engine code caching; JavaScript initialization still runs anew.
- Coalesce concurrent terminal-list reads, but do not retain a stale list after
  completion. Defer emulator/font/socket/full-ring replay for off-screen
  terminal cards until near-view. Near folded cards still replay for the existing
  last-line preview. No terminal-output or session DOM cache.
- Save bounded, tab-local per-session scroll metadata separately from drafts;
  restore pane/document offsets after startup, retain conversation bottom-follow,
  and let explicit hashes/user input win. Board placements go through existing
  wbHistory ownership, not direct writes to its owned scrollers. No exposure,
  frame-height or composer implementation changes.

## Before/after accounting

Numbers below are raw source/request-path counts, **not** compressed wire bytes,
whole-page sizes or measured load times. `N` is restored terminal cards; `K` is
cards near the viewport (including folded cards with last-line previews). Data-dependent ring replay bytes remain
unmeasured. Main-script size was counted using the ordered-source Node helper.

| Operation | Before (`e6e0ebf44`) | After |
|---|---|---|
| Main script tag in each session HTML | 591,999 B (591,982 B source + 17 B tag) | 64 B external tag: **591,935 B less HTML** in this bucket |
| Main script body requested separately | 0 requests; retransmitted inside every no-store document | 1 cold GET of 595,238 B; 0 network fetches when immutable browser cache is warm |
| Cold main-script bucket, HTML tag + asset | 591,999 B | 595,302 B: **+3,303 B** and +1 request cold (new view-state logic plus URL tag overhead) |
| Concurrent terminal list callers (example: 2) | 2 GETs / duplicate enumeration | 1 GET, 2 callbacks; later refresh still makes a fresh GET |
| Initial terminal emulator instances / sockets / ring replays | N / N / N | K / K / K; N-K postponed until near-view |
| All restored terminals off-screen | Emulator/font demand + N sockets and full ring replays | 0 emulator/font demand, 0 terminal sockets, 0 terminal backlog replay until needed |
| Draft restore requests | 0 (local storage) | 0; unchanged draft format/guard |
| Retained old session documents after accepted switch | 0 | 0; unchanged lifecycle |
| Return scroll state | Lost with the old frame | At most 50 tab-local metadata records; restore visible panes/document through existing board placement ownership |

Warm-cache claims assume ordinary immutable HTTP-cache behavior; this phase did
not launch a browser to measure actual cache hits or V8 parse/code-cache reuse.
The new document still initializes all loaded modules. Tracker/CSS and other
inline code remain in HTML: moving those too needs its own load-order/fixture
review rather than an unmeasured broad split.

Scroll records are numeric current-layout offsets, not retained DOM/history or
terminal buffers. A changed document can clamp them, and conversation entries
older than the server's newest window are not proactively re-fetched merely to
reconstruct an old reading anchor. Returning within the rendered window, hidden
pane offsets and bottom-follow are covered by the deterministic tests. Live
acceptance should include long history, layout/font changes and late body-frame
heights; do not infer exact historical-entry restoration from this implementation.

Live Chrome/Electron first-usable timings, compressed transfer and terminal-ring
sizes remain with the lead after the arranged daemon swap. Open latency target
remains an owner question. Executed verification is recorded in the Phase 2 Result.
