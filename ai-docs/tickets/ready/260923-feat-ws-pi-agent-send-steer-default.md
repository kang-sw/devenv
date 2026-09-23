---
title: Pi ws-agent-send always steers a running subagent
parent: 260908-epic-ws-pi-subagent-conversation-view
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dd88d28cb1972ada
sage-review-completeness-reviewed: dd88d28cb1972ada
---

# Pi ws-agent-send always steers a running subagent

## Background

`ws-agent-send(agent_id, message, interrupt?)` in the Pi adapter
(`agents-plugin-pi/src/spawner.ts`, tool registration and `sendToAgent`)
delivers to a mid-stream subagent as `followUp()` unless the lead passes
`interrupt: true`, which maps to `steer()`. A follow-up drains only when the
child would otherwise end its whole run, so a lead message sent without the
flag reads as "delivered only when the subagent is idle". The flag name also
misleads: `interrupt: true` never interrupts anything; it only steers.

Pi SDK semantics (pi-coding-agent 0.84.4): `steer()` is delivered after the
current assistant turn's tool-call batch finishes, before the next LLM call;
it does not cancel in-flight tool calls. Idle and dormant subagents already
receive `prompt()` regardless of the flag. Owner sends from `/audit` and
`/answer` already steer while the child is streaming.

## Decisions

- Remove the `interrupt` parameter from the `ws-agent-send` tool schema
  entirely (not accepted-and-ignored). The Pi tool schema is model-facing
  only, so there is no external caller compatibility to preserve.
- Every lead send to a mid-stream subagent is delivered with `steer()`.
  Idle and dormant delivery stays `prompt()` (dormant auto-resume unchanged).
- The lead tool no longer reaches `followUp()`. The `followUp()` branch and
  the internal `interrupt` parameter of `sendToAgent` stay for non-lead
  callers that must queue after run end: the owner finish action
  (`audit.ts`) and the fork-finish closeout (`spawner.ts`).
- The `interrupt: true` tag in the `ws-agent-send` tool-row summary
  (`agents-plugin-pi/src/tool-row-render.ts`) and its test are removed with
  the schema property.
- Owner-side sends (`/audit`, `/answer`) keep their current behavior.
- supersedes 260903-feat-ws-pi-subagent-rpc-ux (lead-tool mapping only): a
  lead follow-up read as idle-only delivery; the lead tool now always steers.
- Rejected: abort-then-deliver (abort the in-flight tool call, then
  `prompt()` the message). Deferred because abort interacts with the child's
  process tree and nested subagents and needs its own design; the lead can
  already cut a run with `ws-agent-stop` followed by `ws-agent-send`.
- Rejected: steering after each individual tool call inside a batch. Pi
  0.84.4 core polls the steering queue only at turn end; per-call delivery
  would need a child-side hook or an upstream change.

## Prior Decisions

- 260903-feat-ws-pi-subagent-rpc-ux (2026-09-04, Result): "`ws-agent-send` branches on tracked streaming state — `prompt()` for an idle/just-resumed child, `steer()` for `interrupt` mid-stream, `followUp()` to queue during an active run." — bearing: constrains
- ed294ca8 (2026-09-04, commit): "ws-agent-send branches on locally-tracked streaming state ... followUp()/steer() only enqueue and are drained solely by an active agent-loop run" — bearing: constrains
- 8abefa9b (2026-09-04, commit): "Added a fakeRpcClient() duck-typed stub (steer/followUp/prompt/getLastAssistantText) cast as RpcClient, covering all three live branches" — bearing: supports
- 4f6ecbda (2026-09-13, commit): "Arm queued-work obligations before the RPC call and roll them back on rejection, closing the acknowledgement/event race without depending on expanded prompt text." — bearing: constrains
- 654f2fe4 (2026-09-05, commit): "`running` is deliberately a different flag from `streaming`. `streaming` is event-confirmed ...; `running` latches at prompt-ISSUE time (`promptAgent`, the single funnel every dispatch now goes through)" — bearing: constrains
- 8cd3ac13 (2026-09-05, commit): "sendToAgent's steer/followUp branch does not touch runStartedAt (intended)" — bearing: constrains
- 90cf3be7 (2026-09-22, commit): "keeps the initial nested-dispatch busy publication fail-closed, and isolates later publication failures with bounded diagnostics." — bearing: constrains
- 260504-feat-ws-mcp-hook-driven-interrupt (2026-05-04, Decisions): "Treat `agents.interrupt` as durable message delivery, not cancellation." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/pi-lead-guide.md, agents-plugin-pi/test/spawner.test.ts; unnamed dependents agents-plugin-pi/src/tool-row-render.ts#L101-L108 and agents-plugin-pi/test/tool-row-render.test.ts#L162-L163 also read interrupt |
| scope.surface | public-interface | model-facing ws-agent-send tool schema at agents-plugin-pi/src/spawner.ts#L3721-L3748; exported sendToAgent#L3086-L3092 shared by ask.ts#L1423, audit.ts#L743, audit.ts#L781, spawner.ts#L1492 |
| scope.new_public_symbol | no | none; the change removes the interrupt schema property |
| scope.new_type_contract | no | none; sendToAgent may keep its internal interrupt parameter per Phase 1 |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts fakeRpcClient at L1501-L1515; agents-plugin-pi/test/tool-row-render.test.ts#L162-L163 asserts the interrupt tag |
| complexity.reuse_points | confirmed | existing sendToAgent steer branch agents-plugin-pi/src/spawner.ts#L3219-L3237 and fakeRpcClient test stub |
| complexity.side_effect_risk | moderate | sendToAgent is shared with owner /answer, /audit, audit finish, and fork-finish closeout callers that pass false and rely on followUp |
| risk.correctness | moderate | lead streaming delivery moves from followUp to steer, changing when the message lands relative to queued-work boundaries and run end |
| risk.fit | low | Decisions match the existing streaming branch; only the lead tool path changes |
| risk.test | moderate | existing streaming no-interrupt followUp test and tool-row-render interrupt test must be rewritten, not only added to |
| risk.security_or_contract | moderate | removes a property from a model-facing tool schema and changes documented delivery semantics in pi-lead-guide.md |

## Phases

### Phase 1: Drop interrupt and steer by default

Remove `interrupt` from the `ws-agent-send` schema, its `execute` params, and
the `sendToAgent` lead path so a streaming target always receives `steer()`.
Keep `sendToAgent`'s internal `interrupt` parameter and `followUp()` branch
for the owner finish and fork-finish callers; the tool's `execute` calls
`sendToAgent` with `interrupt = true` hardcoded and exposes no choice. The
`pendingQueuedWork` arm/rollback and `running` re-mark around the streaming
branch stay unchanged for the steer path. Remove the `interrupt: true` tag from
`buildAgentSendSummary` in `agents-plugin-pi/src/tool-row-render.ts`. Grep `agents-plugin-pi/` for
remaining lead-facing `interrupt` mentions and update them. Update the tool
description and the `ws-agent-send` row in `agents-plugin-pi/pi-lead-guide.md`
(drop its "follow-up" wording) to state: running -> steered after the current tool batch, before the next
model call; idle or parked -> delivered as a new prompt (parked agents are
resumed first).

Verify with `agents-plugin-pi/test/spawner.test.ts`: a lead send to a
streaming agent calls `steer()` and never `followUp()`; idle and dormant sends
still call `prompt()`; the registered tool schema has no `interrupt`
property; `agents-plugin-pi/test/tool-row-render.test.ts` no longer expects
the `interrupt: true` tag. Existing owner-send, owner-finish, and fork-finish
tests stay green; the direct `sendToAgent(..., false)` followUp test stays,
relabelled as the internal path.
