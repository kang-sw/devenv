# Plan: Pi adapter: owner audit window for subagent conversations (`/audit`) and owner steering with last-writer settle ownership — Phase 2 durable fork-raised `/done` reconciliation sub-slice only

## Relevant Ticket Contract
- For a fork-raised thread, the first `/done` allocates and persists one `reconciliationId`; its `.ws-threads.json` record remains canonical through terminal delivery and successful park, while repeated `/done` after dormant/unbound is a no-op.
- The reconciliation state machine must use authoritative final acceptance, `agent_settled`, lead-delivery enqueue, closeout dispatch, and park completion; it must not infer delivery from runtime flags, timestamps, or an absent `pendingFinal`.
- A streaming fork waits for its next settle. An idle fork with a delivered final parks silently; one with neither final nor lead-visible settlement gets exactly one lead-attributed closeout. A closeout that has no valid final or asks again emits the existing missing-final advisory and parks without synthetic success.
- Terminal final/settled/advisory delivery is a persisted, at-least-once outbox event keyed by `reconciliationId`, with durable lead-ingress dedupe before model visibility. Persist each external transition; clear reconciliation/outbox only after durable delivery acknowledgement and park.
- The child must durably accept a same-ID control envelope before its closeout prompt enters the model queue; a retried ID acknowledges without a second model turn.
- Tests must inject failures at every state write, child acceptance, dispatch acknowledgement, terminal enqueue, ingress dedupe, and park boundary; cover streaming, suppressed owner-held idle settle, delivered final, invalid closeout, repeated `/done`, reload, and process restart.

## Out of Scope
- The rest of Phase 2: `/audit` interactive binding, `lastWriter`/`ownerSends`, owner-held routing/exemptions, widget/API flags, modal controls, and `interrupt` RPC capability work.
- Broader owner steering or modal behavior, ticket Result updates, `agents-plugin-tool/`, and changes to `session_children`.

## Codebase Findings
- `agents-plugin-pi/src/ask.ts#L267-L375` — `ThreadRecord` is the durable fork-raised owner record, but has no reconciliation state/outbox; `saveThreadRegistryFile`/`hydrateThreadRegistry` at `#L978-L1018` use a best-effort whole-file write and restore ordinary thread data only.
- `agents-plugin-pi/src/ask.ts#L1306-L1345` — current fork-raised `/done` immediately clears `threadBound`, marks the thread dormant, and persists it; this is the live cleanup gap the reconciliation state machine must replace without releasing canonical ownership early.
- `agents-plugin-pi/src/spawner.ts#L1644-L1709` and `#L2222-L2430` — final handling is volatile (`pendingFinal`), then `pushToLead` uses an in-process FIFO and `attachEventListener` can silently park a non-thread-bound child after settle. Neither is a durable terminal outbox or ingress-dedupe boundary.
- `agents-plugin-pi/src/agent-sidecar.ts#L133-L169` and `agents-plugin-pi/src/index.ts#L237-L260` — the generic shutdown sidecar explicitly skips `threadBound` records; shutdown later refreshes `forkResume` into the thread file. This supports the ticket's required canonical thread-store ownership but does not persist a reconciliation state machine.
- `agents-plugin-pi/src/fork.ts#L409-L567` — `wireAntiBleedLoop` is a separate listener that treats `agent_settled` as a nudge/advisory decision. Reconciliation must coordinate with this existing event ordering rather than use `running`/settle timing as delivery evidence.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-types.d.ts#L14-L120` and `rpc-client.d.ts#L35-L91` — the installed public parent-to-child RPC surface exposes raw `prompt`/`steer`/`followUp`/`abort`, not an adapter control-envelope command or child durable-acceptance acknowledgement. `core/extensions/types.d.ts#L971-L974` likewise exposes `sendMessage` as void with no observable durable lead-delivery acknowledgement.
- `agents-plugin-pi/test/ask.test.ts#L429-L484`, `test/spawner.test.ts#L1625-L2424`, `test/agent-sidecar.test.ts#L49-L666`, and `test/fork-lifecycle.integration.test.ts#L1-L330` cover ordinary hydration, settle/park, sidecar revival, and fork RPC lifecycle, but contain no injectable crash/at-least-once control or delivery protocol seam.

## Implementation Plan
- Escalate to research before execution.

## Verification Plan
- Research must identify a supported child control/ack transport that can persist `reconciliationId` acceptance before model-queue admission, and a durable lead-ingress acknowledgement point for `sendMessage` delivery; otherwise the required exactly-one child turn and at-least-once/one-visible-terminal-event assertions are not implementable against the current public Pi contract.
- After that decision, add deterministic crash-injection tests around the selected state transitions in `agents-plugin-pi/test/ask.test.ts`, `test/spawner.test.ts`, `test/agent-sidecar.test.ts`, and a real child transport test alongside `test/fork-lifecycle.integration.test.ts`; run `cd agents-plugin-pi && npm test -- test/ask.test.ts test/spawner.test.ts test/agent-sidecar.test.ts test/fork.test.ts test/fork-lifecycle.integration.test.ts` followed by `npm test`.

## Escalations
- Confidence: medium
- Reason: The selected contract requires child-side durable acceptance before model-queue admission and durable lead delivery acknowledgement. Existing RPC accepts only raw text commands, while current push delivery is an in-memory FIFO over a void `sendMessage`; neither exposes the required acknowledgement boundary.
- Research should decide: whether a supported Pi RPC/extension mechanism can provide the child control-envelope acknowledgement and lead ingress acknowledgement without modifying Pi, or whether satisfying the accepted contract requires upstream Pi protocol support/a deliberately scoped adapter transport extension.
