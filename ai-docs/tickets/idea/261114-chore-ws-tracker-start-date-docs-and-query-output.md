---
title: "Document the tracker item start date in tracker.md and print it in tracker_query text output"
related:
  261113-feat-ws-tracker-start-date-range-picker: "added the optional item start date this ticket documents"
---

# Document the tracker start date and print it in query output

## Background

`261113-feat-ws-tracker-start-date-range-picker` (merged 7ee0abb10) added an
optional item `start` date: a `start` attribute on `tk:item.create` and
`tk:item.edit`, `clear="start"`, a write-time refusal when start is after due,
and `detail.start` on tracker hook events. Its worker reported two gaps outside
the ticket's touch points:

- The shipped tracker reference `agents-plugin/rsrc/tracker/tracker.md` and its
  wsflow mirror do not list `start`: it is missing from the create/edit
  attribute tables, the `clear` values, and the hook event `detail` list.
  Agents therefore cannot learn the attribute from the shipped text.
- `tracker_query` text output (`agents-plugin-tool/internal/mcp/tracker_render.go`,
  `tracker_show.go`) prints `due` but not `start`.

## Notes

- Editing tracker.md needs `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (both packages).
- Compatibility: T0 (documentation and additive text output).
