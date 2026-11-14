---
title: "Show provider-exposed reasoning summaries in live dashboard activity"
---

# Show provider-exposed reasoning summaries in live dashboard activity

## Background

The dashboard reports thinking activity but does not carry provider-exposed
reasoning-summary text through to that activity display. Owner approval verbatim:
`670,675=a`. MAIN's approved direction is:

```text
provider reasoning-summary deltas -> adapter -> session state -> dashboard activity display
```

This is accepted backlog, not implementation-ready design. Capture authority is
MAIN's EXEC2 contract, section 675 and latest override (2026-11-14T11:41:17Z),
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/e81ff7f0-01-exec2-approved-tasks-3-4-675.md`.

## Decisions

- Implement the approved live summary pipeline using only provider-exposed
  reasoning **summary** signals. Accessing, reconstructing, inferring, or newly
  generating private hidden chain-of-thought is outside this contract. Opaque
  signatures/encrypted replay data are not summary text.
- MAIN relayed owner answers at 2026-11-14T12:00:19Z, verbatim:
  `710,714=a`, `711,713=b`, and
  `712=a^서브에이전트는 아예 scope 제외입니다. 추가로 대화창 목업도 좀 부탁해요.`
  For this ticket, **710=a** selects the first verified explicit-summary modes,
  with ordinary activity fallback elsewhere; **711=b** requires summaries to be
  retained in completed conversation history, not live-only; **712=a** limits
  the feature to the lead. Subagents are fully outside scope, not a later slice.
  IDs 713 and 714 belong to MAIN's broader answer batch; no additional meaning
  is inferred for this ticket.
- The owner approved the conversation mockup, verbatim **`717=ok`**, relayed by
  MAIN at 2026-11-14T12:07:54Z and 12:10:16Z. The unchanged HTML and both supplied
  screenshots are attached below as the UI basis. The asset approval does not
  select storage fields or API shapes.
- MAIN relayed the owner answer at 2026-11-14T12:25:25Z, verbatim
  **`725,726=a`**. **725=a:** completed provider summaries persist at the existing
  natural turn-history commit boundary. Explicit limit: process death before
  that commit may lose that turn's summary; durable mid-turn retention is not
  required. **726=a:** interrupted/failed turns retain the exact received
  summary prefix, labeled with the actual aborted/failed status (or an actually
  exposed incomplete status), never a fabricated completion. Never synthesize
  missing summary text.
- Intended compatibility: **T0 additive**. Preserve existing protocol/API
  semantics; a mechanism requiring T2, a protocol change, or a minor bump must
  stop and return to MAIN for explicit owner approval, not silently proceed.
- Priority remains 649 / TASK3 / TASK4 / TASK5 / 675. Independent parallel
  worktree capture is authorized; this ordering is not a blanket serial capture
  wait. MAIN owns develop merges and all global question IDs.
- Once decisions and design/readiness gates clear, MAIN's assignment authorizes
  ticket-to-implementation without another owner promotion wait. This capture
  does not run fact population, Sage, promotion, or implementation. Voice remains
  idea-only and is not bundled into this feature.

## Approved UI Reference

Owner approval: `717=ok`. Files copied unchanged from MAIN's
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/shared/mockups/live-reasoning-675/`:

- [Interactive mockup](../assets/261114-feat-ws-live-reasoning-summary/mockup.html)
- [1280-wide reference](../assets/261114-feat-ws-live-reasoning-summary/mockup-1280.png)
- [360-wide reference](../assets/261114-feat-ws-live-reasoning-summary/mockup-360.png)

The approved assets, not inferred height/layout rules, define the visual basis.
Implementation verification must capture the actual conversation UI at both
reference widths and compare the results side by side with these supplied
screenshots: live summary, ordinary unsupported/no-text fallback, and retained
completed summary in collapsed and expanded states. The HTML was read in full
and both screenshots inspected; source/destination SHA-256 hashes and bytewise
comparison matched for all three files. No implementation result exists yet, so
attachment verification is not a claim of implemented visual conformance.

## Evidence and Prior Art

Grounded at repository base `9854ff350`; these are observations, not chosen APIs.

- **Exact installed API evidence:** official Pi coding-agent and pi-ai packages
  under `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/` are
  **1.5.2** (pi-ai is in its `node_modules/@earendil-works/pi-ai/`). Read its
  `docs/extensions.md`, `docs/sdk.md`, `docs/rpc.md`, `docs/json.md`,
  `docs/message-types.md`, `examples/extensions/debug-provider.ts`, and
  `examples/sdk/01-minimal.ts`. The extension's `provider_stream_event` exposes
  `{provider, api, model, data}` before normalization, read-only, notification-only,
  not persisted; handlers are awaited in stream order. The example demonstrates
  observation, not permission to copy whole raw events into this feature.
- **Explicit summary contract:** that installation's official `openai@7.19.0`,
  `node_modules/openai/resources/responses/responses.d.ts`, defines:

  ```typescript
  // ResponseReasoningSummaryTextDeltaEvent
  { type: "response.reasoning_summary_text.delta", delta: string,
    item_id: string, output_index: number, summary_index: number,
    sequence_number: number }
  ```

  The same declarations define summary-part added/done and summary-text done;
  text done carries authoritative `text`, and part done may have
  `status: "incomplete"`. These are SDK contracts, not proof any current model
  request emits them. Installed pi-ai's `dist/api/openai-responses.js` and
  `openai-codex-responses.js` request `summary: "auto"` in their reasoning-effort
  branches; summary availability is not unconditional.
- **Provenance/version gap:** `agents-plugin-pi/package.json` pins development
  Pi packages to **0.84.4**, with wildcard peer ranges. The locally installed
  0.84.4 packages at `/Users/kang-sw/devenv/agents-plugin-pi/node_modules/`
  corroborate that version; pi-ai depends on `openai@6.40.0`. In **both** pi-ai
  versions, `dist/api/openai-responses-shared.js` maps summary-text deltas and
  `response.reasoning_text.delta` to the same `thinking_delta`; summary-part done
  also emits a synthetic separator. Generic `thinking_delta`/`ThinkingContent`
  alone therefore cannot establish summary provenance. The 0.84.4 extension
  declarations lack `provider_stream_event`; 1.5.2 declares it. No SDK upgrade or
  broad provider allowlist is approved here. The existing local official API
  cache `/Users/kang-sw/devenv/ai-docs/.deps/` has no Pi/OpenAI domain; claims above
  use exact installed official declarations/docs and implementation, not guessed
  provider internals or unversioned web prose.
- **Terminal Pi path:** find `onLeadEvent` in
  `agents-plugin-pi/src/dashboard-feed.ts` and `observeGenerating` in
  `src/agent-batches.ts`. Thinking start changes phase; thinking delta text is
  currently ignored. Reply forwarding takes text blocks only. `src/bridge.ts`
  reserves `dashboard.intake` as adapter-only; normal events flow to
  `agents-plugin-tool/internal/mcp/live_snapshot.go`.
- **Hosted Pi differs:** `agents-plugin-pi/src/index.ts` disables normal intake
  for hosted leads. `src/hosted-dashboard-events.ts` currently forwards only
  wakes/task counts through `ws-dashboard-events`; the Go allowlist is
  `internal/wsagent/wake.go:piHostedEvents`. `internal/wsagent/pi.go:mapLine`
  currently uses message updates for timing, and `message` forwards completed
  text/tool calls, not thinking text. Pi 1.5.2 RPC updates are delta-only (no SDK
  cumulative `message`/`partial`); raw provider hooks are not a documented RPC
  session event. Neither hosted nor terminal summary transport exists yet.
- **State/display candidates:** `internal/wslive/state.go` owns adapter events,
  `OptionalKinds`/`accepts` negotiation and unknown-kind refusal;
  `snapshot.go` owns `Activity`, `GeneratingView`, `Lead.Reply` and tolerant
  snapshot reads. There is no dedicated reasoning-summary field.
  `internal/mcp/live_snapshot.go` owns debounced intake snapshots and hosted live
  slots; `internal/wsagent/runner.go` owns runner-fed live state.
  `internal/wsdashboard/pageview.go:conversation` and
  `assets/dashboard.tmpl:r-convlive` render phase/tool activity without summary
  text. Optional capability negotiation and optional display fields are additive
  prior art, **not** a settled new event/store/schema mechanism.
- **Coverage omissions:** current hosted Claude mapping in
  `internal/wsagent/claude.go` consumes whole assistant blocks, without a mapped
  live summary delta path. This inspection establishes no equivalent Claude,
  Codex-harness, or other provider summary capability and makes no parity promise.
  Reasoning-token counts, effort, empty/redacted thinking, captions, and existing
  model-generated tool/display summaries are not substitute summary data.

## Technical Readiness

No open owner questions remain: IDs 710–712, 717, 725 and 726 are answered.
Technical readiness still requires populated facts and design/completeness review;
this accepted todo ticket is not promoted by policy settlement alone.

Derive the exact supported provider/API and terminal/hosted capability matrix,
additive transport, history storage and old/new write-preservation contract.
Preserve summary provenance and the confirmed natural-turn commit/labeled-prefix
semantics. Exact field names, revision/freshness guards, storage API shape and any
new mechanism remain proposals until facts/design review; they are not Decisions.
Do not repurpose reply/caption semantics or select a wire event, store/schema,
SDK upgrade or migration by implication.

## Constraints

Implementation must read `ai-docs/manuals/shipped-surface-boundary.md` and
`release-compatibility.md`; MCP edits also require `ws-mcp.md`. Pi edits must use
exact supported-version official docs/examples and follow relevant event/SDK
cross-references. Do not modify provider requests, reasoning effort, or upstream
SDK versions merely to make data appear without an approved decision.

Respect the 725=a persistence boundary: do not promise durable mid-turn retention
or silently introduce a new checkpoint mechanism to avoid the explicitly accepted
precommit process-loss limit. Respect 726=a: retain only the received provider
summary prefix with its actual terminal status; do not infer or synthesize missing
text. These lifecycle decisions do not approve a particular field/revision/store
API shape. Keep the three 717-approved assets unchanged.

## Phases

### Phase 1: Carry supported lead summaries into live activity and history

After technical fact population and design/readiness gates clear, implement the
agreed bounded lead-only provider-summary path into live activity and retained completed
conversation history, preserving ordinary replies, tools, activity and turn
accounting. Use the first verified explicit-summary modes with ordinary activity
fallback elsewhere. Subagents are excluded entirely. Follow the attached
717-approved UI basis and the settled 725/726 lifecycle contract: persist at the
existing natural turn-history commit boundary, allow the explicit precommit
process-loss limit, and retain received interrupted/failed prefixes with actual
status labels. Keep unsupported summary data absent, never speculative.

Verification must cover ordered/interleaved summary parts, authoritative end,
normal progress and reply/tool transitions, abort/error/retry, no-summary and
unsupported capabilities, empty/redacted blocks, reset/reload/reconnect, and
terminal versus hosted paths in the verified scope. Verify completed summaries
remain in conversation history after live activity clears, reload/reconnect and
writer replacement after the natural turn-history commit. Exercise process death
before that commit and document the permitted loss without claiming mid-turn
durability. Verify interrupted/failed turns retain the exact received prefix and
actual aborted/failed/incomplete label at the natural commit; retries must not
append a later request's text to that prefix or relabel it complete. Negative
tests must prove missing summary text is never synthesized and subagent summaries,
non-summary thinking, signatures/encrypted content and private data never enter the new
snapshot/history/UI path. Pin exact versioned fixtures, exercise old/new
compatibility, redaction/escaping, bounded buffering and nonblocking/debounced
delivery. Run the project's applicable Pi tests (`npm test` in
`agents-plugin-pi`) and Go suite (`go test ./...` in `agents-plugin-tool`), plus
dashboard render/browser evidence
for the approved presentation, including actual 1280/360-width result captures
compared side by side with the attached 717-approved screenshots;
perform no live/provider request merely for capture. Durability/recovery tests
must prove the confirmed retained-history requirement and settled 725=a/726=a
boundaries, not invented storage defaults. During this attachment/capture
step, run no builds, code tests, browser work, provider requests, probes or daemon
actions; EXEC2's verification slot is reserved exclusively for 649's six cases.
