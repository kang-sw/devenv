---
title: "Codex fork-from-here silently forks the whole thread instead of cutting, for any live user-sent bubble (cutCursor never resolves)"
related:
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: blocks
  260713-feat-ws-dashboard-activity-session-fork-cursor: related
related-mental-model:
  - ws-dashboard-agent-harness
---

# Codex fork-from-here silently forks the whole thread instead of cutting, for any live user-sent bubble (cutCursor never resolves)

## Background

Discovered during the manual, real-daemon/real-`codex`-CLI Phase 4 walkthrough
of `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` (2026-07-20).
Reproduced against a genuinely live session (not a stub, not a fixture):

- 2 real turns sent in a Codex agent-chat session.
- Clicked "Fork from here" on the **first** user message bubble.
- Request sent (confirmed via direct network/daemon API inspection):
  `{"action":"fork","cutCursor":"user-sent-mrsxjeyh-1"}` —
  `user-sent-mrsxjeyh-1` is a client-generated pending-bubble id (format
  `user-sent-<base36-timestamp>-<sequence>`), not a real transcript cursor.
  Real transcript cursors returned by `/transcript` are plain sequential
  strings (`"0"`, `"1"`, `"2"`, `"3"`, one per block).
- Response received: `{"applied":true,"data":{"activityId":"codex:...",
  "cutCursor":null}}` — `cutCursor: null` means the daemon's cursor
  resolution failed, silently (no error surfaced to the client).
- Consequence confirmed via a direct `curl` against the **forked** session's
  own `/transcript` afterward: all 4 blocks (both turns' Q+A) are present —
  the fork did not truncate at all; it silently forked the entire original
  thread instead of cutting at the clicked message.

## Investigation (this capture pass — no product code touched, per instruction not to fix this pass)

Traced the client-side mechanism that produces the bad `cutCursor` value.
This is more than the originally-flagged "plausible lead" — reading the
actual reconciliation path confirms the optimistic cursor is never replaced,
not merely suspected to persist:

- `forkAgentChatFromBubble` (`ws-dashboard/frontend/src/App.tsx:5456-5470`)
  reads `bubble.blocks[bubble.blocks.length - 1].cursor` off the clicked
  bubble and sends it straight through as `cutCursor` for the real Codex
  fork call (`App.tsx:5502`, `cutCursor: lastCutBlock?.cursor ?? null`).
- Every optimistically-sent user message gets its cursor minted by
  `appendUserTranscriptBlock`
  (`ws-dashboard/frontend/src/agentChatSessions.ts:181-187`):
  `` `user-sent-${Date.now().toString(36)}-${userTranscriptBlockSequence}` ``
  — exactly the format seen in the repro request. `sendAgentChatMessage`
  (`App.tsx:5412-5419`) calls this and writes the resulting block directly
  into the pane's **canonical** `session.transcript.blocks` array via
  `applyAgentChatSession`.
- The real send/receive poll (`beginRealStreamingTurn`'s `onUpdate` callback,
  `App.tsx:7650-7663`) never touches `session.transcript.blocks` at all — it
  only writes into a separate `streamingBlocks` record state, keyed by
  cursor. Rendering merges the two via `mergeStreamingTranscriptBlocks`
  (`ws-dashboard/frontend/src/agentChatStreamMerge.ts:21-29`): a streaming
  block whose cursor **matches** an existing canonical block's cursor
  replaces it in the rendered view; one whose cursor does not match is
  merely appended after. Since the daemon's real transcript cursors are
  plain sequential strings ("0", "1", "2", ...) that never equal the
  client-minted `user-sent-...` cursor, the poll can only ever *append* a
  second, separately-cursored copy of the same message — it never
  substitutes the canonical entry's stale cursor for the daemon-confirmed
  one.
- Net effect: the canonical `session.transcript.blocks` entry for a live,
  freshly-sent user message keeps its client-only optimistic cursor for the
  entire life of that pane. There is no reconciliation step anywhere in this
  path that rewrites a bubble's block cursor once the real transcript poll
  resolves the same logical message to a real cursor. Any "fork from here"
  click against that bubble hands the daemon a cursor format
  (`user-sent-<rand>-<n>`) it structurally cannot resolve — this is not
  intermittent, it is the code's only behavior for any live user-sent
  message.
- Consistent with the repro: the daemon does not error on an unresolvable
  cursor, it falls back to forking the whole thread and reports that
  fallback only via `cutCursor: null` in the response body — easy to miss
  since `applied: true` still reads as success.
- Not independently re-confirmed this pass (inferred from code, not a second
  live repro): a bubble whose block was loaded via initial resume/hydration
  (rebuilt from a fresh real `/transcript` fetch, never touched by
  `appendUserTranscriptBlock`) should already carry a resolved daemon
  cursor and should therefore fork correctly. If a fix attempt here starts
  from "only live-sent bubbles in the current session are affected," that
  should be verified with a live click on a resumed/hydrated bubble before
  relying on it.
- Out of scope for this capture pass, per instruction: any actual client/
  server cursor-reconciliation design (e.g., swapping the optimistic block's
  cursor for the real one once the poll confirms it, or having the daemon
  match a `user-sent-...`-shaped cursor by content instead of id) — this is
  real design work, not a one-line patch, and belongs to whichever phase
  below takes it on.

## Phases

### Phase 1: Reconcile optimistic user-block cursors with server-confirmed transcript cursors

Design and implement a reconciliation step so a canonical
`session.transcript.blocks` entry created by `appendUserTranscriptBlock`'s
client-side optimistic cursor gets replaced (not merely overlaid-for-render)
by the daemon-confirmed real cursor for the same logical message, once the
real transcript poll observes it. Candidate directions (not prescribed,
needs design discussion): match by turn/position order in the poll
response rather than cursor identity for the first unresolved trailing user
block; have the daemon's cursor resolver treat a `user-sent-...`-shaped
cursor as "the most recent unconfirmed user turn" and resolve it positionally
instead of failing; or extend the real fork request to also carry a
position/turn-index fallback the daemon can use when the primary cursor
doesn't resolve. Whatever direction is chosen, `forkAgentChatFromBubble`
(`App.tsx:5456-5470`) and the daemon's Codex fork cursor resolution
(`codex_projection.rs`'s turn-id-for-cursor path, landed in
`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` Phase 3) both need
to agree on the resolved shape.

**Verification**: repeat this ticket's exact repro (2 real turns, fork from
the first live user-sent bubble) against a real daemon + real `codex`
process; confirm the fork response's `cutCursor` is non-null and that the
forked session's own `/transcript` shows only the blocks up to and including
the cut point, not the full original thread.

## Spec Impact

None yet identified — no existing spec stem documents fork-from-here's
cursor contract at the behavioral level. Contract-first spec: no, this is a
bug-fix; a spec update should follow once Phase 1's chosen reconciliation
shape is implemented.

## Deferral (2026-07-20)

Routed through `ws:lead-discuss` this session. Blocked pending user approval
of the recommended direction — (a) client-side cursor reconciliation
(reconcile optimistic user-block cursors to server-confirmed transcript
cursors on poll-merge); the daemon was confirmed to assign real resolvable
cursors to user blocks too, so direction (a) is viable. Spec-ready but not
implementation-ready until the user picks a direction.

## Deferred to todo (2026-07-21)

Deferred out of the ready queue this round per user curation (agent-chat work
not this round); the existing Deferral/blocker note above remains valid.
