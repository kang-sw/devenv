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

## Open Decision Queue

MAIN assigns IDs and records owner answers; the recommendations below are not
Decisions.

- **technical — Initial capability/version boundary.** Which proven provider/API,
  terminal/hosted modes, and Pi minimum version ship first? Options: (a) restrict
  the first slice to explicitly identifiable summary events on a verified hook
  surface; (b) require broader harness/provider parity first. Recommend (a): the
  0.84.4/1.5.2 provenance gap prevents a safe generic-thinking implementation.
- **technical — Additive transport and state contract.** Choose an evidenced
  optional-field/capability design after tracing both intake and runner paths,
  or defer until a common supported source exists. Recommend reviewing a minimal
  additive proposal first; do not repurpose reply/caption semantics or settle a
  new wire event/schema by implication. Require old/new adapter-daemon-reader
  compatibility evidence and unknown-field write-preservation analysis.
- **policy — Summary lifetime and retention.** Options: (a) live-only content;
  (b) also retain completed/partial summaries in history. Recommend (a) for the
  first slice to minimize persistence exposure, subject to owner confirmation.
  Specify clear/retain behavior at summary end, reply/tool transition, turn end,
  abort, failure/retry, reset/reload and reconnect; neither retention nor a cap
  value is an approved default yet.
- **policy — Activity presentation and absence.** Options: (a) lead activity first;
  (b) lead plus subagent activity. Recommend (a) with an explicit summary label
  and ordinary activity fallback when capability/text is absent. Confirm layout,
  expansion/scroll behavior, partial/error indication, and unsupported-state UI;
  do not fabricate text when a provider exposes none.

## Constraints

Implementation must read `ai-docs/manuals/shipped-surface-boundary.md` and
`release-compatibility.md`; MCP edits also require `ws-mcp.md`. Pi edits must use
exact supported-version official docs/examples and follow relevant event/SDK
cross-references. Do not modify provider requests, reasoning effort, or upstream
SDK versions merely to make data appear without an approved decision.

## Phases

### Phase 1: Carry supported summary signals into live activity

After the decision queue and design/readiness gates clear, implement the agreed
bounded provider-summary path, preserving ordinary replies, tools, activity and
turn accounting. Keep any unsupported capability absent, never speculative.

Verification must cover ordered/interleaved summary parts, authoritative end,
normal progress and reply/tool transitions, abort/error/retry, no-summary and
unsupported capabilities, empty/redacted blocks, reset/reload/reconnect, and
terminal versus hosted paths in the agreed scope. Negative tests must prove
non-summary thinking, signatures/encrypted content and private data never reach
snapshot/ring/UI. Pin exact versioned fixtures, exercise old/new compatibility,
redaction/escaping, bounded buffering and nonblocking/debounced delivery. Run the
project's applicable Pi tests (`npm test` in `agents-plugin-pi`) and Go suite
(`go test ./...` in `agents-plugin-tool`), plus dashboard render/browser evidence
for the agreed presentation; perform no live/provider request merely for capture.
Durability/recovery tests follow the confirmed retention decision, not invented
storage defaults.
