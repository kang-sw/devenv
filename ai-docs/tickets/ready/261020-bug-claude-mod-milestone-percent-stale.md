---
title: Claude mod compaction milestone shows a context percentage well below live usage
related:
  261017-feat-claude-mod-lead-compaction-timing: origin of the advisory and milestone messages and their measurement
  261020-feat-claude-mod-lever-continue-flag: sibling; also edits compaction.ts and timing.ts
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 134e37318eff9222
sage-review-completeness-reviewed: 134e37318eff9222
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
`session.measure` event, pushed after each main-thread turn and when a
rate-limit window moves a whole point (`index.d.ts#L4375-L4376`), divided by the
compaction window `rawMaxTokens` from `$.session.usage({breakdown:
'summary'})`. The text is rendered at that moment and queued with
`$.prompt.submit`; it is never re-read when the prompt is delivered.
Delivery is also withheld while a lever request or held continuation exists
and fires on a later measure.

Candidate causes, none yet verified: the text is delivered after other
queued turns (sample time); `context.tokens` lags live usage (definition);
`rawMaxTokens` differs from the denominator Claude Code itself displays
(denominator). The Claude Code mod typings are on this machine, and they
define `context.tokens` as the last API response's input tokens (uncached,
cache-written and cache-read together; the status line's `total_input_tokens`),
measured against `context.window`, while the breakdown measures against
`rawMaxTokens`, which may be smaller
(`~/.claude/dev-mods/<id>/ws-mailbox-push-probe/.claude-plugin/types/claude-code/index.d.ts#L10880-L10915`).

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

Verified sample-time cause (2026-10-23, read from the engine declarations at
`~/.claude/dev-mods/<id>/ws-mailbox-push-probe/.claude-plugin/types/claude-code/index.d.ts`,
dated 2026-10-16; the typings are on this machine after all):
`session.measure` fires "after each main-thread turn, and when a rate-limit
window moves a whole point", so it also fires mid-turn, including from a
subagent's requests sharing the account's limits. The mod's `session.measure`
hook calls `compaction.measure` without checking whether a main-thread turn
runs (the mod already tracks one in `mainTurn`), and a crossing renders its
text at once and calls `$.prompt.submit`, which runs "a turn of its own, once
the session is idle". The rest of the running turn (tool results, subagent
results, mailbox steer rows) then grows the context before the advisory
reaches the model. The design assumed the measure fires only after a turn
(`261017-feat-claude-mod-lead-compaction-timing`). A `prompt.submit` hook
runs when a prompt is submitted or queued, not when it is dequeued, so
re-rendering there would still freeze the figure mid-turn. The denominator
hypothesis above is independent of this cause and may add to the gap.

## Decisions

- **Show usage read at idle.** The figure shown is the usage read when the
  main thread went idle before delivery, not a snapshot from an earlier
  measure. Other queued prompts may still run before the advisory turn, so
  the head names its measurement point.
- **Show absolute tokens.** Both the advisory and the milestone show used
  and window token counts alongside the percentage, as Pi's advisory does (Pi shows threshold token counts, not the
  current usage: `agents-plugin-pi/src/lead-compaction.ts#L916-L919`,
  `#L933`, `#L946`; the used-token count is new here).
- **Fix scope.** A `session.measure` only records a threshold crossing in the
  latch; it delivers nothing while a main-thread turn runs. Delivery happens
  at whichever comes first of a measure arriving while no main-thread turn
  runs or a main-thread `turn.complete` that was not aborted (scheduled
  through `after`, as the lever step is), and reads `$.session.usage()` fresh
  at that moment for the figure and token counts. Rejected: re-rendering in a
  `prompt.submit` hook (it runs when the prompt is queued, not when it is
  delivered); delivering on `turn.complete` alone (the declarations do not
  state whether the end-of-turn measure arrives before or after it, and the
  first-of-both form is correct either way).
- **No probe gate.** The sample-time cause is verified from the engine
  declarations, so the fix lands without a live probe. Delivery writes one
  debug log line with the crossing's measured tokens and the fresh read, so a
  later denominator check has its figures. Rejected: keeping the probe as a
  gating phase (it now mostly answers the denominator question, and needs a
  live session the user drives); no debug line (leaves that check without
  data).
- **Head format.** `Context window at the end of the last turn: <n>%
  (<used>k / <window>k tokens) / <hard>% (automatic compaction point).`,
  following Pi's advisory and naming the measurement point.
- **Own ticket.** Split from `261020-feat-claude-mod-lever-continue-flag`
  because this fix depended on the cause.
- **wsflow mirror.** Every `agents-plugin/hooks/claude-mod/` edit is mirrored
  into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`).

## Constraints

- Pi (`agents-plugin-pi`) is out of scope.
- The denominator hypothesis (Background) is out of scope; the debug line
  only records figures for it.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "Thresholds are measured against the compaction window ... Current usage is `context.tokens`, and every percentage in the [message is measured against the compaction window]" — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "When a `session.measure` after a main-thread turn shows a crossing, the mod `$.prompt.submit`s that threshold's message, which starts its own turn once idle" — bearing: supports
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "Milestone points sit at one and two thirds of the way from the advisory to the native auto-compact point, `autoCompactThreshold`. The breakdown is read at session start ... and cached in between." — bearing: constrains
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "Milestone message (`{n}` the rounded current usage, `{hard}` the resolved hard threshold)" — bearing: supports
- a9ce2c08 (2026-10-21, commit, 261018-feat-claude-mod-compaction-threshold-knob): "Late read: settling invalidates the threshold cache ... instead of awaiting the read inside measure, so a slow or hung server never stalls observations; the latch is kept" — bearing: constrains
- 1bd9bf10 (2026-10-20, commit): "Guard implemented as \"turn.start cancels the held resume\" (compaction) and a main-thread turn counter compared against the step's start (reconnect)." — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, 05ae9999): "The compaction and reconnect continuation submits are unchanged (out of scope)." — bearing: constrains
- 1ed6398b (2026-10-18, commit): "if the session.end time bound cut the awaited delete short, compaction.end never ran and a pending request or latch carried into the next conversation" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/compaction.ts, timing.ts, register.ts, compaction.test.ts, compaction-engine.test.ts, and their byte-identical agents-plugin-wsflow/hooks/claude-mod/ copies |
| scope.surface | internal | CompactionFeature.measure and turnComplete in compaction.ts and advisoryText/milestoneText in timing.ts are mod-internal; only model-facing message text changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | unknown | measure may need a fresh-usage reader or delivery edge in its signature; the ticket does not fix the shape |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/compaction.test.ts, compaction-engine.test.ts and wsflow copies |
| complexity.reuse_points | confirmed | edges.after scheduling used by turnComplete in compaction.ts; mainTurn flag in register.ts; latch observe in timing.ts |
| complexity.side_effect_risk | moderate | delivery moves from measure to turn end and interacts with the pending lever request, held resume, and latch gating |
| risk.correctness | moderate | first-of-measure-or-turn.complete delivery must fire exactly once and respect request and held gating |
| risk.fit | moderate | changes when the 261017 advisory reaches the model and its pinned head text |
| risk.test | moderate | engine tests must model mid-turn measures and turn ends in both copies |
| risk.security_or_contract | low | no external API or permission surface; mod-internal message text only |

## Phases

### Phase 1: Deliver at idle with a fresh read

Implement the fix scope and head format per Decisions in `compaction.ts`,
`timing.ts`, and their `register.ts` wiring; update `compaction.test.ts` and
`compaction-engine.test.ts` in both copies. Verification: a test where a
measure crosses a threshold while a main-thread turn runs delivers nothing
until the turn ends, then delivers once with the fresh read's figure and
token counts; a crossing measured while idle delivers at once; the
claude-mod test suite passes in both copies.

Implementation notes from review:
- The head format applies to both messages: the advisory head
  (`advisoryText`) adopts the same `Context window at the end of the last
  turn: ...` line that replaces `milestoneText`'s current
  `Context window: <n>% / <hard>% ...` head (`timing.ts#L141`).
- At delivery, re-run the latch's level evaluation with the fresh read, so a
  crossing that grew past the next milestone delivers the higher level once
  rather than a stale lower one.
- The existing lever-request and held-continuation gating still applies at
  both new delivery points: a withheld delivery stays latched for a later
  measure, as today.
- The aborted-turn exclusion matters only when no idle measure follows; keep
  it, it costs one check.
- Verification also asserts the delivery debug line (through the `debug`
  edge in `compaction.ts`) carries the crossing's tokens and the fresh read.
