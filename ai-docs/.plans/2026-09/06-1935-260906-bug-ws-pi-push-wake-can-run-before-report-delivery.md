# Plan: 260906-bug-ws-pi-push-wake-can-run-before-report-delivery — Phase 1: Release the held batch as steering at confirmed start

## Relevant Ticket Contract
- At confirmed `agent_start`, release every held push as `steer` in FIFO order so the held batch precedes the woken run's first model response. The recorded `followUp`/`steer` value remains the busy-time admission rule only.
- Preserve the counted user-message wake, `triggerTurn: true`, custom message families/details/rendering, flush-time status rebuilding, shared wake reservation, and compaction behavior. Do not write Pi settings.
- Amend `pi-adapter-runtime` to describe confirmed-start steering release, including a user-started run that wins while the reservation is pending.
- Offline tests must model Pi's one-at-a-time drain ordering. Owner live checks remain pending: confirm the first request contains the report and one-report wake has one response; with two children, confirm reports precede follow-ups and responses do not exceed one per report.

## Out of Scope
- Changing busy-time admission: busy `followUp` still holds and busy `steer` still interrupts.
- Changing the wake text/count, embedding report payloads in the wake, setting Pi's global `steeringMode`, or changing goal-reminder/compaction/reservation rules.
- Treating fake-harness results as completion of the owner provider-context and simultaneous-settle live checks.

## Codebase Findings
- `agents-plugin-pi/src/spawner.ts#L1283-L1301` — `sendToLead` stores raw summaries in the shared FIFO with a closure that currently captures the recorded mode; its raw-send interface needs a per-flush override so `ws-thread-summary` follows the same confirmed-start rule as family pushes.
- `agents-plugin-pi/src/spawner.ts#L1352-L1457` — `HeldPush` records admission mode, `sendPush` rebuilds status/custom details and preserves `triggerTurn: true`, and `registerPushFlush` clears the reservation before `flushHeldPushes(pi, true)`; the one localized flush must pass `steer` to both family and raw entries without changing admission.
- `agents-plugin-pi/src/ask.ts#L903-L919` — discussion summaries enter the shared FIFO through `sendToLead(..., "followUp")`, so the raw override is required to cover every held message while retaining `followUp` at admission.
- `agents-plugin-pi/test/push-wake.test.ts#L8-L83` — the existing harness records sends and mode echoes but has no queue/dequeue model; it is the intended focused home for mode, FIFO, and first-request ordering coverage.
- `agents-plugin-pi/test/spawner.test.ts#L1497-L1723` and `agents-plugin-pi/test/ask.test.ts#L929-L998` — current confirmed-start assertions encode the old recorded-mode behavior and must be revised or their raw-send fixtures made override-aware; direct busy-time assertions must retain their existing modes.
- `ai-docs/spec/pi-adapter-runtime.md#L674-L702` — the runtime contract currently promises release with each original mode, which conflicts with this phase's settled confirmed-start steering policy.
- `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md#L1439-L1497` and `docs/settings.md#L205-L216` — Pi documents `steer` as the pre-next-response queue and `followUp` as post-completion delivery, with one-at-a-time defaults; this supports the test fake but does not replace the required owner live check.

## Implementation Plan
1. In `agents-plugin-pi/src/spawner.ts`, make the raw held-send closure accept an optional delivery-mode override, preserving its recorded mode for ordinary immediate sends. In the `confirmedStart` branch of `flushHeldPushes`, send every raw and family entry with `steer` while retaining FIFO iteration, `triggerTurn: true`, custom payloads, and release-time status construction; update comments to distinguish recorded admission mode from confirmed-start delivery mode.
2. In `agents-plugin-pi/test/push-wake.test.ts`, change the idle mixed-mode expectations so both confirmed-start sends are `{ deliverAs: "steer", triggerTurn: true }`, while preserving tests that prove a busy `followUp` is held and a busy `steer` interrupts. Extend the existing harness with a small one-at-a-time Pi drain model (steering before the first response and after each turn; follow-up only after the inner loop stops) and assert that mixed held entries are FIFO and visible before the first model response.
3. In `agents-plugin-pi/test/spawner.test.ts`, `agents-plugin-pi/test/ask.test.ts`, and any affected `agents-plugin-pi/test/goal-loop.test.ts` raw-send fixtures, update confirmed-start mode assertions and fixture closures to honor the override. Keep assertions for immediate/busy admission on their recorded modes so the phase does not broaden delivery policy.
4. In `ai-docs/spec/pi-adapter-runtime.md`, replace the original-mode confirmed-start wording with the settled policy: all held messages release as steering in arrival order before the first response; recorded mode governs busy-time admission only; the same behavior applies if a user-started run confirms the pending reservation.

## Verification Plan
- Run `cd agents-plugin-pi && npm test -- test/push-wake.test.ts test/spawner.test.ts test/ask.test.ts test/goal-loop.test.ts`, then the package's full `npm test` suite.
- Review the focused fake drain assertions for: one held `followUp` sends as `steer` with `triggerTurn: true`; held `steer` remains `steer`; busy-time modes are unchanged; a mixed batch is FIFO and precedes the first model response.
- Owner-live acceptance remains explicitly pending: run the one-child and simultaneous-two-child checks from the ticket in the shared predecessor-gate session; inspect provider requests/response counts and ws-block continuity rather than inferring them from offline tests.

## Escalations
- Resolved by owner on 2026-09-06 after relay `f7226a7`: the original plan's whole-batch-before-first-response assertions above are superseded. Pi's default one-at-a-time steering drain delivers the first held message before the initial response and later messages at subsequent steering polls. Acceptance is no blind initial response plus FIFO delivery, not batch coalescing. Global steering settings and payload embedding remain out of scope.
- Review disposition: finding 1 [fixed] fake fidelity in `f7226a7`, with its scope escalation resolved by the owner decision above; finding 2 [fixed] mixed raw/family FIFO in both orders; finding 3 [fixed] independent user-start with pending wake reservation. No Critical findings. Important findings received one relay, without re-review.
- Final offline verification: 509 focused and 935 full-suite tests passed with `WS_PI_SPAWN_ROLE` unset for tests, plus `npm pack --dry-run` and `git diff --check`. Live provider request/count and predecessor continuity checks remain pending.
