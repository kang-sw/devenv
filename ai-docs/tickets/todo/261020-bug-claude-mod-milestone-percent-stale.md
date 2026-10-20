---
title: Claude mod compaction milestone shows a context percentage well below live usage
related:
  261017-feat-claude-mod-lead-compaction-timing: origin of the advisory and milestone messages and their measurement
  261020-feat-claude-mod-lever-continue-flag: sibling; also edits compaction.ts and timing.ts
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

## Decisions

- **Show live usage.** The figure shown is the usage at delivery time, not a
  snapshot from an earlier measure.
- **Show absolute tokens.** The message shows token counts alongside the
  percentage, as Pi's advisory does.
- **Probe first.** Phase 1 records, at a milestone delivery, the shown
  figure, a fresh `$.session.usage()` read at that moment, and Claude Code's
  own `/context` figure, and names which candidate cause explains the gap.
  Rejected: changing the sample point blind (a fresh read fixes only the
  sample-time cause).
- **Fix scope.** For the sample-time cause, re-render the mod's own advisory
  or milestone prompt with a fresh `$.session.usage()` read in the
  `prompt.submit` hook as it enters. When the probe shows a definition or
  denominator cause, the worker stops with the probe evidence. Rejected:
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

## Phases

### Phase 1: Probe the gap

Run the probe per Decisions in a live Claude Code session with the mod loaded and
record the findings, including whether the `prompt.submit` hook sees a
mod-submitted prompt as it enters. Verification: the Result names the cause
with the recorded figures.

### Phase 2: Render the figure at delivery

Implement the fix scope and head format per Decisions; update `compaction.test.ts` and
`compaction-engine.test.ts` in both copies. Verification: a test where usage
rises between measure and delivery shows the delivery-time figure and token
counts; the claude-mod test suite passes in both copies.
