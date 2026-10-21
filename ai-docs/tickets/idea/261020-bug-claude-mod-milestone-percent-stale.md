---
title: Claude mod compaction milestone shows a context percentage well below live usage
related:
  261017-feat-claude-mod-lead-compaction-timing: origin of the advisory and milestone messages and their measurement
  261020-feat-claude-mod-lever-continue-flag: sibling; also edits compaction.ts and timing.ts
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1b3f627c103c824c
sage-review-completeness-reviewed: 1b3f627c103c824c
---

# Claude mod compaction milestone shows a context percentage well below live usage

## Background

The Claude mod's advisory and milestone messages (`timing.ts`
`advisoryText` / `milestoneText`) show context usage as
`N% / hard%`. The user observes that when the message appears, real usage is
already around 70-80% while the message shows a much lower figure, as if
measured at an earlier point.

Today's measurement (`compaction.ts` `measure`, `register.ts`
`session.measure` hook): the figure is `context.tokens` from the
`session.measure` event, pushed after each main-thread turn, divided by the
compaction window `rawMaxTokens` from `$.session.usage({breakdown:
'summary'})`. The text is rendered at that moment and queued with
`$.prompt.submit`; it is never re-read when the prompt is delivered.
Delivery is also withheld while a lever request or held continuation exists
and fires on a later measure.

Candidate causes, none yet verified: the text is delivered after other
queued turns (sample time); `context.tokens` lags live usage (definition);
`rawMaxTokens` differs from the denominator Claude Code itself displays
(denominator). The Claude Code mod typings are not on this machine, so the
definition of `context.tokens` is unconfirmed.

The user wants the message to show the exact usage at the moment it is
delivered.

Leading hypothesis (2026-10-21, unverified; demoted to `idea/` on it): the
denominator. The mod's percent is of the compaction window, while Claude
Code's "Context left until auto-compact" indicator is relative to the
automatic compaction point, so it reads higher at the same moment (with the
hard point at 89%, a mod figure of 65% reads as about 73% used there). If
the user's 70-80% came from that indicator, the gap is two scales, not a
stale sample. A probe should record which figure the user read (`/context`
or the status-line indicator). Since `261018-feat-claude-mod-compaction-threshold-knob`,
a low `claude.compaction_advisory_percent` (e.g. `10%`) brings the
milestones down to roughly 36% and 63% for a cheap live repro.

## Decisions

- **Show live usage.** The figure shown is the usage at delivery time, not a
  snapshot from an earlier measure.
- **Show absolute tokens.** Both the advisory and the milestone show used
  and window token counts alongside the percentage, as Pi's advisory does (Pi shows threshold token counts, not the
  current usage: `agents-plugin-pi/src/lead-compaction.ts#L916-L919`,
  `#L933`, `#L946`; the used-token count is new here).
- **Probe first.** Phase 1 records, at a milestone delivery, the shown
  figure, a fresh `$.session.usage()` read at that moment, and Claude Code's
  own `/context` figure, and names which candidate cause explains the gap.
  Rejected: changing the sample point blind (a fresh read fixes only the
  sample-time cause).
- **Fix scope.** For the sample-time cause, re-render the mod's own advisory
  or milestone prompt with a fresh `$.session.usage()` read in the
  `prompt.submit` hook as it enters. When the probe shows a definition or
  denominator cause, or that the hook does not see a mod-submitted prompt,
  the worker stops with the probe evidence (Phase 2 then lands nothing, the
  token-count head included). The `$.session.usage()` fields used for the
  used and window counts follow the probe's finding. If the
  continue-flag ticket has removed the composer `claim` hook, register a
  `prompt.submit` hook for the mod's own prompts only; do not revive the
  claim branch. Rejected:
  pre-deciding those fixes (they change what the figure means, which is the
  user's call).
- **Head format.** `Context window: <n>% (<used>k / <window>k tokens) /
  <hard>% (automatic compaction point).`, following Pi's advisory.
- **Own ticket.** Split from `261020-feat-claude-mod-lever-continue-flag`
  because this fix depends on the probe outcome.
- **wsflow mirror.** Every `agents-plugin/hooks/claude-mod/` edit is mirrored
  into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`).

## Constraints

- Pi (`agents-plugin-pi`) is out of scope.
- Runs concurrently with `261020-feat-claude-mod-lever-continue-flag`, which
  edits the same `timing.ts` texts and removes the `register.ts` `prompt.submit`
  claim hook; implement against this ticket alone, and the lead resolves the
  overlap at merge.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "Current usage is `context.tokens`, and every percentage in the [message is measured against the compaction window]" — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "Milestone points sit at one and two thirds of the way from the advisory to the native auto-compact point, `autoCompactThreshold`. The breakdown is read at session start ... and cached in between." — bearing: constrains
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "Milestone message (`{n}` the rounded current usage, `{hard}` the resolved hard threshold)" — bearing: supports
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, 05ae9999): "The compaction and reconnect continuation submits are unchanged (out of scope)." — bearing: constrains
- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, 1ffd64e1): "The lever's landing bypasses the mod's session.compact hook, so it reaches the manual feature through the new leverLanded edge." — bearing: constrains
- 1ed6398b (2026-10-18, commit): "if the session.end time bound cut the awaited delete short, compaction.end never ran and a pending request or latch carried into the next conversation" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/{timing.ts,compaction.ts,register.ts,compaction.test.ts,compaction-engine.test.ts} and the byte-identical agents-plugin-wsflow copies |
| scope.surface | internal | no exported package surface; timing.ts exports advisoryText/milestoneText/deliveryText are mod-internal, but the model-facing prose head changes |
| scope.new_public_symbol | no | none named; a usage-reading helper is not exported public surface |
| scope.new_type_contract | unknown | delivery-time render may change the deliveryText/milestoneText signatures (used tokens argument); depends on the Phase 1 probe |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/compaction.test.ts, compaction-engine.test.ts |
| complexity.reuse_points | confirmed | timing.ts advisoryText/milestoneText/percentOf and the $.session.usage read already used by session.measure in register.ts#L719-L726 |
| complexity.side_effect_risk | moderate | prompt.submit hook (register.ts#L614) already rewrites composer prompts and claims held continuations; re-rendering mod-submitted prompts adds a path beside it |
| risk.correctness | moderate | root cause unverified; Phase 1 probe must separate sample time, context.tokens definition, and rawMaxTokens denominator |
| risk.fit | moderate | Claude Code mod typings are not on this machine, so whether prompt.submit sees a mod-submitted prompt is unconfirmed; the mod host contract is only probed live |
| risk.test | moderate | needs a test where usage rises between measure and delivery; Phase 1 is a live-session probe that tests cannot cover |
| risk.security_or_contract | low | no auth, input, or external-contract change; prose and figure only |

## Phases

### Phase 1: Probe the gap

Add temporary probe instrumentation that logs, at each advisory or milestone
prompt the mod submits and at `prompt.submit` entry, the shown figure, a fresh
`$.session.usage()` read, and a timestamp; then stop and report to the lead
the steps for the user to run a live Claude Code session with the mod loaded
past a milestone and read `/context` at delivery. With the user's figures,
record the findings in the Phase 1 Result, including whether the
`prompt.submit` hook sees a mod-submitted prompt as it enters, and remove the
instrumentation. Verification: the Result names the cause
with the recorded figures.

### Phase 2: Render the figure at delivery

Proceed only when the Phase 1 Result names the sample-time cause and confirms
the hook sees mod-submitted prompts; otherwise stop with that evidence.
Implement the fix scope and head format per Decisions; update `compaction.test.ts` and
`compaction-engine.test.ts` in both copies. Verification: a test where usage
rises between measure and delivery shows the delivery-time figure and token
counts; the claude-mod test suite passes in both copies.
