---
title: "Wire agent chat UI to real Codex/Claude adapters (MVP-complete)"
related:
  260620-feat-ws-dashboard-agent-client-activity-sources: prerequisite
  260711-feat-ws-dashboard-agent-activity-chat-ui: prerequisite
  260713-fix-ws-dashboard-agent-chat-ui-usability-polish: related
  260713-feat-ws-dashboard-activity-session-fork-cursor: prerequisite
  260720-bug-dashboard-fork-from-here-cutcursor-resolution: blocked-by
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

### Result (40c343b2/0a57dec4/e082bbd2) - 2026-07-13

Implemented on `impl/chat-adapter-wi` (commits `40c343b2` feat, `0a57dec4`
fix cycle 1, `e082bbd2` fix cycle 2; range `efaec81d..e082bbd2`).

Rewrote `activitySessionClient.ts`'s `beginRealStreamingTurn` from Phase 1's
one-shot fetch into a real poll loop against the existing `transcript` GET
endpoint: polls on a fixed interval, stops on the daemon-reported `live:
false` signal (backed by `is_turn_active()` in both `codex_app_server.rs`
and `claude_cli.rs`), and diffs each poll against the previously-seen block
count via a new `blocksSincePolledLength` helper (colocated with
`mergeStreamingTranscriptBlocks` in `agentChatStreamMerge.ts`) so only the
re-included in-progress tail block plus newly appended blocks are handed to
`onUpdate`, not the full transcript every tick. Timer scheduling is
injectable (mirrors `gitToolbar.ts`'s `GitRefreshSchedulerEnvironment`
pattern) so tests can manually tick polls instead of waiting on real
intervals — required since this test suite has no fake-timer framework. No
`App.tsx` change was needed: its call site already merges any block slice
size by cursor. No new SSE/websocket endpoint was added; `auth.rs`'s
`authenticate_websocket_upgrade` seam stays reserved future work per the
ticket's explicit non-goal.

Reviewed (partitioned correctness/fit/test), two fix cycles, all re-reviews
clean:
- First pass: fit clean. Correctness 1 Important — `onComplete`/`onError`
  could fire twice under overlapping in-flight polls (a lagging poll
  resolving after the first's terminal branch had already fired), causing
  double FIFO dequeue/double turn start at the `App.tsx` call site; the
  `stopped` flag was only set by external `stop()`, not by either terminal
  branch. Test 1 Important — no coverage for the `delta.length > 0`
  skip-empty-slice guard.
- Fix cycle 1 (`0a57dec4`): set `stopped = true` in both the completion and
  error terminal branches before invoking callbacks, so a lagging poll's own
  terminal branch early-returns via the pre-existing `if (stopped) return;`
  guard; added a regression test driving two genuinely overlapping in-flight
  polls via a deferred-promise fetch stub (the file's queue-based mock can
  only resolve in call order); added a "no-growth poll" test for the
  `delta.length > 0` guard, plus two optional Minor test-quality fixes
  (tautology-to-oracle replacement, post-`stop()` guard exercise).
- Re-review surfaced a new Critical test finding: the "no-growth" test from
  fix cycle 1 was itself a false positive — it asserted a same-length,
  non-empty re-poll skips `onUpdate`, but `blocksSincePolledLength` always
  re-includes the tail block by design (`start = Math.max(0, lastSeenLength
  - 1)`), so that scenario actually still fires `onUpdate` with a 1-element
  delta; the test only passed because it read its assertion after too few
  microtask flushes, before the mocked fetch chain had actually settled a
  second time.
- Fix cycle 2 (`e082bbd2`): rescoped the test to the guard's real
  reachability condition (an empty-`blocks` first poll, `lastSeenLength ===
  0`), and replaced the fixed microtask-count flush with a macrotask
  `setTimeout` drain. Verified load-bearing by a guard-removal sanity check
  (temporarily disabling the `delta.length > 0` guard made the test fail as
  expected; restoring it made the test pass again) — independently repeated
  by the re-reviewer, not just claimed by the implementer.
- Re-review: all three partitions accepted the fixes as correct and
  complete; clean across correctness/fit/test.

Tests: `npm run test:agent-chat-stream-merge`, `npm run test:agent-chat-client`,
`npm run build`, `npm run test:agent-chat-tabs`, `npm run test:agent-chat-bubbles`
— all green at final commit `e082bbd2`.

Deviations: none structural. The exact poll interval (1500ms) was left to
implementer judgment per the plan.

Not executed (explicitly deferred, matching the ticket's own framing):
the plan's manual "real daemon" verification step (send a message against a
live Codex/Claude session, confirm incremental rendering) — no live daemon
session was available in this sandbox; flagged for a human/manual check
before Phase 4's end-to-end walkthrough.

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

### Result (33bb209a) - 2026-07-13

Landed the Codex fork daemon handler and connected the frontend
follow-through Phase 1 deliberately deferred here.

- `CodexProjector` (`codex_projection.rs`) gained internal turn-id tracking
  (`turn_id_for_cursor`, populated from `turn/started`'s previously-discarded
  `params.turn.id`) plus a pure `project_fork_turns` function and a
  `CodexProjector::seeded` constructor, so a forked session's transcript
  shows correct pre-fork history immediately and continues ingesting live
  notifications right after. Turn ids stay internal-only, never copied into
  the outward `TranscriptBlock` (the existing privacy contract is
  unaffected).
- `CodexAppServerProvider::fork` resolves `cutCursor` to a provider turn id,
  spawns a **dedicated new connection** for the forked thread (mirrors
  `create_session`'s spawn/register pattern; `thread/fork` loads by
  `threadId` from disk so the source connection need not stay alive, and
  this crate's one-connection-per-projector pump can't demux notifications
  by thread), and registers a new `CodexSession` with a fresh `activity_id`.
- `CodexControlRequest` gained a `Fork { cut_cursor: Option<String> }`
  variant wired through `codex_session_control`'s match arm
  (`codex_routes.rs`). Live Claude fork and `goal`/`rewind` variants remain
  unwired, per this phase's explicit scope.
- `forkActivitySession` (`activitySessionClient.ts`) now reads the real
  `data.activityId`/`data.cutCursor` from the daemon response instead of
  echoing the request; a new `hydrateForkedAgentChatSession` helper fetches
  the forked session's transcript so `forkAgentChatFromBubble`'s Codex
  branch applies the new pane instead of always throwing.
- Fix cycle (review-cycle 1, correctness, Important, `33bb209a`): the
  daemon's `fork()` originally echoed back the caller's raw `cutCursor`
  even when turn-id resolution failed and the fork silently fell back to
  the whole thread, misleadingly implying the requested cut was honored.
  Fixed to return the actually-resolved turn id (`None` on resolution
  failure) instead; a matching latent bug in the frontend's fallback logic
  (which would have re-substituted the stale request cursor for a
  legitimate `null`, undoing the backend fix at the client boundary) was
  fixed in the same commit. Re-review confirmed clean; fit review was clean
  on the first pass.
- Deviations (both correctness fixes needed to satisfy the plan's own
  contract, not scope creep): (1) a latent serde bug where
  `#[serde(rename_all = "camelCase")]` on the enum doesn't rename fields
  inside struct variants, requiring an explicit `#[serde(rename =
  "cutCursor")]` on `Fork`'s field — verified via an isolated serde 1.0.228
  repro; (2) `CodexProjector::seeded` as necessary glue not explicitly named
  in the plan text, since `project_fork_turns`'s output needs a way to
  pre-populate the projector's internal state.
- Verification: `cargo test -p ws-dashboard-core codex_projection` (12
  passed), `cargo test -p ws-dashboard-daemon` (all unit/route/server tests
  passed), `npm run build`, `npm run test:agent-chat-client` — all green.
  The full spawn-a-real-process-and-fork path and the manual
  browser-walkthrough verification stay deferred to Phase 4, consistent
  with `create_session`'s own pre-existing untested-spawn precedent and
  this ticket's own Phase 4 framing.
- `260713-feat-ws-dashboard-activity-session-fork-cursor` (idea ticket):
  this phase lands the daemon-side fork handler that ticket's Phase 1
  progress note said was still missing; that idea ticket can now be closed
  once Phase 4's manual walkthrough confirms end-to-end fork behavior.

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

### Result (e1726e80) - 2026-07-13 — automated half only, manual walkthrough outstanding

Landed the automated half of this phase's verification bar. The manual
walkthrough this phase's own text requires is **not done** — see below.

- Added `codex_session_send_receive_multi_poll_e2e` and
  `claude_session_send_receive_multi_poll_e2e`
  (`crates/daemon/tests/routes.rs`), each driving a real
  `live: true -> live: false` three-poll HTTP sequence through the
  `Arc<CodexSession>`/`Arc<ClaudeSession>` handle `insert_session_for_tests`
  returns, ingesting projector lines between polls (mirrors the existing
  single-shot round-trip tests' scripted-peer pattern, per `260620` Phase
  4's fixture precedent — no real subprocess spawn).
- Added matching frontend wire-contract tests
  (`activitySessionClient.test.ts`) driving `beginRealStreamingTurn` against
  `fetch` responses shaped exactly like what the two daemon tests assert,
  closing the loop between the real daemon response shape and the real
  frontend polling code.
- Test-only: no production source file touched, per the plan's explicit
  scope (Phases 1-3 already complete; this phase is verification-only).
- Review: correctness, fit, and test partitions all clean on the first
  pass — no fix cycle needed.
- Verification: `cargo test -p ws-dashboard-daemon --test routes` (158
  passed), `cargo test -p ws-dashboard-core` (34 passed, unaffected),
  `npm run test:agent-chat-client`, `npm run build` — all green.
- Deviations: (1) the Codex test pre-seeds the projector with a bare
  `turn/started` before session insertion, since `insert_session_for_tests`
  needs a pre-built projector and the scripted reply peer only answers RPC
  requests, not notifications — there is no other way to get poll #1 into a
  live state without new test infra, which the plan explicitly ruled out;
  (2) frontend delta-size assertions had to be derived from
  `blocksSincePolledLength`'s actual formula rather than assumed 1:1
  correspondence with newly-added blocks (an initial draft under-counted
  the re-included tail block).
- **Outstanding, genuine blocker (not resolved by this commit)**: this
  phase's own text requires "a real browser walkthrough (real daemon, real
  harness process, not the stub) of send/receive/fork-from-here" before the
  ticket is done. The executing agent has no browser automation tool
  available in this environment and cannot perform this walkthrough — per
  the plan's Escalations, this was not faked, silently skipped, or
  substituted with an automated proxy. `codex`/`claude` CLIs are confirmed
  installed and authenticated in this sandbox, so a human owner could run
  this walkthrough interactively from the same machine if convenient. This
  ticket **stays in `ready/`**, not moved to `.done/`, until a human
  completes that pass; the Constraints section's "not done until real
  send/receive works end-to-end... designed but not connected is the
  specific failure mode this ticket exists to correct" bar is explicitly
  not yet met.

#### Edition (manual-pass) - 2026-07-20

The manual walkthrough this phase required was finally performed today
against real (non-stub, non-fixture) processes: a real daemon, a real
`codex` CLI process, and a real `claude` CLI process — not the stub, not a
scripted-peer test double.

- **Real send/receive: CONFIRMED working end-to-end for both harnesses.**
  Verified via direct network/daemon inspection (not just visual/UI
  observation): real `activityId`s were returned on session create for both
  Codex and Claude; the transcript poll showed genuine `live: true ->
  live: false` transitions matching each harness's actual turn completion,
  not a synthetic ticker; and the returned transcript blocks were verbatim
  model replies carrying real `turnId`s from the underlying process, not
  stub/fixture text. This closes the "real send/receive" half of this
  phase's verification bar for both Codex and Claude.
- **Fork-from-here: CONFIRMED BROKEN for real live sessions.** Repro: 2 real
  turns in a Codex agent-chat session, then "Fork from here" on the first
  user message bubble. Request sent:
  `{"action":"fork","cutCursor":"user-sent-mrsxjeyh-1"}` — a client-side
  optimistic-bubble id, not a real transcript cursor (real cursors are
  plain sequential strings like `"0"`/`"1"`/`"2"`/`"3"`). Response:
  `{"applied":true,"data":{"activityId":"codex:...","cutCursor":null}}` —
  `cutCursor: null` shows the daemon's cursor resolution silently failed
  instead of erroring. Confirmed via a direct `curl` against the forked
  session's own `/transcript` that all 4 blocks (both turns' Q+A) are
  present — the fork did not truncate at all; it silently forked the whole
  original thread. Filed as a new bug ticket,
  `260720-bug-dashboard-fork-from-here-cutcursor-resolution` (`todo/`,
  now in this ticket's `related:` frontmatter as `blocked-by`), which also
  traces the client-side mechanism (an optimistic user-bubble cursor that
  is never reconciled with the daemon-confirmed real cursor once the poll
  resolves it) in more depth than a bare lead — see that ticket's
  Investigation section for exact line citations. Fixing it is out of scope
  for that ticket and for this Edition: it is design-level client/server
  cursor-reconciliation work, not a one-line patch.
- **Consequently, this ticket still cannot move to `.done/`.** Phase 4's
  verification bar covers send/receive **and** fork-from-here; the former is
  now genuinely met, the latter is not. This ticket stays in `ready/` until
  `260720-bug-dashboard-fork-from-here-cutcursor-resolution` is fixed and a
  fork-from-here repro against a real live session is re-run clean.
- Note for future readers: the separately-tracked, unrelated
  `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` (CSS/DOM
  visibility bug hiding the transcript element in the Playwright e2e
  environment) did not interfere with any of the verification above — all
  claims here were confirmed via direct network/daemon API inspection
  (request/response bodies, transcript polling state), not via visual
  on-screen checks, so that bug's environment-level rendering issue is
  orthogonal to this walkthrough's findings.

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

## Deferral (2026-07-20)

Blocked-by `260720-bug-dashboard-fork-from-here-cutcursor-resolution`
(unresolved — awaiting user design-direction), already recorded in this
ticket's `related:` frontmatter. Phases 1-3 landed; Phase 4's remaining
fork-from-here confirmation is gated on that bug's fix.

## Deferred to todo (2026-07-21)

Deferred out of the ready queue this round per user curation (agent-chat work
not this round); the existing Deferral/blocker note above remains valid.

## Disposition

2026-07-22: Demoted to idea/. The dashboard agent-dogfooding track (agent
activity source, agent-chat real-adapter wiring, and related acceptance) is
deprioritized in favor of completing the dashboard/terminal usability track
first. Rationale: once terminal usability reaches 100%, swapping the
underlying CLI harness is a viable alternative to native agent surfacing, so
finishing the dashboard is the higher-value path now. Prior shipped work
stands (see phase Results above); only unfinished work is parked. Re-promote
when the dashboard/terminal track is complete and agent dogfooding resumes.
