# Plan: 260905-chore-ws-pi-goal-announcement-wording — Phase 1: Reword the announcement and the lever result

## Relevant Ticket Contract

- Decision 1: the `/goal` announcement reads `Goal armed: <goal>`; one word
  changes, the reminder and every other goal-loop string stay as they are.
- Decision 2: `goal-compact-and-continue`'s returned text becomes
  `Compaction requested; the conversation will resume from a summary
  carrying: <carry_forward>`. No behavior change.
- Constraint: exactly two strings change.
- Tests: update the existing `buildGoalAnnouncement` assertion; add an
  assertion that the lever's returned text starts with `Compaction
  requested` and carries the `carry_forward` argument verbatim.
- Spec Impact: amend the goal-loop anchor where the announcement is quoted;
  add the lever's result text to the compaction sub-anchor.
- Live check is owner-run (`/goal` a trivial goal); out of band.

## Codebase Findings

- `agents-plugin-pi/src/goal-loop.ts:160-162` — `buildGoalAnnouncement`
  returns `` `Goal settled: ${goal}` ``; called once at `:382` via
  `pi.sendUserMessage`. Change point 1.
- `agents-plugin-pi/src/goal-loop.ts:501-526` — the
  `goal-compact-and-continue` tool; `:526` returns
  `` `Compacting and continuing goal with carry-forward: ${p.carry_forward}` ``.
  Change point 2. `ctx.compact({ customInstructions: p.carry_forward })` at
  `:521-522` stays as is.
- `agents-plugin-pi/test/goal-loop.test.ts:220-222` —
  `describe("buildGoalAnnouncement")` asserts the old string; update.
  No existing test exercises the lever's returned text; add one that
  invokes the tool's `execute` (or the smallest exported helper, if the
  result text is extracted into one) with a stub `ctx` whose `compact` is a
  no-op recorder, asserting `text.startsWith("Compaction requested")` and
  `text.includes(carryForward)`.
- `ai-docs/spec/pi-adapter-runtime.md:1032` — quotes `Goal settled: <goal>`
  in the goal-loop anchor; reword to `Goal armed: <goal>`.
- `ai-docs/spec/pi-adapter-runtime.md:1079-1082` — the lever sub-anchor
  describes the `ctx.compact` call but not the returned text; add one
  sentence naming the returned text so the model's transcript evidence is
  specified.
- Doc comments at `goal-loop.ts:60-61` and `:360` describe the lever; check
  whether either quotes the old result text and update only if so (comment
  prose does not count against the "two strings" constraint).

## Implementation Plan

1. `goal-loop.ts:161`: `Goal settled:` → `Goal armed:`.
2. `goal-loop.ts:526`: return text →
   `Compaction requested; the conversation will resume from a summary carrying: ${p.carry_forward}`.
   If exercising `execute` from a test needs an unreasonable stub, extract
   the text into a small exported pure helper (e.g.
   `buildCompactionLeverResult(carryForward)`) and call it from `:526`.
3. `test/goal-loop.test.ts:222`: update the expected string; add the lever
   result assertion per Codebase Findings.
4. Spec: the two passages above; anchor ids unchanged.

## Verification Plan

- `cd agents-plugin-pi && npm test`.
- `grep -rn "Goal settled\|Compacting and continuing" agents-plugin-pi/src agents-plugin-pi/test ai-docs/spec` returns nothing.
- Owner-run: `/goal` a trivial goal and confirm the lead does not read the
  announcement as completion.

## Escalations

- None.
