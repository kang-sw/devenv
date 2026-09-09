# Plan: Show agent model, effort, latest input tokens, and estimated cost in widget rows — Phase 1: Add event-backed agent row telemetry

## Relevant Ticket Contract
- Each live-agent row must show the actual resolved `<model-name> (<effort>)` when available, the most-recent model call's reported input tokens, and cumulative estimated child-attributable USD cost. Each unavailable field renders `—` independently; never substitute zero, a session total, or a price.
- Latest input is the reported per-model-call `usage.input`, with cache fields preserved according to Pi's usage model; it is not a cumulative session-token reading. Cost must count each child call once across continuation and dormant resume, exclude inherited fork history, and remain `—` when a child-only total cannot be recovered safely.
- Telemetry collection belongs outside `render(width)`. Rendering remains side-effect free: no RPC, model, filesystem, or per-frame polling. Preserve the existing row order, awaiting-row visibility, cap, question hints, resolved count heading, and 40/80/120-column bounds. At 40 columns, telemetry must not silently displace an awaiting owner's `/answer <id>` cue; a width-aware abbreviation may be chosen during implementation.
- This is adapter-only. No Go/shared resource change, provider billing or pricing lookup, model call, delegation/settlement behavior change, or new model tool is authorized.

## Out of Scope
- The recently landed agent-panel heading/count behavior, including its uncapped deduplicated count, pending-question suffix, body cap, and `+N more` summary.
- Billing integration, pricing/catalog/model requests, and any model-visible tool or workflow behavior.
- Unrelated one-shot explore accounting and source outside `agents-plugin-pi/`.

## Codebase Findings
- `agents-plugin-pi/src/agent-widget.ts#L62-L72,175-L218,260-L299` — rows are pure projections of the RPC and thread registries; rendering is width-aware and heading-first. The current row shape has no telemetry fields, while the formatter places `/answer <id>` after the ordinary row text, creating the 40-column cue-priority constraint.
- `agents-plugin-pi/src/spawner.ts#L826-L929,1969-L1985,2580-L2670,2793-L2845` — each `RpcAgentRecord` already carries `modelBase`/`modelEffort`; fork launch and every dormant resume create a child-local `RpcClient`, with `captureForkSelection` refreshing the actual fork selection. Resume reuses the record model/effort unless that fork-specific refresh occurs.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/json-event.d.ts#L10-L25` and `.../json-event.js#L16-L29` — RPC `message_update.usage` is a cumulative assistant snapshot with the message omitted, so it cannot be added per update. A finalized `message_end` retains the assistant message, but event payloads provide no required stable message/event identifier.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-ai/dist/types.d.ts#L265-L304` — one finalized assistant message has per-call `usage` (input, output, cache read/write, and estimated cost breakdown) and only an optional `responseId`; `timestamp` is present but is not documented as an event identity.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.d.ts#L154-L170` and `.../core/agent-session.d.ts#L179-L202` — `getSessionStats()` exposes a cumulative session cost, which is unsafe for a fork because it cannot distinguish inherited parent history from child calls.
- `agents-plugin-pi/src/spawner.ts#L2159-L2246,2299-L2345` — the existing listener is per child client and already owns lifecycle mutations/refreshes, but `applyRpcEvent` intentionally ignores `message_update` and `message_end`; it has no durable telemetry state or per-call dedupe ledger.
- `agents-plugin-pi/src/agent-sidecar.ts#L110-L172,186-L289` and `agents-plugin-pi/src/index.ts#L514-L548,660-L676` — ordinary non-thread-bound records are snapshotted to the shutdown sidecar and revived as dormant records. Extending only this path would lose telemetry for an owner-held respondent.
- `agents-plugin-pi/src/ask.ts#L218-L256,395-L410,456-L495` — owner-held/respondent-fork recovery uses the separate thread registry and its denormalized `PersistedForkResume`, while `captureOrphans` deliberately excludes `threadBound` records. Thus both recovery formats need a common, validated telemetry snapshot and identity ledger.

## Implementation Plan
- Escalate to research before execution.

## Verification Plan
- Research must establish a required, stable identity for every finalized child model call that survives duplicate delivery and both durable recovery routes, or establish a Pi-guaranteed at-most-once `message_end` contract that makes an identity ledger unnecessary.
- Once that attribution contract is resolved, define focused `agents-plugin-pi/test/agent-widget.test.ts`, `test/spawner.test.ts`, `test/agent-sidecar.test.ts`, and `test/ask.test.ts` cases for model refresh, changing latest input with accumulating cost, repeated/delta events, fork prefix exclusion, sidecar and thread-registry revival, missing values, 40/80/120 rendering with retained `/answer` priority, and unchanged waiting rows/hints.
- Run `cd agents-plugin-pi && node --test test/*.test.ts`; compare the result to the known baseline of 130 failures (129 Linux SDK-path fixtures and one stale `ws-ask` expectation).

## Escalations
- Confidence: low
- Reason: The bounded SDK survey proves that `message_end` carries child-local per-call usage and that `message_update`/`getSessionStats()` are unsafe sources, but it does not establish a mandatory event identity. `AssistantMessage.responseId` is optional and no required message/event ID is exposed. Inventing a timestamp/content fingerprint or treating session totals as child totals would violate the ticket's once-only and fork-prefix requirements.
- Research should decide: whether Pi guarantees exactly-once finalized `message_end` delivery per `RpcClient` across reconnect/resume; if it does not, what stable call identifier is available for all providers; and how the same normalized snapshot/identity ledger should validate and round-trip through both `PersistedOrphan` sidecars and `ThreadRecord.forkResume` recovery without double counting.
