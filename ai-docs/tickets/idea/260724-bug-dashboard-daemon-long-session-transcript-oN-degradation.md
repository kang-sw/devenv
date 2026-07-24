---
title: "ws-dashboard daemon: long-uptime responsiveness degrades from O(conversation) live-transcript rebuild/refetch + unbounded projector growth"
related:
  260723-bug-dashboard-terminal-http-poll-oN-fallback: sibling O(N) fix in the
    terminal-output subsystem; this ticket is a distinct accumulation point in
    the agent-chat/transcript subsystem NOT covered by that redeploy
related-mental-model:
  - ws-web-dashboard
---

# ws-dashboard daemon: long-uptime responsiveness degrades from O(conversation) live-transcript rebuild/refetch + unbounded projector growth

## Symptom

Reported during long dogfooding uptime on 2026-07-24: overall ws-dashboard
daemon responsiveness dropped significantly. Terminal panes are "so-so", but the
sharpest clue is that **an agent/CLI session with a LONG conversation history is
slow even though it is just a CLI session**. The operator suspected an O(n) path
or in-memory accumulation growing with conversation length and/or uptime, and
suspected a **redeploy (restart) would clear it** — i.e. the growth is in runtime
state, not persisted state.

That hunch is largely correct: a restart *masks* the degradation by discarding
the accumulated in-memory projector transcripts (see suspect #3), but a restart
is a side effect, not a fix — the O(n)-per-tick work returns and re-accumulates
as soon as sessions run long again.

## Pending-redeploy relationship (why this is NOT the terminal fix)

Two O(N) **terminal-output** defects were already fixed on the goal branch but
were not yet deployed at report time:

1. Terminal PTY output re-render O(N) -> rAF-coalesced
   (`createOutputCursorFlushScheduler`, `frontend/src/terminals.ts`).
2. Terminal HTTP-poll fallback O(N) -> one batched request/tick with front-trim
   (sibling ticket `260723-bug-dashboard-terminal-http-poll-oN-fallback`, now
   `.done/`).

Both are terminal-pane specific and touch none of the agent-chat / transcript
subsystem. **This ticket is a distinct accumulation point** — the pending
terminal redeploy does not resolve the long-CLI-session slowness. All suspects
below are in the agent-chat subsystem.

## Confirmed suspects (static-traced; file:line anchors)

### #1 (PRIMARY) — Live transcript is fully rebuilt + refetched every 1.5 s, O(conversation) per poll

- Frontend live-turn poll loop: `frontend/src/activitySessionClient.ts:423`
  `realStreamingPollIntervalMs = 1500`; `beginRealStreamingTurn`
  (`activitySessionClient.ts:447-518`) calls `fetchRealTranscript` every 1500 ms
  while the turn is live.
- `fetchRealTranscript` (`activitySessionClient.ts:155-166`) GETs
  `.../{codex|claude}-sessions/{activityId}/transcript` with **no
  cursor/before/limit** — it always requests the whole transcript.
- Those routes resolve to the in-memory projector path (not the paginated file
  path): `crates/daemon/src/claude_routes.rs:160` -> `claude_activity_transcript`
  (`crates/daemon/src/claude_cli.rs:882-903`) and the codex twin
  `crates/daemon/src/codex_app_server.rs:~882-899`. Both build
  `blocks = projector.transcript_blocks()` and return `next_cursor: None,
  has_more: false` — **no pagination for live sessions**.
- `transcript_blocks()` rebuilds the entire Vec from scratch on every call,
  cloning every string of every block:
  - Claude: `crates/core/src/claude_projection.rs:175-201` (iterates all of
    `self.order`; `render_kind.clone()`, `title.clone()`, `text.clone()`,
    `role.clone()`, `turn_id.clone()` per block).
  - Codex: `crates/core/src/codex_projection.rs:214-233` (identical shape).

Cost per poll = O(conversation length): daemon rebuild+clone + full JSON
serialize + full HTTP body + frontend parse, repeated every 1.5 s for the whole
live turn. The frontend does diff to a tail delta
(`blocksSincePolledLength`, `activitySessionClient.ts:478`) so render is bounded,
but daemon CPU/alloc + serialize + transport + parse are all O(n) per tick and
unmitigated. Scaling key: conversation block count × 1.5 s live-poll cadence.

### #2 — Activity-feed poll rebuilds the FULL transcript of every live session just for a boolean, every 3 s

- Feed poll cadence: `frontend/src/App.tsx:444`
  `workRootActivityRefreshIntervalMs = 3_000`; effect at `App.tsx:4790-4839`
  calls `fetchWorkRootActivity` every 3 s for the selected work root (skipped
  when `document.hidden`).
- Daemon `/activity` handler `work_root_activity`
  (`crates/daemon/src/work_root_activity.rs:187-223`) merges
  `codex_activity_items` + `claude_activity_items`.
- `claude_activity_item` (`claude_cli.rs:846-877`) and `codex_activity_item`
  (`codex_app_server.rs:845-871`) each compute
  `let has_transcript = !projector.transcript_blocks().is_empty();`
  (`claude_cli.rs:850`, `codex_app_server.rs:849`). This rebuilds+clones the
  entire transcript of every live session **purely to derive a boolean and a
  "0" cursor**; the built Vec is discarded.

Even with no transcript pane open, every 3 s the daemon rebuilds the full
transcript of every live codex/claude session in the selected root. Cost =
O(sum of conversation lengths across live sessions) per 3 s — pure waste; a cheap
`is_empty()`/count accessor on the projector would suffice.

### #3 — Projectors never evict blocks: unbounded in-memory transcript per live session

- `ClaudeProjector` (`claude_projection.rs:119-145`) and `CodexProjector`
  (`codex_projection.rs:103`) hold `order: Vec<String>` + `blocks: BTreeMap<..>`
  that only ever grow per turn. Grep for
  `retain|evict|truncate|cap|drain|pop_front` over both files returns only the
  *diagnostics* "bounded" comments — no block eviction/cap exists. "Bounded"
  applies to the diagnostics vector, not the transcript.
- Dead sessions themselves ARE pruned: `codex_app_server.rs:603`
  `sessions.retain(|_, s| s.is_live())` and `claude_cli.rs:616-627`
  `list_for_work_root` filters `is_live()`. So the leak is bounded by the number
  of currently-live sessions — but each live session's transcript grows unbounded
  for its lifetime, and that in-memory Vec is exactly what #1 and #2 rebuild.

This is the "in-memory runtime state cleared by restart" the operator suspected —
the substrate that makes #1/#2 progressively worse the longer a session runs, and
why a redeploy appears to resolve it. Scaling key: per-session conversation
length, cleared only on daemon restart.

### #4 (secondary, client-side) — Frontend re-groups + re-parses markdown for the whole transcript every render, no windowing/memoization

- Pane holds the full transcript in state and grows by append:
  `frontend/src/agentChatSessions.ts:201`
  `blocks: [...session.transcript.blocks, block]`.
- Every render, `frontend/src/agentChatPaneBody.tsx:386-389` computes
  `mergeStreamingTranscriptBlocks(session.transcript.blocks, streamingBlocks)`
  over the full array, then passes it to `AgentChatTranscriptBubbles`.
- `AgentChatTranscriptBubbles` (`frontend/src/agentChatBubbles.tsx:330-357`)
  calls `groupTranscriptIntoBubbles(blocks, sourceKind)` **with no `useMemo`**;
  `bubbles.map(...)` renders every bubble and each `ChatBubbleView` runs
  `renderMarkdownFragment(bubble.text)` (`agentChatBubbles.tsx:242,267`). No
  virtualization/windowing.
- Reconcile also builds an O(n) `Set` of all resolved cursors per poll
  (`agentChatSessions.ts:256-260`) and maps the full block array
  (`agentChatSessions.ts:272`).

Each 1.5 s poll delta triggers a state update -> full re-group + full markdown
re-parse of the entire conversation. Stable React keys limit DOM mutation, but
grouping + markdown fragment construction is O(conversation) CPU per tick in the
browser — a parallel client-side contributor to the long-conversation slowness,
independent of #1's daemon cost.

### #5 (secondary) — Historical/resumed transcript re-reads + re-parses the entire `.jsonl` per paginated poll

- `named_agent_transcript_blocking` (`work_root_activity.rs:1340-1498`) does
  `std::fs::read_to_string(native_path)` (`work_root_activity.rs:1389`) then
  `parse_codex_session_transcript(&raw)` (`work_root_activity.rs:1391`), which
  splits all lines and parses every record
  (`work_root_activity.rs:1523-1568`) before `paginate_transcript_blocks`
  windows the result (`work_root_activity.rs:1281-1312`). The window bounds the
  *response*, but the read+parse is O(file size) every call.
- Serves the paginated transcript route
  (`work_root_activity.rs:243-272`) for named/historical agents; polled less
  aggressively than the live 1.5 s loop, so lower live-symptom weight, but a
  resumed long session still re-reads+re-parses a large `.jsonl` per poll.

## Ruled out / bounded (not growth vectors)

- Codex dialogue dedup `is_duplicate_codex_dialogue_block`
  (`work_root_activity.rs:1892-1902`) only compares `blocks.last()` -> **O(1)**,
  so #5's parse is O(n), **not** O(n^2). No quadratic dedup exists.
- `stderr_tail` buffers are bounded: `claude_cli.rs:362-366` and
  `codex_app_server.rs:457-461` drain overflow past `MAX_STDERR_LINES`.
- Pending JSON-RPC maps (`codex_app_server.rs:185,287,301`; `claude_cli.rs:245`)
  remove entries on response/timeout — not a leak.
- `events.rs` `ActivityEventStore.events` is loaded from a static fixture
  (`include_str!(".../instance_events.json")`, `events.rs:28`) — a mock, not a
  growing runtime log. `runtime_debug_events` is an MCP tool, not a daemon
  in-memory log in this tree.
- Session registries prune dead sessions (`retain(is_live)`), so registry size is
  bounded by live-session count, not uptime.

## Verification gap

All findings above are **static-traced only** — no profiling under load was done.
To quantify and confirm before committing to a fix:

- Daemon CPU profile / flamegraph while one long (thousands-of-blocks) live
  session polls `/transcript`, to measure `transcript_blocks()` rebuild + serde
  cost per tick vs. total request time (confirms #1 dominance).
- Open the activity feed (3 s poll) with several long live sessions and **no
  transcript pane open** — expected steady CPU proportional to total block count
  (isolates #2).
- Confirm whether multiple work roots each run their own 1.5 s / 3 s loops
  concurrently (multiplying n across roots); the poll effects are per selected
  root in `App.tsx`, but background roots' live sessions still hold growing
  projectors (#3).
- React Profiler on the agent-chat pane during a live long-history turn to
  quantify #4's per-tick grouping + markdown cost.
- Confirm the resumed-session poll cadence for the paginated route to weight #5
  relative to #1.

## Fix directions (names only — not decided, not authorized)

- #1: incremental/append transport (send only blocks after a client-held cursor;
  the paginated route already supports `cursor`/`limit`, the live routes bypass
  it), or push deltas (SSE/websocket) instead of full-refetch polling; cache the
  projected `Vec<TranscriptBlock>` and invalidate on ingest.
- #2: replace `!transcript_blocks().is_empty()` with a cheap `block_count()`/
  `is_empty()` accessor; never materialize the full Vec for the feed item.
- #3: bounded ring buffer / windowed retention with "load older" backfill from
  the on-disk transcript, or a dirty-flag + memoized projection.
- #4: `useMemo` on `groupTranscriptIntoBubbles`, `React.memo` `ChatBubbleView`,
  bubble-list virtualization, memoize `renderMarkdownFragment` per block.
- #5: incremental file tailing (byte-offset / parsed cursor) instead of full
  `read_to_string` + full re-parse, or an in-memory parsed cache keyed by file
  mtime/len.

## Relation to other work

Sibling: `260723-bug-dashboard-terminal-http-poll-oN-fallback` (`.done/`) fixed an
O(N) fallback in the **terminal-output** subsystem. This ticket is the analogous
O(N) shape in the **agent-chat/transcript** subsystem — a **distinct accumulation
point not covered** by that fix or the pending terminal redeploy.

## Reporter context

Captured from a dogfood-surfaced long-uptime degradation report on 2026-07-24.
The operator's plan was to cycle/redeploy to assess long-term stability; this
ticket records that a redeploy only masks (does not fix) the accumulation. Not yet
reproduced under controlled load or profiled — see Verification gap. Idea-tier
investigation; **not** ready-queue material and not authorized for the drain
queue.
