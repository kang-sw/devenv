---
title: Move the user-preferences section to the bottom of the Pi lead compaction summary
dropped: 2026-10-20
---

# Move the user-preferences section to the bottom of the Pi lead compaction summary

## Background

After a lead compaction, the Pi adapter injects a summary built by
`buildLeadCompactionSummary` in `agents-plugin-pi/src/lead-compaction.ts`. The
lead's prose sits under fixed headings from `LEAD_PROSE_SECTIONS`, and
"User preferences and style" (`user_preferences`) is currently the first of
them, so it lands mid-summary, far from where the resumed lead reads last.
The user wants that section moved to the bottom.

## Open Decision Queue

Last ID: (9)

(1) [rejected] [technical] Will move `user_preferences` to the last entry of `LEAD_PROSE_SECTIONS` - capture error: the user asked to move `## Dialog`, not the user-preferences prose heading.

(6) [open] [policy] Reorder the Pi lead compaction summary to intro -> `## Session` -> `## Carried forward by the lead` -> child-agent sections -> `## Background tasks running` -> file-list sections -> `## Dialog` -> `## Resume`.
- Context (user intent): agents weight earlier prompt text as more important; `## Dialog` is reference data akin to tool results, so placing it before the lead's distilled prose is a minor negative. Today the order is Session -> child agents -> background -> Dialog -> file lists -> prose -> Resume (`src/lead-compaction.ts#L831-L844`).
- Alternative: move only `## Dialog` below the prose (file lists keep preceding the prose). Inlined re-reads are the same kind of reference data, and the prose -> Dialog -> Resume tail cannot be parsed by today's backward search because Dialog may quote headings verbatim.
- Alternative: Dialog directly above the prose (the withdrawn earlier proposal); keeps Dialog ahead of the prose, against the intent.
> Recommend the full reorder: the prose sits right after the session facts, every reference-data section (inlined files, Dialog) follows it, and `## Resume` stays last as the action instruction.

(7) [open] [technical] Will drop this ticket and capture the corrected scope as a new ticket `261020-chore-pi-compaction-prose-before-dialog` over renaming this stem in place - the capture commit `fabdfa073` names this stem, so a drop keeps the misread capture traceable while the new stem describes the real change.

(8) [open] [technical] Will make `extractLeadProse` search forward under the new layout - prose starts at the prose heading that directly follows the `## Session` section and ends at the always-rendered `## Child agents in flight` heading - over keeping the backward search from `## Resume` - with Dialog and inlined files after the prose, a backward search would land inside their verbatim quotes, while the sections before the prose are generated text that cannot quote the heading.

(9) [open] [technical] Will keep today's backward extraction as the fallback when the prose heading does not directly follow `## Session` (a summary written before this change) over handling only the new layout - the first compaction after upgrading reads an old-layout summary, and a forward search there could start inside a Dialog quote and carry garbage as prose.

## Phases

### Phase 1: Reorder the prose sections

Reorder `LEAD_PROSE_SECTIONS` per the settled placement. The order also drives
the lever schema (`leadProseParameterSchema`) and the preparation headings
list, so both follow automatically.

Verification: update any test pinning the heading order; `npm test` in
`agents-plugin-pi` passes; a rendered summary shows the user-preferences
heading as the last prose heading.


## Resolution (2026-10-20)

Capture error: the user asked to move the Pi compaction summary's `## Dialog` below the lead prose, not the `user_preferences` prose heading. Recaptured as 261020-chore-pi-compaction-prose-before-dialog.
