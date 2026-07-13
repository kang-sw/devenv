---
title: "Wire agent chat UI to real Codex/Claude adapters (MVP-complete)"
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: prerequisite
  260711-feat-ws-dashboard-agent-activity-chat-ui: prerequisite
  260713-fix-ws-dashboard-agent-chat-ui-usability-polish: related
  260713-feat-ws-dashboard-activity-session-fork-cursor: prerequisite
related-mental-model:
  - ws-dashboard-agent-harness
sage-review-design: completed
sage-review-completeness: completed
---

# Wire agent chat UI to real Codex/Claude adapters (MVP-complete)

## Background

`260620` built real Codex (Phase 2) and Claude CLI (Phase 4) daemon-side
adapters — real process spawn, real routes (`codex_routes.rs`,
`claude_routes.rs`), registered in `router.rs:388-408`. `260711` built the
frontend chat UI (bubbles, streaming render, send/queue/fork/resume-scaffold)
entirely against a local stub (`activitySessionStub.ts`) matching only
`260620` Phase 1's *inert* TS type draft (`activitySessionApi.ts`, explicitly
documented at its top as "no fetch helper, no route registration, no handler
— illustrative shapes only").

**Nobody ever chartered the step that connects these two, in either ticket or
a third one.** This was not sloppiness on either side — `260620` Phase 1
never claimed to define a wire-level contract, Phase 2/4 correctly built real
routes against Rust/daemon conventions independently, and `260711` correctly
used the stub for the parallel-buildable UI work per its own stated
dependency. But the result is: today, the chat UI cannot talk to a real
harness at all, and this gap was discovered only via a post-hoc manual
browser walkthrough, not planned for. **This ticket exists specifically to
close that planning gap and must not repeat it**: every phase below must
land in a genuinely working, end-to-end state before being marked done —
partial/"designed but not connected" completion is the exact failure mode
this ticket is correcting for.

Confirmed via investigation (this ticket's own pre-work) that this integration
is purely **additive** — no rework of already-shipped code on either side:
- Frontend: exactly 6 named stub functions, called only from `App.tsx` (12
  call sites) and their own tests. `activitySessionStub.ts`'s header already
  states its shapes are kept conformant to `activitySessionApi.ts`
  "so a later real handler can replace call sites here without reshaping
  callers" — a deliberate seam.
- Backend: `router.rs` route registration is a flat, order-independent
  append list. `CodexControlRequest` is matched exhaustively in exactly one
  place (`codex_routes.rs:177-191`); new variants only touch that one arm.
- Streaming: the daemon already receives incremental per-event data
  internally (`CodexConnection`/`ClaudeConnection` in `codex_app_server.rs`
  and `claude_cli.rs` deliver events via unbounded mpsc channels, not
  batched) — only the HTTP-level exposure is missing.

## Goal (MVP scope)

Real, working send/receive chat for the **Codex** and **Claude** harnesses
only (matching `260620`'s completed adapters). OpenCode stays on the stub
until `260620` Phase 3 unblocks (external dependency, out of scope here).
"MVP-complete" means: a user can open a real Codex or Claude session in the
chat UI and send a message and see the real response stream in, against the
actual harness process, not the stub. **Fork-from-here against a real
process is MVP scope for Codex only** (`thread/fork` is Passthrough-tier per
the harness capability matrix). Claude's only fork path is Hack-tier
(transcript-truncation workaround, same tier as rewind) — per
`ai-docs/mental-model/ws-dashboard-agent-harness.md`'s Extension Points,
Hack-tier capabilities do not get folded into normal adapter phases; they
need a dedicated ticket with explicit experimental/unsupported UI labeling
and owner sign-off. Real Claude fork-from-here is therefore **out of scope
here** — leave Claude's fork-from-here scaffolded-disabled (same pattern as
`260711`'s resume-from-here gate) until a separate ticket charters it. Design
polish beyond MVP (rewind/resume-from-here for any harness) stays
scaffolded-disabled per `260711`'s existing capability-gate precedent unless
a future phase explicitly re-scopes it.

## Phases

### Phase 1: Real fetch client, swap stub call sites

Write a real fetch client for the REST-nested paths the existing backend
routes actually use (e.g.
`/api/dashboard/servers/{serverRoute}/work-roots/{workRootId}/activity/{codex-sessions|claude-sessions}/{activityId}/...`
per `codex_routes.rs`/`claude_routes.rs`), matching create/prompt/control/
transcript operations. Swap the 6 stub call sites in `App.tsx` to call the
real client behind matching function signatures (per the seam
`activitySessionStub.ts` already documents). Harness selection must route
Codex/Claude sessions to the real client and leave any other harness
(OpenCode, or anything without a real adapter) on the stub — do not regress
already-working stub-backed flows for unadapted harnesses. For the
fork-from-here request shape specifically, read
`260713-feat-ws-dashboard-activity-session-fork-cursor` first — it already
specifies the cursor/turn-cut-point field `ActivitySessionForkRequest` needs;
build the real client's fork request against that shape rather than
reinventing it, and use it to also close that idea ticket's Phase 1 (the
real fork route is exactly the trigger condition that ticket was waiting
for).

**Verification**: typecheck/build passes; existing frontend unit tests
(`agentChatSessions.test.ts`, `activitySessionStub.test.ts` equivalents for
the real client) pass against the new client; a targeted test confirms each
swapped call site issues the correct REST-nested request shape for a Codex
and a Claude session.

### Result (53d420fe/089feb8e) - 2026-07-13

Implemented on `impl/chat-adapter-wi` (commits `53d420fe` feat, `089feb8e`
fix cycle; range `758c9a50..089feb8e`).

New `ws-dashboard/frontend/src/activitySessionClient.ts` mirrors
`gitToolbar.ts`'s fetch-client idiom against the real Codex/Claude REST-nested
routes, covering create/prompt/control/transcript for both local and
`/servers/{serverRoute}/...`-scoped variants. All 7 actual `App.tsx` call
sites (not 6 — several stub functions had more than one call site) plus a
previously-missing prompt-send call site were routed to the real client for
Codex/Claude sessions, with OpenCode left on the unmodified stub.
`ActivitySessionForkRequest`/`Response` gained an optional `cutCursor` field
per `260713-feat-ws-dashboard-activity-session-fork-cursor`'s guidance
(adopting the stub's `cutBlocks` reference shape as a single wire-shaped
cursor); this covers only the frontend-type half of that idea ticket's
Phase 1 — the daemon-side fork handler it also requires does not exist yet
and lands with this ticket's own Phase 3, so that idea ticket stays open
until then. Real fork
POSTs against the future `/control` Fork action and is expected to fail
(422) until Phase 3 adds the `CodexControlRequest::Fork` variant — this
matches Phase 1's stated verification bar of correct request shape, not
end-to-end success.

Reviewed (partitioned correctness/fit/test), one fix cycle, all re-reviews
clean:
- First pass: fit clean. Correctness 2 Important — `loadAgentChatHistory`'s
  `Promise.all([real, stub])` let a real-route failure drop the OpenCode
  stub-backed history entries too, regressing an unadapted harness's working
  flow; and swapping the previously-inert stub steer to a real
  `turn/steer` call while leaving the FIFO mid-turn resend unchanged caused
  a steered Codex message to be delivered twice. Plus 1 Minor: real fork
  routed any real harness (including Claude) to the hardcoded Codex
  `/control` URL, though unreachable today since `capabilities.fork` is
  `false` for Claude. Test 2 Important — missing server-scoped `serverRoute`
  variant assertions for `resumeAgentChatSession`/`activityHistoryList`, and
  no error-path coverage for `beginRealStreamingTurn`'s `onError` branch.
- Fix cycle (`089feb8e`): isolated the real-list fetch failure with
  `.catch(() => null)` so OpenCode history degrades to stub-only entries
  instead of failing outright; added an `alreadyDelivered` option to
  `beginSimulatedTurn` so a FIFO-dequeued, already-steered Codex message is
  not re-sent via `/prompt`; gated the real fork path to
  `realHarness === "codex"` only, matching the steer branch, and fixed the
  adjacent comment; added the missing server-scoped test variants and
  `beginRealStreamingTurn` error-path coverage.
- Re-review: all three partitions accepted the fixes as correct and
  complete; clean across correctness/fit/test.

Tests: `npm run build` (tsc -b + vite build), `test:agent-chat-client`,
`test:agent-chat-tabs`, `test:agent-chat-capabilities`,
`test:agent-chat-bubbles`, `test:agent-chat-stream-merge`,
`test:work-root-activity` — all green at final commit `089feb8e`.

Deviation from plan: `beginRealStreamingTurn` does not itself send the
prompt (its own docstring language implied it would) — the actual prompt
POST fires once from `sendAgentChatMessage`, to avoid double-posting the
same turn's prompt across both call paths; documented in-code.

Deferred, not fixed (test Minor, disposition marked optional): `sendAgentChatPrompt`'s
test varies harness and `serverRoute` together rather than holding one axis
fixed, so no single assertion isolates e.g. "claude + server-scoped"
specifically — each individual branch is still covered at least once across
the two calls, non-blocking.

### Phase 2: Streaming delivery via polling (MVP), not SSE

Implement a frontend polling loop against the existing polling-only
`transcript` GET endpoint, mirroring the established interval pattern in
`gitToolbar.ts:244` (this codebase already treats polling as an accepted
transport, not a hack — see `workRootActivity.ts:154`'s `"pollFallback"`
transport mode). Replace the stub's synthetic `onComplete` ticker with a
real diff-against-last-seen-length poll, and adapt
`mergeStreamingTranscriptBlocks`'s delta-merge assumptions to fit
full-refetch-diffed transcripts rather than true incremental deltas. Do
**not** build a new SSE/websocket endpoint in this ticket — `auth.rs:172-180`
already reserves a `authenticate_websocket_upgrade` seam for that as
explicitly-future work, and the daemon's per-event mpsc channels mean that
upgrade is additive infrastructure later, not a rework of the polling path
built here.

**Verification**: a unit test on the adapted merge/diff logic (poll-diff
against last-seen transcript length) covering unmatched-append-ordering and
multi-poll stability, mirroring `agentChatStreamMerge.test.ts`'s existing
coverage style; manual check that a real send against a stub-free session
renders incrementally rather than only on full completion.

### Phase 3: Missing capability control variants (scoped by harness tiering)

Add the capability control variants needed for real MVP interactions
(at minimum: whatever `CodexControlRequest` needs beyond today's
`Compact`/`Steer`/`Skills` to support fork-from-here against a real Codex
session — this is the only real-fork wiring in MVP scope, see Goal section)
to `codex_routes.rs` (~line 88-93 enum, ~177-191 match). Do not add a live
Claude fork control variant in this phase (Hack-tier, explicitly out of
scope per the Goal section). Cross-check every candidate variant against
`ai-docs/mental-model/ws-dashboard-agent-harness.md`'s per-harness capability
tiering before wiring it live: only wire a capability the tiering confirms
the real harness genuinely supports (e.g. Codex `thread/fork` is real;
rewind is Claude's unofficial-hack tier and no harness qualifies for
point-based resume today per `260711` Phase 3's existing finding). Do not
wire a control variant just because the frontend type draft has a matching
field — follow the same scaffolded-disabled precedent `260711` Phase 3 set
for resume-from-here if a candidate capability doesn't clear the tiering bar.

**Verification**: a Rust unit/route test on the new `CodexControlRequest`
match arm(s) confirming the added variant(s) dispatch correctly; manual
confirmation that fork-from-here against a real Codex session produces a
new session with the correct transcript cut point.

### Phase 4: End-to-end verification, including manual walkthrough

Automated: a real (not fixture-only, unless a real binary is unavailable in
the dev/CI environment, in which case use a real-capture fixture per
`260620` Phase 4's Claude CLI fixture precedent) E2E test confirming actual
send/receive works for both Codex and Claude harnesses through the real
client and polling path built above. Manual: before marking this ticket
done, perform a real browser walkthrough (real daemon, real harness process,
not the stub) of send/receive/fork-from-here — this ticket was itself
discovered through exactly this kind of manual pass catching what automated
assertions missed, and the same discipline applies to verifying this
ticket's own completion.

## Constraints

- This ticket is not done until real send/receive works end-to-end for
  Codex and Claude — "designed but not connected" is the specific failure
  mode this ticket exists to correct, not an acceptable stopping point for
  any phase.
- If a phase discovers it cannot reach a genuinely working state within
  reasonable scope, flag/escalate explicitly rather than landing partial
  work framed as complete.
- OpenCode wiring is out of scope until `260620` Phase 3 unblocks.
- Any capability-tiering question already resolved by `260711` Phase 3 or
  the harness mental model should not be re-litigated here — apply it.
