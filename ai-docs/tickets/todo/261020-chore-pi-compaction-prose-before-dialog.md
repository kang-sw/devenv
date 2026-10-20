---
title: Put the lead prose before the reference-data sections in the Pi lead compaction summary
related:
  261020-chore-pi-compaction-user-preferences-last: dropped misread capture of this request (it moved the user-preferences prose heading instead of the Dialog)
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
  layout: the prose starts at the prose heading that directly follows the
  `## Session` section and ends at the always-rendered `## Child agents in
  flight` heading. Rejected: keeping the backward search from `## Resume`
  (with Dialog and inlined files after the prose it would land inside their
  verbatim quotes, while the sections before the prose are generated text
  that cannot quote the heading).
- **Old-layout fallback.** When the prose heading does not directly follow
  `## Session` (a summary written before this change), extraction keeps
  today's backward search. Rejected: handling only the new layout (the first
  compaction after upgrading reads an old-layout summary, and a forward
  search there could start inside a Dialog quote and carry it as prose).

## Constraints

- `LEAD_PROSE_SECTIONS` order is unchanged.

## Phases

### Phase 1: Reorder the summary and switch prose extraction

Reorder the `sections` array in `buildLeadCompactionSummary` per Decisions;
update the comment that says the file lists sit "Before the prose heading"
and the Dialog note "older ones are carried by the prose below" (now
"above") so they match the new layout. Change `extractLeadProse` to the
forward search with the old-layout fallback, and update its doc comment.

Verification: update the tests pinning the old order
(`test/lead-compaction.test.ts#L486` and `#L710`); add tests that the prose
heading directly follows `## Session`, that `## Dialog` and the file-list
sections follow the prose, that `## Resume` stays last, that
`extractLeadProse` returns exactly the prose for a new-layout summary whose
Dialog quotes both `## Carried forward by the lead` and `## Resume`, and that
it still extracts the prose from an old-layout summary. `npm test` in
`agents-plugin-pi` passes.
