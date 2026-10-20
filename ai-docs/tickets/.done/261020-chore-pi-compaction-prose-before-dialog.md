---
title: Put the lead prose before the reference-data sections in the Pi lead compaction summary
related:
  261020-chore-pi-compaction-user-preferences-last: dropped misread capture of this request (it moved the user-preferences prose heading instead of the Dialog)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 26592f881e820a32
sage-review-completeness-reviewed: 26592f881e820a32
completed: 2026-10-20
---

# Put the lead prose before the reference-data sections in the Pi lead compaction summary

## Background

After a lead compaction, the Pi adapter injects a summary built by
`buildLeadCompactionSummary` in `agents-plugin-pi/src/lead-compaction.ts`.
Today its order is intro -> `## Session` -> child-agent sections ->
`## Background tasks running` (when any) -> `## Dialog` -> file-list sections
(`## Required re-reads (inlined)`, `(to read first)`, `## On-demand
references`) -> `## Carried forward by the lead` -> `## Resume`
(`src/lead-compaction.ts#L831-L844`).

Agents weight earlier prompt text as more important. `## Dialog` and the
inlined re-reads are reference data, closer to tool results than to the
lead's distilled state, so placing them ahead of the lead's carried-forward
prose is a minor negative for the resumed lead. The user wants the prose
ahead of that reference data.

## Decisions

- **New order:** intro -> `## Session` -> `## Carried forward by the lead` ->
  child-agent sections -> `## Background tasks running` -> file-list
  sections -> `## Dialog` -> `## Resume`. The prose follows the session facts
  directly, every reference-data section follows it, and `## Resume` stays
  last as the action instruction. Rejected: moving only `## Dialog` below the
  prose (inlined re-reads are the same kind of reference data, and a
  prose -> Dialog -> Resume tail cannot be parsed by the backward search
  because Dialog may quote headings verbatim); Dialog directly above the
  prose (still ahead of the prose, against the intent); moving the
  `user_preferences` prose heading (the original misread capture).
- **Forward extraction.** `extractLeadProse` (used by the in-hook fallback
  compaction to hand the previous compaction's prose to the summary model as
  `<previous-prose>`, `src/goal-loop.ts#L1679`) searches forward under the new
  layout. "The prose heading" below means `LEAD_PROSE_SECTION_HEADING`
  (`## Carried forward by the lead`); the `LEAD_PROSE_SECTIONS` subsections
  live inside the prose and are not separate anchors. Layout detection: the
  first `\n\n## ` heading after the `## Session` heading is the prose
  heading. When it is, the prose starts after that heading and ends at the
  first `\n\n## Child agents in flight\n` after the start (blank line before
  it, matching how the `## Resume` anchor is matched today). Rejected: keeping
  the backward search from `## Resume`
  (with Dialog and inlined files after the prose it would land inside their
  verbatim quotes, while the sections before the prose are generated text
  that cannot quote the heading).
- **Old-layout fallback.** When the first heading after `## Session` is
  anything else (an old-layout summary puts `## Child agents in flight`
  there), or the summary has no `## Session` heading, extraction keeps
  today's backward search unchanged. Rejected: handling only the new layout (the first
  compaction after upgrading reads an old-layout summary, and a forward
  search there could start inside a Dialog quote and carry it as prose).

## Constraints

- `LEAD_PROSE_SECTIONS` order is unchanged.

## Prior Decisions

- 261020-chore-pi-compaction-user-preferences-last (2026-10-20, c8021b98): "Settled order: Session -> prose -> child agents -> background tasks -> file lists -> Dialog -> Resume. Rejected Dialog-only move (prose->Dialog->Resume tail is unparseable by the backward search since Dialog quotes headings)" — bearing: supports
- 261020-chore-pi-compaction-user-preferences-last (2026-10-20, c8021b98): "extractLeadProse (fallback-compaction previous-prose carry, goal-loop.ts:1679) switches to a forward search bounded by Session and `## Child agents in flight`, keeping the backward search for old-layout summaries during upgrade." — bearing: supports
- 261003-feat-pi-lead-compaction-dialog-transcript (4e7e8ad5, commit): "dialog text now precedes the prose heading and can quote it, so extraction anchors on the last `## Resume` and the last prose heading before it." — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, Decisions): "The section header reads, for example, \"The newest 42 of 57 dialog items (older ones are carried by the prose below).\"" — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, Result): "`extractLeadProse` now searches from the end, so a dialog message that quotes a previous summary is not taken for the prose." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (94cdf8bd, commit): "The prose is also passed as customInstructions so a degraded native compaction (summary-build failure) is still steered by it." — bearing: constrains
- 261016-feat-pi-compaction-token-thresholds (2026-10-17, Result): "Preserved retain-none context, existing summary/dialog assembly and all compaction trigger/resume control." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/test/lead-compaction.test.ts, src/goal-loop.ts only read as the extractLeadProse caller |
| scope.surface | internal | extractLeadProse is exported from lead-compaction.ts but its signature is unchanged; only summary section order changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/lead-compaction.test.ts (order pins at L486 and L710; further pins at L357, L431-L447, L592-L595); new cases land in the same file |
| complexity.reuse_points | confirmed | buildLeadCompactionSummary sections array and extractLeadProse in lead-compaction.ts#L831-L860 read in full |
| complexity.side_effect_risk | moderate | the fallback compaction in goal-loop.ts#L1679 feeds extractLeadProse of the stored previous summary, which may be old-layout, and the Dialog quote can contain the prose and Resume headings |
| risk.correctness | moderate | forward extraction must stay correct for both layouts and when Dialog quotes headings; a wrong boundary carries Dialog text as prose |
| risk.fit | low | single-function reorder following the ticket's settled Decisions and existing section constants |
| risk.test | moderate | several existing assertions pin the old order or the "prose below" wording and must be updated alongside new quoting and old-layout cases |
| risk.security_or_contract | low | summary text layout only; no tool, config, or protocol surface changes |

## Phases

### Phase 1: Reorder the summary and switch prose extraction

Reorder the `sections` array in `buildLeadCompactionSummary` per Decisions;
update the comment that says the file lists sit "Before the prose heading"
and the Dialog note "older ones are carried by the prose below" (now
"above") so they match the new layout. Change `extractLeadProse` to the
forward search with the old-layout fallback, and update its doc comment.

Verification: update the tests pinning the old order or wording
(`test/lead-compaction.test.ts#L357` for the "prose below" text, `#L431-L447`,
`#L486`, `#L592-L595` for the Dialog-quoting extraction case, and `#L710`);
add tests that the prose
heading directly follows `## Session`, that `## Dialog` and the file-list
sections follow the prose, that `## Resume` stays last, that
`extractLeadProse` returns exactly the prose for a new-layout summary whose
Dialog quotes both `## Carried forward by the lead` and `## Resume`, and that
it still extracts the prose from an old-layout summary. `npm test` in
`agents-plugin-pi` passes.

### Result (0324cbb55) - 2026-10-20

Landed the reorder in `buildLeadCompactionSummary` (intro, Session, prose,
child-agent sections, background tasks, file lists, Dialog, Resume) and the
forward `extractLeadProse` with the old-layout fallback; the Dialog header now
reads "carried by the prose above". Tests: updated the order/wording pins in
`test/lead-compaction.test.ts` and `test/goal-loop.test.ts` (the file-list
sections are now followed by `## Dialog`); added cases for the full heading
order with Resume last, a Dialog quoting the prose, child-agent, and Resume
headings, and an old-layout summary.

Verification: `node --test test/lead-compaction.test.ts test/goal-loop.test.ts`
276 pass, 0 fail. Full `npm test` in `agents-plugin-pi` shows 31 failures
(child-spawn, web-search, and socket tests) identical with and without the
change (baseline 2301 pass / 31 fail; after 2304 pass / 31 fail); this worktree
needed temporary node_modules symlinks to run at all. `lite` review: clean.

Decisions: a new-layout match with no `## Child agents in flight` terminator
falls through to the backward search (defensive, untested). Minor reviewer
note: prose containing a literal `\n\n## Child agents in flight\n` would
truncate extraction, the same risk class as the old Resume anchor.
