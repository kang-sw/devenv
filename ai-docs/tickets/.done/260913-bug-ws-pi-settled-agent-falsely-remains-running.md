---
title: "Stop counting idle-settled Pi agents as running after terminal result delivery"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 416abc74407a1131
sage-review-completeness-reviewed: 416abc74407a1131
completed: 2026-09-13
---

# Stop counting idle-settled Pi agents as running after terminal result delivery

## Background

Live downstream dogfooding reproduced a contradictory Pi subagent lifecycle twice. An exploration agent emitted `agent_settled` with `reason: idle`, and the lead received its ordinary `last_message`, but the same pushed message ended with:

```text
1 delegated agent still running
```

The first reproduction delivered only a short acknowledgement as `last_message`. The lead trusted the running-status line and waited, although the settled agent could not produce more output without an explicit follow-up. The second reproduction delivered a complete, rich analysis as `last_message` and still reported one agent running. This demonstrates that an ordinary settled result can be the valid terminal result—especially for exploration—and that requiring every role to call `ws-report-to-lead(kind: "final")` is not the correct general fix.

At the same point, `ws-agent-list` classified the retained non-streaming client as `idle`, while the live agent widget classified any retained client as `running`. Stopping the agent changed the pushed count to zero and the record to dormant.

Source inspection found three coupled causes:

- `computeFanIn()` in `agents-plugin-pi/src/spawner.ts` counts `expectedReport` as running even after `agent_settled` has cleared `record.running` and `record.streaming`.
- The idle-settle ordinary-result path pushes `ws-agent-settled` with an empty `TerminalDelivery`; unlike the exited-child path, successful direct-parent delivery does not discharge the report obligation. `expectedReport` therefore remains true and also blocks automatic parking.
- `classifyRegistryRowState()` in `agents-plugin-pi/src/agent-widget.ts` renders any retained live client as running, regardless of its settled and non-streaming state.

The result is a deterministic orchestration stall rather than a model still doing work: the lead is instructed to wait for output that cannot arrive autonomously.

## Decisions

- `agent_settled` is the terminal lifecycle event for every role. Its `last_message` is the role's final output and the agent is no longer running.
- Remove `ws-report-to-lead(kind: "final")` as a required or public terminal channel. `ws-report-to-lead` remains available for progress, questions, and intermediate findings that must arrive before settlement.
- Ticket workers and forks may retain structured final-output templates in their prompts and playbooks, but the adapter does not parse those templates or make terminal delivery depend on a model choosing a tool call. The lead reads the settled output and judges its adequacy qualitatively.
- A missing, malformed, or insufficient settled output is not autonomous work. The lead resumes or nudges the same persistent agent; that new send starts a new running turn and produces a later settled result.
- Preserve lifecycle-only fences around settlement: work generation, descendant waiting and subtree revision, direct-parent routing, terminal queue admission and retry, and `lastWriter` / `threadBound` ownership. A parent settlement cannot bypass outstanding descendants or substitute a stale pre-child result for fresh synthesis.
- Add a per-generation settlement-admission latch so duplicate ordinary `agent_settled` events cannot enqueue duplicate lead messages or owner notifications.
- Owner-held settled output routes to the owner rather than the lead. Generic Finish starts a fresh lead-owned handoff; it does not replay the old owner-held result as a new lead result.
- Retain current best-effort transcript recovery across shutdown or reload. This ticket does not add a durable exactly-once terminal outbox.
- Keep `ws-fork(expects_commit: true)` as a prompt-level and lead-visible expectation. The lead judges whether the settled output satisfies it; the adapter does not enforce it with a final-output parser.
- Execution, terminal delivery, descendant waiting, ownership, and lead action-required state must not be represented by one `running` count.

## Constraints

- Clear a terminal-delivery obligation only after the settled result has been admitted to the direct parent's delivery queue; a held or failed delivery remains recoverable without duplicate delivery.
- Preserve generation fences, nested-child completion protection, owner-held/thread-bound routing, exit handling, and resumability while retiring final-tool-specific state.
- Do not add an adapter parser, automatic adequacy inference, or automatic retry for final prose. The lead owns result judgment and follow-up.
- A settled agent may auto-park after its terminal result is admitted. A later `ws-agent-send` resumes it through the existing persistent-session path.
- `waitingOnChildren` remains distinct from a running agent turn. Goal-loop yielding follows work that can still make autonomous progress, not an already delivered settled result or a lead action-required condition.
- Caller-visible wording distinguishes actual execution from pending delivery or lead action required.
- Preserve prompt-level final-output templates and `expects_commit` intent without treating either as an adapter-enforced terminal protocol.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/fork.ts, agents-plugin-pi/src/ask.ts, agents-plugin-pi/src/subtree-lifecycle.ts |
| scope.surface | public-interface | ws-agent-list, pushed lifecycle status, and ws-report-to-lead are caller-visible adapter surfaces |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | no new exported type or signature is required; lifecycle fields remain adapter-internal |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts, agents-plugin-pi/test/fork.test.ts, agents-plugin-pi/test/recursive-worker.test.ts, agents-plugin-pi/test/agent-widget.test.ts |
| complexity.reuse_points | confirmed | RpcAgentRecord workGeneration and TerminalDelivery in agents-plugin-pi/src/spawner.ts; subtree state in agents-plugin-pi/src/subtree-lifecycle.ts |
| complexity.side_effect_risk | high | terminal delivery, parking, direct-parent routing, and owner-held state interact across turns |
| risk.correctness | high | duplicate or stale settlement can misroute output or leave descendant work incomplete |
| risk.fit | high | the settled-output contract replaces final-tool-specific behavior across worker and fork paths |
| risk.test | high | lifecycle races, retries, resumed sessions, and nested children need regression coverage |
| risk.security_or_contract | moderate | changing child terminal semantics changes the caller-visible report contract without adding authority |

## Phases

### Phase 1: Make settled output the universal terminal channel

Route every role's final output through the idle-settle path after successful direct-parent queue admission. Retire `kind: "final"`, `pendingFinal`, and universal `expectedReport` completion bookkeeping; preserve progress and question reports as mid-turn signals. Keep structured worker and fork output shapes in prompts while leaving adequacy judgment and same-agent follow-up to the lead.

Apply generation and subtree fences before admitting a settled result, coalesce duplicate settle events per work generation, and route owner-held results to the owner. Align the live widget, `ws-agent-list`, pushed status line, and goal-loop yield predicate around actual autonomous execution. After terminal admission, park eligible agents without losing their resumable session record.

Add regression coverage for:

1. exploration agents settling with rich or short ordinary results: one delivery, zero running agents, automatic parking, and explicit lead follow-up remaining possible;
2. ticket workers and forks settling with complete, malformed, or missing requested output sections: settled output remains preserved, no adapter parser determines completion, and a lead nudge starts the next running generation;
3. `expects_commit: true` remaining visible in the fork prompt and lead context without adapter parsing or false-running state;
4. held or failed terminal delivery remaining pending but not labeled as execution, with retry producing no duplicate;
5. duplicate ordinary settle events producing one terminal delivery or owner notification per generation;
6. nested children, stale pre-child settlement, owner-held results, generic Finish handoff, exited/stopped agents, replacement-work races, and resumed dormant agents retaining ordering and routing guarantees;
7. the widget, `ws-agent-list`, pushed status line, and goal-loop yield predicate agreeing about actual autonomous work.

### Result (4f6ecbda) - 2026-09-13

- Native `agent_settled` now ends autonomous execution for every role and delivers the generation-scoped ordinary assistant result exactly once; progress and question reports remain intermediate only.
- Execution fan-in, descendant waiting, pending delivery, and owner/thread holds now have distinct lifecycle accounting and caller-visible states. Eligible agents park only after direct-parent queue admission and remain resumable.
- Worker and fork completion no longer depends on final-report parsing. Structured output and `expects_commit` remain prompt-level lead expectations, while owner-held output and generic Finish retain distinct routes.
- Generation fences now use the consumed queued-user `message_start` boundary for accepted steer/follow-up instructions. Assistant `message_end` supplies current-generation provenance, so empty, aborted, missing, replacement, and pre-queue output cannot borrow a stale prior answer.
- Verification: `cd agents-plugin-pi && npm test` passed 1,649 tests with 2 expected skips, including all six fork lifecycle provider/version combinations; focused recursive lifecycle tests passed 15/15.
- Independent review: round one was clean for Fit and found two Important Correctness plus two Important Test issues, all addressed in `d8e56421`; round two was clean for Fit and Test and found one remaining Important queued-generation race, addressed in `4f6ecbda`. No Critical finding remains; the fixed two-round cap prohibited a third sweep.


## Resolution (2026-09-13)

Implemented native settled-result delivery with execution-only fan-in, generation-scoped terminal text, retryable direct-parent and owner delivery, distinct waiting/delivery states, and preserved resumability. The full Pi suite and both independent review rounds completed; no Critical finding remains.
