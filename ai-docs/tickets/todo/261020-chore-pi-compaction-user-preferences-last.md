---
title: Move the user-preferences section to the bottom of the Pi lead compaction summary
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

Last ID: (1)

(1) [open] [technical] Will move `user_preferences` to the last entry of `LEAD_PROSE_SECTIONS` (still before `## Resume`) over placing it after `## Resume` - `extractLeadProse` locates the prose by searching back from `## Resume`, so the prose block plus `## Resume` must stay the summary's last two sections; anything after `## Resume` would break prose carry-over between compactions.

## Phases

### Phase 1: Reorder the prose sections

Reorder `LEAD_PROSE_SECTIONS` per the settled placement. The order also drives
the lever schema (`leadProseParameterSchema`) and the preparation headings
list, so both follow automatically.

Verification: update any test pinning the heading order; `npm test` in
`agents-plugin-pi` passes; a rendered summary shows the user-preferences
heading as the last prose heading.
