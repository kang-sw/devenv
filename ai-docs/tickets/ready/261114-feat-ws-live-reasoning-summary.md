---
title: "Show provider-exposed reasoning summaries in live dashboard activity"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7cdd116388c6e855
sage-review-completeness-reviewed: 7cdd116388c6e855
---

# Show provider-exposed reasoning summaries in live dashboard activity

## Background

The dashboard reports thinking activity but does not carry provider-exposed
reasoning-summary text through to that activity display. Owner approval verbatim:
`670,675=a`. MAIN's approved direction is:

```text
provider reasoning-summary deltas -> adapter -> session state -> dashboard activity display
```

This is accepted backlog with a lead-settled technical contract, pending
Sage design/completeness review rather than an additional owner wait. Capture
authority is MAIN's EXEC2 contract, section 675 and latest override
(2026-11-14T11:41:17Z),
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
- Compatibility tier: **T0 additive**, redesigned under MAIN's 2026-11-15
  04:44:47 disposition; no T2 behavior or new owner question is selected.
  The new native Pi bundle explicitly advertises the optional hosted status
  field `capabilities: ["reasoning_summary_v1"]`, only on supported host
  VERSION 1.5.2. This process-local opt-in, never the presence of
  summary text, gates hosted summaries and new aborted/error/incomplete reasons.
  Old bundles omit the field; unsupported new bundles send an empty list.
  They retain exactly the old hosted reason values and answer-check suppression.
  The hello/protocol version, event kinds, tools and legacy terminal path stay
  unchanged. If this additive gating cannot carry the change, stop rather than
  introduce breaking semantics.
- Build-tool provenance: the repair regenerated the bundle with an existing
  read-only cached esbuild **0.28.1**, because the bounded cached lookup did not
  locate pinned 0.28.2 under the no-install boundary. Runtime/SDK pins, lockfile,
  target and bundling script are unchanged. Canonical input freshness, shipped
  gate assertions and syntax checks passed; this is **not** a pinned-builder
  pass or a claim of bit-identical output from esbuild 0.28.2.
- The lead selected the minimal receiver-owned contract below to resolve the
  three fact-population technical gaps: exact eligible summary sources,
  identity/freshness ordering, and immutable natural-turn persistence. Source
  provenance/version/bounds come from the formed contract; receiver-owned RAM
  retry replaces the rejected client persistence handshake. This is technical
  settlement under existing owner answers, not a new owner question or ID.
- Live data uses an optional stage-free `agent.reasoningSummary` projection;
  terminal final data accompanies the existing real `turn_end`. Hosted final
  projection precedes native settlement, which alone ends the turn. Existing
  natural `RingTurn` receives optional ordered `summaryHistory`; the receiver
  freezes/retries that real record in RAM and commits at file rename.
- The lead auto-confirmed both final design-review corrections without a new
  owner question/ID: explicit positive safe `attempt` identity within each
  request, and a first-v1 mutation barrier over all earlier canonical intake
  operations and their write side effects. Implicit response-ID-only attempt
  keys cannot survive projection/history; starting FIFO only at v1 admission
  cannot fence an earlier locally abandoned legacy handler or scheduled write.
- Retain T0 through separately negotiated optional fields, unchanged legacy
  paths and immutable old-record preservation, not by assuming any optional
  field is automatically compatible. No new event kind, schema/protocol version
  or MCP tool is selected. No client receipt stage, response acknowledgement
  line, extra source closed-turn queue or saved badge is part of this contract.
- MAIN's same disposition permits recording the known invisible idle-reconnect
  gap rather than expanding transport in this fix round. The launcher can
  replace its daemon socket without an in-flight error or a client-visible
  identity change; terminal summary acceptance can then survive until the next
  visible response/error. Initial negotiation and observed loss remain gated,
  but unconditional revocation before the next send across that hidden boundary
  is **not certified**. Existing old receivers ignore optional fields; this is
  not permission to change hello/protocol semantics or claim a runtime pairing
  was tested. The existing idle-reconnect follow-up remains outside this ticket.
- Priority remains 649 / TASK3 / TASK4 / TASK5 / 675. Independent parallel
  worktree capture is authorized; this ordering is not a blanket serial capture
  wait. MAIN owns develop merges and all global question IDs.
- Once decisions and design/readiness gates clear, MAIN's assignment authorizes
  ticket-to-implementation without another owner promotion wait. This capture
  does not run fact population, Sage, promotion, or implementation. Voice remains
  idea-only and is not bundled into this feature.

## Selected Technical Contract

### Source and eligibility

- Enable only the host-provided coding-agent `VERSION === "1.5.2"`, native ws
  extension, lead context, and built-in `openai/openai-responses` SSE or
  `openai-codex/openai-codex-responses` SSE/WebSocket. Hosted default/isolated
  modes qualify only with that extension loaded; MCP-only fallback, 0.84.4,
  unverified versions/providers and subagents retain ordinary activity.
- Read the host's existing exported `VERSION`; use local structural optional
  event types, not new SDK imports/upgrades or executable probes. At each
  request reject extension provider overrides when the public registry's
  `getRegisteredProviderConfig` or `getRegisteredNativeProvider` returns a
  registration; missing/throwing checks fail closed. Eligibility proves neither
  actual emission nor vendor internals: display only correlated observed text.
- Observe `provider_stream_event` without mutation or awaited dashboard I/O.
  Accept only `response.reasoning_summary_part.added/done` with `summary_text`
  and `response.reasoning_summary_text.delta/done` as text. Copy only response
  ID from `response.created`, reasoning item ID/output index from
  `response.output_item.added`, and actual terminal status from response/message
  events. Never copy whole raw events, output arrays, request/error bodies,
  generic thinking, `reasoning_text.delta`, signatures or encrypted content.
- Bind valid item IDs to the active response/provider/API/model; require safe
  nonnegative indexes/sequence numbers. Append deltas, replace the same part at
  authoritative text/part done, and sort by `(output_index, summary_index)`.
  Duplicate/decreasing sequence numbers are ignored within a response only.
  Unknown overlap or unbound/stale data fails closed, never generic backfill.

### Identity, projection and admission

A producer is a fresh UUID per extension `session_start`, retired at shutdown.
Its increasing outer-turn counter follows the existing ws `agent_start` to final
`agent_settled` span, not each Pi request/`turn_end`. Each main-agent
`before_provider_request` starts a distinct positive safe request counter without
inspecting its payload. Within that request reserve a positive safe `attempt`
counter starting at 1. Advance it only after an observed failed attempt followed
by a newly bound response; a repeated old response or unexplained overlap cannot
advance it. The request counter does not advance merely because transport retries
or a continuation response changes without another request hook. Response/item
IDs stay internal correlation keys, never substitute for serialized counters.

Only a bound response can produce an attempt record; an attempt that never binds
a response creates no invented history. A failed reservation may advance at the
next valid response without manufacturing a record for the response-less attempt.
A new request-hook invocation advances request and resets its attempt counter.
Part replacement/deduplication is scoped to `(request, attempt, output, part)`;
raw sequence numbers order only their bound response/attempt. Reject unexplained
overlap, retired responses/items and unsafe/wrapped counters rather than join
attempts. Raw callbacks may precede consumed `message_start`, so that event is
not the identity boundary. Built-in serial correlation and hook-once-before-
transport-retry behavior need the exact offline fixtures below.

The optional v1 projection has no lifecycle stages:

```text
reasoningSummary: {v: 1, producer, turn, revision,
  activeRequest: {request, attempt} | null, truncated,
  requests: [{request, attempt, provider, api, model, status, truncated,
    parts: [{output, part, text, observedBytes, retainedBytes, truncated, status}]}]}
```

It replaces the whole bounded current-turn collection. Omission of the entire
optional projection means no update. Its `activeRequest` always names an included
`{request, attempt}` pair or is `null`; omitting the referenced attempt record
clears selection to `null`, never leaves a stale request-only reference. Clearing
live selection does not erase collected history. Each `requests` array element
is one bound attempt, ordered by `(request, attempt)`, not a logical request
containing an unbounded nested retry list. Status is an actual `streaming`,
`complete`, `aborted`, `failed` or `incomplete` fact; local
truncation is separate. Invalid fields cannot clear unrelated activity. Store
live data on the lead, not shared subagent Activity; reject any `agentId`.

Revisions are increasing positive safe integers per producer, never reset by
feed reset/resync or retry. Equal revision/content is an idempotent replay;
conflicting equal or older revisions cannot overwrite newer data. Preserve
closed-turn/retired-producer watermarks across ordinary reset, and disable stale
source closures. A UUID or higher revision grants no session authority.

- Gate both live/final fields on terminal intake's successful
  `accepts: reasoning_summary_v1`; off/error/observed reconnect revokes acceptance.
  Invisible idle reconnect has the explicitly recorded limitation in Decisions.
  The field capability is advertised only with complete receiver ownership,
  framing/admission and commit support; unsupported standalone/file-only intake
  remains legacy. Old adapters omit fields; old daemons receive none under
  initial/observed negotiation, subject to the recorded hidden-reconnect gap.
- From connection open, track canonical intake operations at ingress, including
  pre-v1 legacy calls and the completion of their write side effects. This
  lightweight tracking does not change legacy scheduling or unrelated tools.
  At ingress enqueue the first syntactically v1 projection and every subsequent
  canonical intake call on that connection, including reset/start/end, before
  asynchronous handler execution; validation still decides acceptance.
- Before the first v1 mutation, establish a barrier: all earlier canonical
  operations and their scheduled/in-progress snapshot/ring write side effects
  must actually quiesce. A handler response alone is insufficient if deferred
  writes can still land. Local abandon, timeout or cancellation cannot mark
  unfinished work quiescent or bypass the barrier. Drain/fence earlier callbacks
  so they cannot reset/write newer state after admission. If quiescence cannot
  be established within a bounded receiver admission budget, refuse/revoke
  optional admission rather than mutate or wait indefinitely. Subsequent
  canonical calls stay ingress-ordered under FIFO; cancellation skips a queued
  mutation only when no mutation/write remains possible. This is connection-
  scoped tracking/barrier/FIFO, not a global queue, new protocol or changed
  unrelated-tool/legacy scheduling. Send order or mutex races are not proof.
- Fence optional admission and real-end candidate acceptance with the daemon's
  actual connection/slot generation and current harness/session ownership,
  including terminal-to-hosted takeover. Publish suppression alone is not a
  ring-write guard. Resync an actually open turn under its same real identity;
  never fabricate start/end or repeat tool accounting to recover a closed turn.
- Terminal final bounded data is an optional `reasoningSummary` field on the
  existing real `turn_end`, validated/consumed atomically with its normal reducer
  application. It cannot end a turn without its real start or alter counts,
  duration, origin or existing reason. Only the ordinary feed's existing
  uncertain-delivery requeue remains; there is no new disk-ack source queue.
- Hosted mode keeps intake disabled. The new bundle adds the optional
  `capabilities: ["reasoning_summary_v1"]` declaration to its existing `status`
  widget, renewed at real turn boundaries before final projection/native
  settlement. Only that explicit process-local opt-in enables hosted summaries
  and new natural-end reasons; omission/empty support preserves legacy reasons
  and answer-check behavior. Old receivers ignore the additive field. Send only
  the validated lead projection through string-array `ws-dashboard-events`;
  construct a fresh internal event
  accepting no reset/turn/batches/usage/key authority. Coalesce progress and
  synchronously flush the final bounded projection in the extension's settled
  handler before the native settled record. The runner attaches it internally
  to its real end application; a widget is never an end command or delivery ack.
  Preserve holder/process identity, sequence and `Replay/LineEnd/HolderSeq`;
  quiet replay is non-writing and wrapped records do not authorize guessed text.

### Actual byte bounds and prefix qualification

Bound a turn to 8 KiB aggregate raw text, **16 total bound-attempt records** in
`requests` across all request/attempt pairs, 64 parts and 128-byte IDs, with
existing provider/API/model display bounds. This is not 16 logical requests each
with an additional retry allowance. At flush retain exact UTF-8-safe prefixes
and terminal facts within a 24-KiB serialized envelope; omitted records/text set
aggregate/per-part truncation. An omitted active attempt clears `activeRequest`
to `null`, never a dangling or request-only replacement. Record actual
observed/retained UTF-8 bytes with safe counters; authoritative replacement
resets the observed text length. Do not insert ellipsis into provider text or
reinterpret local clipping as a provider abort/incomplete status.

The hard bound is **49,152 bytes for the actual complete serialized UTF-8 JSONL
record including LF**, not character counts or raw-text size. Terminal checks
occur after actual RPC ID allocation/stringification and include tool name,
session/adapter arguments, both counters, the active pair and every other
event/envelope field; reject before writing any bytes and cancel the unused
pending ID. Counter/attempt metadata consumes the same envelope/frame budget;
it never adds a per-request retry allowance or raises either byte bound. Hosted
checks use one projection line and the verified 1.5.2 UUID/widget envelope,
including both JSON encoding
layers. No mixed large batch or summary fragment/chunk protocol is selected.
Rebuild a smaller explicitly truncated projection or reject fixed-overhead
failure; never widen the scanner/protocol limit. Render local clipping as an
observed retained prefix within the approved component, not a full summary or
new panel/height rule. No per-token full-turn serialization or awaited I/O in
the provider callback is needed.

### Natural commit and failure behavior

At the existing real natural end freeze one actual `RingTurn`, preserving its
counts, duration, origin and reason, with optional
`summaryHistory: {v: 1, commit, requests}`. The deterministic commit ID binds the
receiver reply ID and real source-turn identity; hosted identity additionally
binds holder/observed start sequence. Every `summaryHistory.requests` attempt
record preserves both positive safe `request` and `attempt` counters, ordered by
that pair; internal response IDs are not persisted as their replacement. The
same aggregate 16-record limit applies to live, final and retained data. Exact
terminal prefixes/parts remain independent across attempts, and response-less
attempts create no invented record. Actual abort/error/length/incomplete facts
terminalize received prefixes; tool failures alone do not relabel them. Earlier completed attempts
remain completed. No new history kind or fake answer/turn is created; rendering
associates the retained payload below its answer using the 717 component.

Register the frozen candidate in receiver RAM before ordinary intake acceptance.
That response still means applied/accepted, not persisted. Serialize natural-turn
commits per session in the daemon, append via existing immutable temp+rename,
and wake history readers only after successful rename. Actual ring records,
not the source or a new saved badge, are history truth.

On I/O failure retain and retry that exact candidate receiver-side at the existing
recovery cadence, without reapplying lifecycle/accounting or replaying previously
written tools/replies/the whole batch. Never evict an accepted pending candidate
silently. Persistent I/O/capacity failure requires explicit admission/storage
failure rather than successful acceptance followed by discarded prefixes;
ordinary harness work does not wait for a client persistence handshake. RAM
pending state may be lost if the receiver process dies before commit (725=a);
no durable mid-turn checkpoint or stronger power-loss/fsync promise is added.

Use compact receiver deduplication and existing ring IDs for uncertain duplicate
retry/replay; no full successful-payload map or client receipt API is needed.
The same frozen candidate must not append twice, overwrite a prior file or
recreate a closed turn. Quiet replay remains non-writing. Old readers ignore
optional data on a legitimate turn; old writers append different immutable
files without stripping prior bytes. Preserve 50-record live paging, 2000-file
disk pruning and cursors. A pruned/absent record never authorizes resurrection.
Snapshot reconstruction may drop unknown live data: snapshots are not retained
summary authority, and overlapping todo audit/compatibility work is not assumed
implemented.

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
  text. These are additive prior art for the selected optional-field contract,
  not an existing implemented summary path or authority for a new schema/event.
- **Coverage omissions:** current hosted Claude mapping in
  `internal/wsagent/claude.go` consumes whole assistant blocks, without a mapped
  live summary delta path. This inspection establishes no equivalent Claude,
  Codex-harness, or other provider summary capability and makes no parity promise.
  Reasoning-token counts, effort, empty/redacted thinking, captions, and existing
  model-generated tool/display summaries are not substitute summary data.
- **Supported-version/mode facts (static recheck at `367c4e5aa`):** the
  installed 1.5.2 observer is wired on the main Agent before normalization
  (`/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/core/sdk.js#L240-L275`).
  Its regular Responses stream observes parsed events before dispatch; Codex
  Responses observes them before terminal-event normalization on both SSE and
  WebSocket paths (the same installation's
  `node_modules/@earendil-works/pi-ai/dist/api/openai-responses-shared.js#L530-L575`
  and `openai-codex-responses.js#L484-L490`, `#L541-L581`, `#L1208-L1215`).
  Thus 1.5.2 terminal and hosted leads with the native ws extension loaded
  have a summary-specific source candidate, not an implemented feature or
  proven provider emission. 0.84.4 has no such inspected public hook
  (`rg -n provider_stream_event` in its `dist/core/extensions/types.d.ts`,
  `dist/core/sdk.js` and `docs/extensions.md` returned no matches).
  Its loader accepts event-name strings without validating them
  (`/Users/kang-sw/devenv/agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/loader.js#L231-L237`);
  inert optional registration is a compatibility candidate requiring offline
  load fixtures, not authority to import newer SDK types or upgrade packages.
  Intermediate versions, a minimum introduction release, other APIs/providers,
  and Claude/Codex-harness parity remain unverified. Hosted launch resolves the
  actual CLI's adjacent SDK, tries the bundled extension in default and isolated
  modes, and falls back to an MCP/identity-only bridge when unavailable
  (`agents-plugin-tool/internal/wsagent/pi_lead.go#L35-L106`, `#L181-L221`,
  `agents-plugin-tool/internal/wsagent/pi_lead_bridge.mjs`). The bridge is not
  a summary observer. Pi 1.5.2 `docs/rpc-extension-ui.md` and the checked
  `examples/extensions/rpc-demo.ts` demonstrate string-array widgets as
  fire-and-forget RPC UI records, not raw-provider session events or confirmed
  delivery; current hosted intake remains disabled.
- **Selected gate/framing/ordering evidence:** the installed 1.5.2 public
  `dist/core/model-registry.d.ts#L54-L61` exposes both provider override checks;
  `dist/core/model-runtime.js#L347-L354` returns the extension registrations.
  Both inspected host declaration sets export `VERSION`, and
  `agents-plugin-pi/scripts/bundle.mjs` keeps host imports external. Terminal
  serialization is `agents-plugin-pi/src/mcp-stdio-client.ts:request/callTool`;
  exact 1.5.2 `dist/modes/rpc/rpc-mode.js#L127-L139` and `jsonl.js:serializeJsonLine`
  define the hosted UUID/widget envelope and final stringify/LF. MCP
  `internal/mcp/server.go#L239-L240` uses the default scanner; installed Go
  1.26.2 `src/bufio/scan.go#L78-L94` gives its 64-KiB token bound, so raw text
  length alone is unsafe. The same server starts handlers concurrently
  (`#L271-L334`), requiring ingress admission rather than send-order claims.
  Exact 1.5.2 `dist/core/agent-session.js#L738-L745` awaits extension settled
  handlers before public native settlement, grounding the final hosted widget.
- **Auto-confirmed retry/admission corrections:** installed pi-ai 1.5.2
  `dist/api/openai-codex-responses.js#L178-L339` invokes `onPayload` once before
  the WebSocket retry/missing-continuation and SSE retry loops; `#L541-L577`
  exposes raw failed/error events before terminal normalization. Therefore
  one request hook is not one transport attempt: explicit serialized attempt
  counters are needed while raw response IDs remain internal. Static source
  evidence corroborates the lead's exploration result, not a fixture pass.
  MCP `internal/mcp/server.go#L271-L334` launches concurrent handlers, and
  `agents-plugin-pi/src/dashboard-feed.ts#L673-L742` can locally abandon a call
  without undoing remote effects. Tracking only post-v1 requests misses that
  earlier legacy work; connection-open tracking and actual pre-v1 operation/
  write-side-effect quiescence are required before the first optional mutation.
- **Freshness and request lifecycle facts:** terminal feed coalescing,
  resync, capability revocation on failed/off responses, and the five-second
  abandoned-call guard exist in
  `agents-plugin-pi/src/dashboard-feed.ts#L459-L742`. That local guard cannot
  undo a request already applied remotely; there is no summary revision or
  producer-generation guard today. The existing `agent` event permits a lead
  patch with omitted batches/caption preserving those values
  (`agents-plugin-tool/internal/wslive/state.go#L562-L576`, `#L671-L682`),
  and `task_counts` demonstrates a separately negotiated optional field
  (`agents-plugin-tool/internal/mcp/live_snapshot.go#L498-L503`). An optional
  summary field on that event is the selected additive reuse path; the
  stage-free contract above settles request correlation, part replacement,
  retry/late-event separation, reset/resync and hosted replay behavior.
  Implementation and offline proof are still outstanding. Provider sequence
  numbers alone do not identify a new request or producer. Pi 1.5.2 `docs/extensions.md#L336-L366` and
  `#L663-L696` distinguish a provider request/turn from the final
  `agent_settled` boundary; `agent_end` can precede retries or continuations.
  No request payload, signature or encrypted content is needed as summary text.
- **Natural-turn history and failure facts:** the ws terminal turn spans
  Pi `agent_start` through `agent_settled`, not each Pi `turn_end`
  (`agents-plugin-pi/src/dashboard-feed.ts#L758-L781`). The reducer emits the
  existing natural `RingTurn` only with an open start, preserving duration,
  counts, origin and supplied reason
  (`agents-plugin-tool/internal/wslive/state.go#L289-L312`). Terminal aborts
  already use `agent_settled.aborted` with the pinned-version stop-reason
  fallback (`agents-plugin-pi/test/dashboard-feed.test.ts#L180-L216`). Hosted
  Pi maps message errors but the runner currently emits a reasonless live
  turn end (`agents-plugin-tool/internal/wsagent/pi.go#L830-L839`,
  `#L992-L1016`, `agents-plugin-tool/internal/wsagent/runner.go#L2725-L2744`).
  Neither path retains summary prefixes yet. The lead has selected optional
  ordered `summaryHistory` on the existing real natural turn record, without a
  new event kind or schema/protocol version. It must preserve actual
  failed/aborted/incomplete facts independently of the turn's tool-failure count and must not concatenate a
  retry or manufacture completion. The selected receiver-owned contract now
  supplies those mechanics under the settled 725/726 policy. Existing ring writes
  are best effort: terminal intake logs append failure without surfacing it, and
  the runner logs append failure
  (`agents-plugin-tool/internal/mcp/live_snapshot.go#L684-L698`,
  `agents-plugin-tool/internal/wsagent/record.go#L415-L431`). Append acceptance,
  failure handling and replay/idempotency are selected requirements above,
  still awaiting implementation/proof; no exactly-once transaction or durable
  mid-turn checkpoint exists by implication.
- **Old/new write-preservation facts:** snapshot tolerance is read-only
  evidence, not retention authority. Terminal intake creates `NewState`;
  runner recovery seeds finished agents, usage and commands, and both rebuild
  typed snapshots (`agents-plugin-tool/internal/mcp/live_snapshot.go#L199-L225`,
  `agents-plugin-tool/internal/wsagent/record.go#L349-L373`,
  `agents-plugin-tool/internal/wslive/state.go#L975-L1029`). An older snapshot
  writer therefore drops unknown live/in-progress summary data. In contrast,
  ring appends create fresh names through temp+rename, readers ignore unknown
  fields, and prior entry files are never rewritten
  (`agents-plugin-tool/internal/wslive/ring.go#L166-L219`, `#L253-L280`,
  `#L321-L337`). Optional metadata on immutable natural-turn entries is a
  preservation candidate until ordinary pruning: disk cap 2000 and live page
  cap 50 (`agents-plugin-tool/internal/wslive/snapshot.go#L81-L86`). This is
  not unlimited retention. Existing `TestRingReadSkipsJunkAndToleratesUnknownFields`
  checks reading, not old-writer byte preservation
  (`agents-plugin-tool/internal/wslive/ring_test.go#L142-L161`). A mixed-version
  append/read/byte-identity fixture and separate pruning/paging fixtures are
  still required before certifying T0 under `ai-docs/manuals/release-compatibility.md`.
- **Overlapping work and exact verification gaps:**
  `261114-feat-ws-conversation-audit-event-store` and
  `261114-feat-ws-dashboard-conversation-audit-view` are both todo with no
  Results; their broader raw/hidden-event audit store and read API are not an
  existing summary transport or retention mechanism. The todo
  `261112-feat-ws-mixed-version-compat-guards` has no Results either; its writer
  audit cannot be assumed landed. Existing feed, hosted-widget, ring and intake
  tests provide reuse points (`agents-plugin-pi/test/dashboard-feed.test.ts`,
  `agents-plugin-pi/test/hosted-dashboard-events.test.ts`,
  `agents-plugin-tool/internal/wsagent/wake_test.go#L145-L181`,
  `agents-plugin-tool/internal/mcp/live_snapshot_test.go`). The scope-bounded
  search `rg -n 'reasoningSummary|reasoningSummaries|reasoning_summary|provider_stream_event'
  agents-plugin-pi/src agents-plugin-tool/internal/{wslive,wsagent,mcp,wsdashboard}`
  returned no matches. Explicit-summary provenance, request/freshness guards,
  committed normal and failed-prefix retention, old/new writes, caps/escaping,
  and both-width actual UI conformance are therefore not verified by existing
  feature tests. Static inspection and source/reference byte comparisons only
  were performed for this population; no build, test, browser, provider request,
  probe or runtime operation was run.

## Technical Readiness

No open owner questions remain: IDs 710–712, 717, 725 and 726 are answered.
Fact population is present, and the lead has resolved its three technical gaps
with the selected contract above. Sage design/completeness review is next; this
ticket remains accepted todo until that gate completes. No new owner promotion
wait, global ID, source implementation or readiness claim is introduced.

Review must check completeness/compatibility of the selected source matrix,
raw-stream correlation, serialized bounds, ingress/owner fencing, hosted final
ordering and receiver commit/retry semantics. Both final important review
concerns are now auto-confirmed in the explicit attempt/aggregate-budget contract
and first-v1 quiescence barrier; the same design/completeness delta review is
next, not an owner approval handoff. Offline fixtures and actual
both-width UI evidence remain future implementation verification, not facts
already proven by static reads. Internal helper/export names may follow existing
module conventions; they must not reintroduce the rejected persistence handshake
or broaden the selected wire/store mechanism.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

Implementation must read `ai-docs/manuals/shipped-surface-boundary.md` and
`release-compatibility.md`; MCP edits also require `ws-mcp.md`. Pi edits must use
exact supported-version official docs/examples and follow relevant event/SDK
cross-references. Do not modify provider requests, reasoning effort, or upstream
SDK versions merely to make data appear without an approved decision.

Respect the 725=a persistence boundary: do not promise durable mid-turn retention
or silently introduce a new checkpoint mechanism to avoid the explicitly accepted
precommit process-loss limit. Respect 726=a: retain only the received provider
summary prefix with its actual terminal status; do not infer or synthesize missing
text. Use the lead-settled minimal contract above; changes needing a new event
kind, schema/protocol version or altered legacy semantics require returning the
actual incompatibility to the lead, not silently treating optional as T0. Keep
the three 717-approved assets unchanged and the received-prefix status separate
from local truncation so clipping cannot be mistaken for complete provider text.
Preserve explicit `(request, attempt)` identity and its aggregate 16-record limit
to prevent retry concatenation/resource bypass. Do not admit the first optional
mutation while an earlier canonical operation or its write effects can still
land; local timeout/cancellation is not evidence of quiescence.

## Prior Decisions

- 261114-feat-ws-dashboard-answer-ack-nudge (2026-11-14, commit bf4e1be1): "turnStart refuses a turn with no duration (Decision 4), and newestTurn never falls back to an older turn" — bearing: constrains
- 9feb6485 (2026-11-14, commit): "Pi 0.84.4 AgentSettledEvent has no aborted field, so retain turn_end.message.stopReason as a fallback; newer hosts expose agent_settled.aborted directly." — bearing: constrains
- 261113-feat-ws-compaction-before-queued-followup (2026-11-14, commit 02baa619): "The hold gates mail and child reports too: a mail-only turn would push the compaction back the same way (Decision 3, hold scope)." — bearing: constrains
- 261112-feat-ws-dashboard-lead-wake-events (2026-11-13, commit fd75fe2f): "Gate wake separately and gate new status fields with task_counts so older status-capable daemons never receive unsupported count fields." — bearing: supports
- edb6f8a2 (2026-11-11, commit): "Slot ownership is per session with takeover: a re-hosted session's new server owns the copy; the older slot's publish and close become no-ops." — bearing: constrains
- b679d6b9 (2026-11-07, commit): "Order at turn_end: tools, reply, tools the close finished, turn line. turn_start flushes only ahead of a reply it settles." — bearing: constrains
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, commit 14605071): "Rejected: a snapshot per event (an unbounded log), and one ring file per session (it would need locking across MCP processes that share a key)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/dashboard-feed.ts, src/index.ts, src/hosted-dashboard-events.ts; agents-plugin-tool/internal/wslive, internal/mcp/live_snapshot.go, internal/wsagent, internal/wsdashboard/pageview.go and assets/dashboard.tmpl |
| scope.surface | cross-module | Pi extension, adapter intake/hosted runner, shared live/history data and dashboard rendering; intended T0 only |
| scope.new_public_symbol | yes | selected optional reasoningSummary on existing agent/real turn_end, lead live projection, optional RingTurn summaryHistory and reasoning_summary_v1 capability; no new tool/event kind; internal helper names follow module conventions |
| scope.new_type_contract | yes | auto-confirmed request+attempt identity, active pair/null and aggregate 16-attempt budget across stage-free live/final/history data; atomic terminal end, ordered hosted end and receiver-owned commit; first-v1 quiescence barrier and T0/legacy invariants require delta review |
| scope.test_surface | existing | agents-plugin-pi/test/dashboard-feed.test.ts, test/hosted-dashboard-events.test.ts; agents-plugin-tool/internal/wslive/ring_test.go, internal/mcp/live_snapshot_test.go, internal/wsagent/wake_test.go; new fixture-file need is unsettled |
| complexity.reuse_points | confirmed | selected createDashboardFeed queue/capabilities, piHostedEvents widget filtering, State.Apply real turn end, immutable AppendRing/ReadRingPage, newLiveWriter and conversation; ingress FIFO, owner checks and RAM retry must be implemented |
| complexity.side_effect_risk | high | pre-v1 canonical handlers and deferred writes may outlive local abandon; track from connection open and establish actual bounded quiescence before optional mutation, then ingress FIFO/owner fences |
| risk.correctness | high | onPayload once can cover transport retries/continuation recovery; explicit per-request attempt counters, bound-response-only records, aggregate caps and late-legacy mutation fencing prevent contamination |
| risk.fit | moderate | exact 1.5.2 built-in source modes and minimal receiver-owned integration are lead-selected; design/completeness must review their T0/legacy and failure boundaries |
| risk.test | high | 1.5.2 observer versus 0.84.4 gap, hosted/terminal skew, crash/commit boundary and actual 1280/360 UI evidence are not covered by implemented summary tests |
| risk.security_or_contract | high | private thinking/signatures must never enter browser/history; optional shared-store data must remain T0 under old/new writers |

## Phases

### Phase 1: Carry supported lead summaries into live activity and history

After Sage design/completeness and readiness gates clear, implement the selected
lead-only contract in one slice: eligible raw observer; bounded stage-free live
projection; optional final data on real terminal turn_end; ordered hosted final
widget/native settlement; explicit request/attempt identity with aggregate
budgets; connection-open tracking, first-v1 quiescence and ingress/owner/revision
fencing; and receiver-owned frozen natural RingTurn commit/retry. Preserve ordinary replies, tools, usage,
turn accounting and old-version fallback. No subagent slice, client persistence
handshake, new event kind/schema/protocol/tool or durable checkpoint is included.
Follow unchanged 717-approved UI placement and settled 725/726 natural-commit,
precommit-loss and actual failed-prefix semantics.

Required verification before claiming implementation success:

- Pin exact 1.5.2 native-extension terminal/hosted OpenAI Responses SSE and Codex
  SSE/WS fixtures; verify host VERSION/override checks in source and shipped
  bundle. 0.84.4, unknown versions/providers, MCP-only fallback, no summary text
  and empty/redacted/signature-only input keep ordinary activity. No SDK upgrade
  or live provider request is needed to prove this contract.
- Exercise raw callback-before-message_start, interleaved item/part ordering,
  duplicate sequence numbers and authoritative done replacement within each
  request/attempt. Prove `onPayload` once before transport retry, including Codex
  missing-continuation recovery: an observed failed attempt followed by a new
  bound response advances attempt, not request. Preserve both counters and each
  exact failed prefix in live/final/history; reject unexplained overlap and
  retired responses; no bound response means no invented history. Exercise
  actual error/abort/length/incomplete facts.
  Prove generic thinking, signatures, encrypted content, missing-text synthesis
  and subagent input never enter the new live/history/UI path.
- Measure actual final JSONL UTF-8 bytes for ASCII controls, quotes/backslashes,
  non-ASCII, long fixed fields and both widget encoding layers: at most 49,152
  bytes including both counters, active-pair metadata and all envelopes/LF.
  Check 8-KiB/24-KiB limits and **16 total attempt records**, including many
  retries under one request and mixed requests. Bound omission clears the active
  pair to null. Verify zero-byte rejection/canceled unused IDs, exact retained
  prefixes, observed/retained byte counts and separate local truncation versus
  provider status. No scanner-limit
  change, chunks, false full-summary claim or executable markup is acceptable.
- Start a canonical legacy intake before the first v1 call, locally abandon or
  time it out, then delay both its handler and separately its deferred write
  effects while v1 reset/start/end/retry calls arrive. Prove ingress tracking
  starts at connection open and the first optional mutation waits for actual
  quiescence; cancellation/response completion alone cannot release it. Exercise
  bounded failure to quiesce: optional admission is refused/revoked without
  mutation. Then verify FIFO, producer retirement, conflicting revisions,
  replacement/takeover owner fences and unchanged unrelated-tool/legacy
  scheduling/accounting. Terminal final data/end is atomic; hosted
  final projection precedes native settlement without becoming an end command.
  Quiet/wrapped holder replay preserves existing sequence/hold boundaries.
- Fail natural-turn append, retry its frozen candidate, lose ordinary intake
  responses and exercise persistent I/O/capacity failure. Prove no accepted
  candidate is quietly evicted, saved before rename, appended twice, recounted
  or retried as a whole tools/reply batch. Process death before commit may lose
  RAM state; after commit, reload/reconnect/writer replacement preserves real
  history. Actual aborted/failed/incomplete prefixes and earlier completed
  attempts remain distinct, with no client persistence handshake.
- Read a new summary-bearing natural turn with an old reader, append using an
  old writer, and compare original bytes before a new reader recovers it.
  Exercise 50-record paging/2000-file pruning, no pruned-record resurrection,
  and older snapshot reconstruction dropping live data without changing ring
  authority. Static tolerant parsing alone is insufficient T0 proof.
- Capture actual conversation UI at 1280 and 360 and compare side by side with
  the unchanged approved screenshots: live, ordinary fallback and retained
  below-answer collapsed/expanded summary. Add no subagent surface or inferred
  height/layout rules; prefix/status qualification stays in that component.

Run applicable Pi tests (`npm test` in `agents-plugin-pi`) and the Go suite
(`go test ./...` in `agents-plugin-tool`) in MAIN's assigned implementation slot.
These are future verification obligations, not runs claimed by ticket capture.
During this ticket-settlement step, use static reads, ticket edits, formatting
checks, the ticket guard and Git diff only. Run no builds, code tests, browser
work, provider requests, probes, SSH or daemon/runtime actions;
heavy implementation verification waits for MAIN's assigned slot, without
waiving any verification above (current owner dispatch instruction supersedes
the obsolete temporary 649-exclusive capture wording).

## Blocked - 2026-11-15

**VERIFIED:** MAIN's 23:10:43/23:12 authorization is consumed: one exact
empty-derived key-only interpreter diagnostic and one once-only receiver-02
follow-on invocation. Both invocation outcomes are recorded; no further execution
is authorized by that instruction. Retained branch:
`impl/feat/live-reasoning-summary-675/fruit-annex-hunk`, HEAD `d459a96bd`.
Receiver binding remains `5d018785c8caddc237cf24bb5229c871aac8783b`; schema
checkpoint remains `a5b05c09a4e41a99c50fd8055108f2df82f8947d`.

- Original receiver-01 recipe/supervisor and raw failure remain preserved.
  Exact diagnostic command/output and prior failure are in
  `ai-docs/.plans/2026-11/14-2329-261114-live-summary-bootstrap-evidence-02.md`.
  All five expected-key equality flags were true; the only unexpected key was
  `__CF_USER_TEXT_ENCODING`. No unexpected value was reported.
- New recipe:
  `ai-docs/.plans/2026-11/14-2329-261114-live-summary-receiver-verification-batch-02.md`;
  derivative supervisor:
  `ai-docs/.plans/2026-11/14-2341-261114-live-summary-bounded-supervisor-02.py`,
  Git blob `f8e09177a5e833973a0c929d45edd0df99e3c76d`. The only operational
  deltas were discarding that unsolicited key before unchanged exact environment
  guards and substituting exclusive output root `receiver-02`.
- Follow-on exit: **2**, `unlisted owned descendant executable`. The first owned
  snapshot rejected anchor PID/PGID/SID **17803**, observed as
  `/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/Resources/Python.app/Contents/MacOS/Python`,
  against configured interpreter admission identity
  `/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/bin/python3.9`.
  This occurred before metadata commands, formatting or tests: **0/23** format
  files, **0/21** receiver tests, historical child not started. No fixture
  listener or holder was created.
- Complete receipts were read at
  `/tmp/ws-live-summary-675-receiver-02/logs/supervisor.jsonl:1–15`,
  `/tmp/ws-live-summary-675-receiver-02/logs/anchor.jsonl:1`, and
  `/tmp/ws-live-summary-675-receiver-02/logs/anchor.stderr` (empty).
  Both owned ps helpers exited/reaped zero. Owned TERM preceded the anchor's
  sole reap (`-15`); the final owned snapshot contained only its zombie and no
  descendants. Positive absence was true, detached lineage empty, pipes closed,
  cleanup error null, and elapsed time **0.177410167 seconds**, within the
  unchanged 630-second work plus 90-second cleanup bound.

**INFERRED:** No executable equivalence or specific admission correction is
established. MAIN disposition is pending; no fix is selected. Feature acceptance,
full suites, Pi checks, visuals and independent lite review remain unproven.
Idle-reconnect remains idea/out of scope; **T0 is the target, not certified**.
Existing phases/results, source binding, recipes and evidence are unchanged by
this status note. No phase Result or ticket-completion claim is added.

omitted: Further probes, diagnostics, repairs, reruns, commits, integration,
release and authority expansion; none is authorized by this note.
