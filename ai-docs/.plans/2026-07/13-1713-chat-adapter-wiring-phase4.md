# Plan: 260713-feat-ws-dashboard-agent-chat-real-adapter-wiring — Phase 4: End-to-end verification, including manual walkthrough

## Relevant Ticket Contract

- Phase 4 text (verbatim intent): "Automated: a real (not fixture-only,
  unless a real binary is unavailable in the dev/CI environment, in which
  case use a real-capture fixture per `260620` Phase 4's Claude CLI fixture
  precedent) E2E test confirming actual send/receive works for both Codex
  and Claude harnesses through the real client and polling path built
  above. Manual: before marking this ticket done, perform a real browser
  walkthrough (real daemon, real harness process, not the stub) of
  send/receive/fork-from-here..."
- Constraints section: "This ticket is not done until real send/receive
  works end-to-end for Codex and Claude"; "If a phase discovers it cannot
  reach a genuinely working state within reasonable scope, flag/escalate
  explicitly rather than landing partial work framed as complete."
- Prior phases already landed: real fetch client
  (`activitySessionClient.ts`, Phase 1), real poll loop
  (`beginRealStreamingTurn`/`blocksSincePolledLength`, Phase 2), real Codex
  fork daemon handler (Phase 3). Phase 4 is verification-only per the
  ticket's own framing — no new production behavior is expected.
- Hard environment constraint (imposed by the calling agent, not the
  ticket): the executing agent is an autonomous coding agent with no
  browser automation tool available. The "Manual: ... perform a real
  browser walkthrough" half of this phase **cannot be executed by this
  agent** and must be escalated, not faked or silently skipped.

## Out of Scope

- The manual real-browser walkthrough itself (send/receive/fork-from-here
  against a live daemon in an actual browser) — no browser tool exists in
  this environment; see Escalations.
- Any new production code. Phases 1-3 are complete per their own Result
  sections; Phase 4 is verification-only per the ticket's own framing.
- OpenCode — out of scope for this entire ticket (stub-only, per Goal
  section).
- Live Claude fork-from-here, rewind/resume-from-here for any harness —
  already ruled out of MVP scope by the Goal section; not re-litigated.
- Building a literal full-stack (browser-driven) automated E2E harness.
  No such infra exists in this repo and building one is disproportionate to
  a verification-only phase; the automated half is scoped to daemon-level
  and client-level tests per Codebase Findings below.
- Real subprocess spawning of the actual `codex`/`claude` binaries inside
  the automated test suite (see Codebase Findings — feasible in this
  sandbox but not a safe/repeatable CI pattern; the repo's own prior phase
  already chose the fixture/scripted-peer route for this reason).

## Codebase Findings

- `codex --version` / `codex login status` and `claude --version` /
  `claude --print "hi"` were run in this sandbox: both binaries are
  installed and **authenticated** (Codex: "Logged in using ChatGPT"; Claude
  answered a real prompt). This means a genuine live-process round trip is
  *possible* here, but is not evidence it is available in a general CI
  environment, and spawning real agentic subprocesses from an automated
  test is slow, non-deterministic, and mutates on-disk session state —
  the same tradeoff the ticket's own precedent already resolved (next
  bullet). Decision: do not build the automated test around a live binary
  spawn; use the same real-connection/scripted-peer pattern already in
  this file.
- `ai-docs/tickets/ready/260620-feat-ws-dashboard-agent-client-activity-sources.md`
  Phase 4 Result (~L982-L1065): the "real (not fixture-only)" bar for that
  phase's *automated* suite was satisfied by projector fixtures
  (`ws-dashboard/crates/core/tests/fixtures/claude-cli-turn.ndjson`)
  **captured once from a real spawned `claude` binary** during a
  verification spike, then replayed deterministically in `cargo test`; two
  genuinely-live-binary tests were written but marked `#[ignore]` ("2
  ignored real-binary smokes" in the Result's Tests line), i.e. opt-in, not
  part of the default automated run. This is the named fixture precedent
  the current ticket phase explicitly permits falling back to; applying
  the same shape here.
- `ws-dashboard/crates/daemon/tests/routes.rs:13963-13991`
  (`spawn_codex_reply_peer`) and `:14360-14417`
  (`spawn_claude_reply_peer`): existing in-process scripted-peer helpers.
  These spin a real `tokio::io::duplex`, hand one end to a real
  `CodexConnection`/`ClaudeConnection` (the same types the real subprocess
  transport uses), and answer JSON-RPC/stream-json lines from a second
  task. This is "real" at the connection/parsing/route layer — only the
  external OS process is substituted — and is the established in-repo
  pattern for driving real HTTP routes without a real binary.
- `ws-dashboard/crates/daemon/tests/routes.rs:13994-14063`
  (`codex_session_prompt_and_transcript_round_trip_local`) and
  `:14419-14488` (`claude_session_prompt_and_transcript_round_trip_local`):
  existing single-shot round-trip tests (one prompt POST, one transcript
  GET) built on the scripted-peer pattern. They do **not** exercise a
  multi-poll sequence with a `live: true -> live: false` transition, which
  is exactly the shape `beginRealStreamingTurn`'s poll loop depends on —
  this is the concrete coverage gap Phase 4 should close at the daemon
  layer.
- `ws-dashboard/crates/daemon/src/codex_app_server.rs:881-902`
  (`codex_activity_transcript`) and the equivalent
  `claude_activity_transcript` in `ws-dashboard/crates/daemon/src/claude_cli.rs`
  (~L882): `live` in the transcript response is
  `projector.is_turn_active()`, read fresh from `session.projector` (an
  `Arc<AsyncMutex<CodexProjector>>` / `Arc<AsyncMutex<ClaudeProjector>>`) on
  every GET. Because `insert_session_for_tests` returns the live
  `Arc<CodexSession>` / `Arc<ClaudeSession>` handle, a test can hold that
  handle and call `session.projector.lock().await.ingest_line(...)`
  **between** polls to simulate the harness pushing more transcript
  content over time, then GET the transcript route again — this drives a
  real multi-poll HTTP sequence (create/seed -> prompt -> poll #1 (live) ->
  ingest more -> poll #2 (still live) -> ingest terminal event -> poll #3
  (live:false)) entirely through real routes, with no new test
  infrastructure needed.
- `ws-dashboard/frontend/src/activitySessionClient.ts:390-475`
  (`beginRealStreamingTurn`, `blocksSincePolledLength` import at L26): the
  frontend poll loop already has dedicated unit-test coverage in
  `ws-dashboard/frontend/src/activitySessionClient.test.ts` (multi-poll,
  overlapping-poll, delta-guard, error-path — see Phase 2's Result) against
  a hand-mocked `globalThis.fetch` (stub installed at `L59-74`). That file
  already proves the frontend polling logic is correct in isolation; the
  remaining gap for "through the real client and polling path" is a
  **wire-shape contract** check — does what the real daemon route actually
  returns match what the frontend polling code expects — not new
  polling-logic coverage.
- No existing harness in this repo boots the real daemon binary and drives
  it with a real browser or even a real Node `fetch` over a real socket
  from the frontend test suite (`activitySessionClient.test.ts` mocks
  `globalThis.fetch` directly). Building such cross-process infra is a new
  capability, not a natural extension of either existing suite, and is
  disproportionate to a verification-only phase — treated as out of scope
  (see Out of Scope).

## Implementation Plan

1. **Daemon-side multi-poll E2E test, Codex** — add
   `codex_session_send_receive_multi_poll_e2e` (or similar name) to
   `ws-dashboard/crates/daemon/tests/routes.rs`, adjacent to
   `codex_session_prompt_and_transcript_round_trip_local`. Reuse
   `spawn_codex_reply_peer` and `insert_session_for_tests`, but keep the
   returned `Arc<CodexSession>` handle. Sequence: POST `/prompt` -> GET
   `/transcript` (assert `live: true`, N blocks) -> lock
   `session.projector` and `ingest_line` a `turn/started` +
   `item/completed` (simulating mid-turn progress) -> GET `/transcript`
   again (assert more blocks, still `live: true`) -> `ingest_line` a
   `turn/completed` -> GET `/transcript` (assert `live: false`, final block
   count). Assert no provider-private fields leak, matching the existing
   round-trip test's forbidden-string check.
2. **Daemon-side multi-poll E2E test, Claude** — mirror step 1 using
   `spawn_claude_reply_peer`/`ClaudeProjector`/`ClaudeSession`, with
   Claude's terminal signal (confirm the exact ingest line shape that
   flips `ClaudeProjector::is_turn_active` to false from
   `ws-dashboard/crates/core/src/claude_projection.rs` before writing
   assertions, and prefer reusing lines already validated in
   `ws-dashboard/crates/core/tests/fixtures/claude-cli-turn.ndjson` over
   hand-writing new ones).
3. **Frontend wire-contract test** — in
   `ws-dashboard/frontend/src/activitySessionClient.test.ts`, add one test
   per harness that drives `beginRealStreamingTurn` against a scripted
   `globalThis.fetch` sequence whose JSON response bodies are copied
   verbatim from what step 1/2's daemon test actually asserts (same field
   names/shape: `activityId`, `status`, `live`, `blocks[].cursor`, etc.),
   confirming `onUpdate` fires with the expected deltas across polls and
   `onComplete` fires once `live` flips false — closing the contract loop
   between the real daemon shape and the real frontend polling code
   without new cross-process infra.
4. Do not touch any production source file in `ws-dashboard/frontend/src`
   or `ws-dashboard/crates/daemon/src` — this phase is test-only, matching
   the ticket's own "verification" framing and Phases 1-3 already being
   complete.
5. Do not attempt real subprocess spawning of `codex`/`claude` inside the
   test suite; do not add real-binary `#[ignore]`-tagged smoke tests either
   unless steps 1-3 leave clear spare scope — they are optional/precedent-
   matching, not required by the phase text.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --test routes` — new Codex/Claude
  multi-poll tests plus full existing suite green.
- `cargo test -p ws-dashboard-core` — unaffected; confirm still green (no
  core changes expected).
- `npm run test:agent-chat-client` (or whichever npm script covers
  `activitySessionClient.test.ts`) — new wire-contract tests plus existing
  suite green.
- `npm run build` — unaffected by test-only changes; run once to confirm no
  incidental type drift.
- Manual browser walkthrough (send/receive/fork-from-here against a real
  daemon and real harness process) is **not executable by this agent** —
  see Escalations. Do not mark this phase or the ticket itself complete on
  the basis of the automated tests alone; the Result note for this phase
  must explicitly record the manual walkthrough as outstanding.

## Escalations

- Confidence: high (automated half only).
- The **manual browser walkthrough is a genuine, unavoidable blocker for
  this ticket's completion** and cannot be satisfied by an autonomous
  coding agent in this environment: there is no browser automation tool
  available, and the ticket text itself is explicit that this exact kind of
  gap ("designed but not connected") was only caught by a human manual pass
  before, and demands the same discipline for verifying its own completion.
  Do not fake, skip silently, or substitute an automated proxy (e.g. an
  HTTP-level script hitting the daemon and calling it equivalent to "a real
  browser walkthrough") for this step. This must be flagged back to the
  human ticket owner: after the automated tests in this plan land, the
  ticket should stay open (or move to a state reflecting "automated
  verification done, manual walkthrough pending") until a human performs
  the real send/receive/fork-from-here pass against a live daemon and a
  real Codex/Claude process, per the ticket's own Constraints section
  ("flag/escalate explicitly rather than landing partial work framed as
  complete").
- Secondary, lower-stakes note for the human owner: both `codex` and
  `claude` CLIs are actually installed and authenticated in this sandbox
  (confirmed via `codex login status` and a live `claude --print` call),
  so a real-process manual check is technically possible from this same
  machine/session if the owner wants to run it interactively — this plan
  does not rely on that fact for the automated tests, but it may make
  scheduling the outstanding manual walkthrough easier.
