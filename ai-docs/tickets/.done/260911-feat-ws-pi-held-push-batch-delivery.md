---
title: Batch accumulated Pi adapter push messages per wake
related:
  260911-bug-ws-pi-question-queue-dogfood: dogfood session that exposed one-at-a-time model delivery after a counted wake
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5ca251937373b77f
sage-review-completeness-reviewed: 5ca251937373b77f
completed: 2026-09-13
---

# Batch accumulated Pi adapter push messages per wake

## Background

The Pi adapter currently drains every record from its held push queue when a counted wake starts, but submits each record as a separate steering message. With Pi's default `one-at-a-time` steering mode, the model therefore receives one accumulated ws message per assistant turn even though the adapter flushed the queue together.

The owner has changed the local Pi steering mode to `all`, which is an adequate current workaround and makes this follow-up non-urgent. That setting is global, however: it also changes non-ws steering behavior and should not become an adapter-owned default merely to batch ws traffic.

## Decisions

- Flush held pushes at the lead's `agent_end` boundary, before full `agent_settled`, as one `ws-push-batch` custom message sent with `deliverAs: "followUp"` and `triggerTurn: true`. Retain the existing `agent_settled` wake as the fallback for pushes that arrive after the boundary snapshot or while compaction prevents release.
- Preserve strict arrival FIFO in the model-visible content. Encode one XML-escaped `<message>` element per item inside a versioned `<ws-push-batch>` root, retaining the exact ws custom type and stable agent, command, or question identity. Do not reorder actionable items ahead of earlier reports; mark them at their FIFO position and append an action summary after the ordered body.
- Include report, settled, advisory, orphaned, approval, and headless-question pushes in the same batch. TUI-consumed owner questions continue immediately through their modal/widget path and contribute only their existing lead advisory; batching must not duplicate the raw question into model context.
- Preserve the original structured items in `details.items` and render them as separate current-style TUI cards even though the model receives one XML batch. Pi does not expose `details` to the model, so every actionable fact also remains in the XML `content`.
- Revalidate approvals and questions at snapshot time. Retain stale controls in FIFO position marked `superseded`; never present them as actionable.
- Snapshot without removal, submit once, and remove exactly that snapshot only after synchronous acceptance. On failure, leave it queued and re-arm the existing fallback wake. Do not add a debounce window.
- Keep payloads uncapped in the initial implementation rather than truncating or splitting actionable content without an evidence-backed host limit. Crash-persistent outboxes and `/audit` visibility are separate follow-ups; this phase preserves current restart durability.
- Do not change or persist Pi's global steering mode. Existing individual custom types remain valid for immediate, non-held delivery and old transcript replay.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts#L1445-L1666, agents-plugin-pi/test/push-wake.test.ts#L46-L101 |
| scope.surface | public-interface | model-visible custom-message content and owner-visible push rendering gain a versioned ws-push-batch contract |
| scope.new_public_symbol | no | no exported TypeScript symbol is required; the new custom message type is a runtime protocol value |
| scope.new_type_contract | yes | ws-push-batch content and details.items define the XML model envelope and structured renderer input |
| scope.test_surface | existing | agents-plugin-pi/test/push-wake.test.ts#L74-L101 exercises confirmed-start FIFO steering delivery |
| complexity.reuse_points | confirmed | heldPushQueue and flushHeldPushes in agents-plugin-pi/src/spawner.ts#L1594-L1656 |
| complexity.side_effect_risk | high | batching changes when model-visible push reports arrive and how send failures are handled |
| risk.correctness | high | FIFO, stale-control validation, agent_end release, and mixed actionable/non-actionable delivery must remain atomic |
| risk.fit | high | one XML model message must coexist with separate current-style TUI cards and Pi's retry/compaction ordering |
| risk.test | high | verification must compare both Pi steering modes with mixed push families and oversized/send-failure cases |
| risk.security_or_contract | high | model-visible push delivery and per-item identity change without a settled batch contract |

## Phases

### Phase 1: Implement turn-boundary ws push batching

Add lead-side `agent_end` release that snapshots all currently held pushes and submits one versioned `ws-push-batch` follow-up. Serialize items as XML-escaped FIFO `<message>` elements in model-visible `content`, append the action summary only after the ordered body, preserve original structured payloads in `details.items`, and render each item as its current separate TUI card. Keep child-final settlement unchanged.

At snapshot time, revalidate approval and headless-question actionability and mark stale controls `superseded`. Commit queue removal only after synchronous batch acceptance; retain and re-arm fallback delivery on failure. Keep `agent_start` and `agent_settled` handling for idle-wake starts, late arrivals, and compaction deferral, without duplicate release or any global steering-mode mutation.

Verification covers both Pi steering modes; `agent_end` delivery before full settlement; mixed report, approval, headless question, advisory, final, and owner-summary payloads; strict FIFO model content plus trailing action summary; per-item TUI rendering; XML escaping; stale controls; send failure; compaction; late arrivals; retry ordering; uncapped large content; and exactly one model continuation for one boundary snapshot. Owner-live acceptance confirms that intermediate child reports and actionable controls accumulated during a long lead turn arrive together at the next continuation while preserving the existing owner-facing cards.

### Result (08c3eba) - 2026-09-13

Implemented one versioned `ws-push-batch` envelope per held FIFO snapshot, with uncapped XML-safe model content, a trailing actionable-control summary, snapshot-time supersession checks, and original structured items for separate TUI cards. Lead `agent_end` is the primary follow-up boundary; confirmed starts and the existing settled/countable-wake path provide fallback delivery after compaction, late arrival, or synchronous rejection. Queue removal and terminal-obligation transfer occur only after synchronous send acceptance.

Round-1 review identified and fixed one FIFO defect: an actionable steer arriving behind an older held follow-up now joins that batch instead of overtaking it. The same fix pass centralized the internal batch wire contract and added transport-level assertions that standalone report, approval, and question steers retain their individual custom types. Correctness, fit, and test re-review all passed with no remaining findings.

Verification: the focused affected suite passed 852/852 tests, and the full package command (`npm test`, which unsets all four `WS_PI_*` role/policy/channel variables) passed 1769 tests with 2 intentional skips and no failures. The automated transport harness covers both Pi steering modes and the long-turn mixed actionable/report case; interactive owner-live acceptance was not available in the worker session.

Decisions: retained immediate individual custom messages only when no older held prefix exists; made the batch protocol shared within the adapter without exposing it as a package API; preserved visual-only TUI collapse while leaving model content uncapped. Captured the review findings-path authority mismatch separately as `260913-bug-ws-reviewer-cannot-write-findings-path`.


## Resolution (2026-09-13)

Implemented and reviewed one FIFO `ws-push-batch` per held lead-turn snapshot, including XML-safe model content, structured TUI cards, snapshot-time actionability validation, atomic retry behavior, and fallback delivery. Full `agents-plugin-pi` test suite passed (1769 pass, 2 skipped, 0 fail).
