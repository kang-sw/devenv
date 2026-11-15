---
title: "Document the tracker item start date in tracker.md and print it in tracker_query text output"
related:
  261113-feat-ws-tracker-start-date-range-picker: "added the optional item start date this ticket documents"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dfa74cca6e935cc5
sage-review-completeness-reviewed: dfa74cca6e935cc5
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
- `tracker.query` text output uses `formatTrackerItemRow`,
  `formatTrackerAssignmentRow`, and `formatTrackerItem` in
  `agents-plugin-tool/internal/mcp/tracker_render.go`, which print `due` but not
  `start` (#L293-L337, #L438-L457); the adjacent
  `agents-plugin-tool/internal/mcp/tracker_show.go` renderer is instead
  `tracker.show`'s `formatTrackerCardFields`, also missing `start`
  (#L92-L123).

## Decisions

- MAIN's 2026-11-15 08:10:44Z triage explicitly assigns promotion and repair of the start-date documentation and text-output gaps, naming `tracker_show.go`; cover both query renderers and the existing show-card renderer.
- Compatibility: T0, documentation and additive text output only. Preserve the existing start-date validation, storage and tracker event semantics.

## Notes

- Editing tracker.md needs `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (both packages).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Current Facts

- Existing `start` storage and projection are already implemented: create stores it and edit sets/clears its register (`agents-plugin-tool/internal/wstracker/state.go#L1281-L1286`, #L1326-L1339); `ItemView.Start` is optional JSON (`agents-plugin-tool/internal/wstracker/view.go#L120-L121`, #L765). `clear="start"` already produces null (`agents-plugin-tool/internal/mcp/tracker_tools.go#L330-L358`).
- Date validation is the existing due validator, not new strict calendar validation: month 1–12 and day 1–31 (`agents-plugin-tool/internal/wstracker/payload.go#L437-L448`, `check.go#L168-L173`). The start-after-due refusal is write-time only, checks the unchanged side from the folded item, and permits a missing/cleared end; replay tolerates concurrent inverted pairs (`agents-plugin-tool/internal/wstracker/check.go#L603-L651`, `start_test.go#L103-L118`). Hook `detail.start` is already optional RFC 3339, like due (`agents-plugin-tool/internal/wstrackerhook/event.go#L62-L70`, #L97-L117).
- Canonical command tables, clear values and hook detail omit `start` (`agents-plugin/rsrc/tracker/tracker.md#L102-L103`, #L265-L266, #L361-L364). Standard mirrors include both `agents-plugin-wsflow/rsrc/tracker/tracker.md` and `agents-plugin-pi/rsrc/tracker/tracker.md`; both currently byte-match canonical (checked with `cmp`). Resource manifests and byte-identity guards already exist (`agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L18-L44`, `wsflow_mirror_test.go#L47-L82`, `pi_mirror_test.go#L20-L76`).
- Existing coverage surfaces: `agents-plugin-tool/internal/mcp/tracker_render_test.go`, `tracker_tools_test.go` (query/manual coverage and `TestTrackerUpdateStart`), `tracker_support_test.go#L247-L283` (`TestTrackerShowCardsOwnStoreOnly`), and the resource guards above. These tests were inspected, not executed during fact population. Query detail/scope dispatch is `agents-plugin-tool/internal/mcp/tracker_tools.go#L1565-L1567`; show cards use the separate renderer noted above, whose absent due prints `due none` rather than omitting it.

## Prior Decisions

- 261113-feat-ws-tracker-start-date-range-picker (2026-11-14, Result): "a one-sided edit reads the other end from the folded item, a clear never refuses, and an inverted pair folded from concurrent edits is accepted unflagged." — bearing: constrains
- 261018-bug-lead-run-wrapped-line-reads-as-heading (2026-11-15, Result): "Regenerated the canonical manifest and maintained byte-identical wsflow/Pi resource copies." — bearing: constrains
- e9a3f557 (2026-11-08, commit): "The card line 'card <store> <alias>' is the Phase 2 renderer's hook." — bearing: constrains
- 9e96807f (2026-11-11, commit): "Kept minimal and additive; new mine-scope sections appear only when non-empty so existing outputs are unchanged." — bearing: supports
- 261108-feat-ws-tracker-caller (2026-11-08, commit): "Output IDs use the display alias (maya.pc-5); input accepts both forms." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Canonical tracker.md, wsflow/Pi resource mirrors and manifests; mcp/tracker_render.go, tracker_show.go and focused coverage |
| scope.surface | public-interface | Shipped tracker reference and existing tracker.query/tracker.show text results |
| scope.new_public_symbol | no | Existing start attribute and renderers only; no new API authorized |
| scope.new_type_contract | no | Existing ItemView.Start and hook detail.start; storage, validation, events and arguments unchanged |
| scope.test_surface | existing | mcp/tracker_render_test.go, tracker_tools_test.go, tracker_support_test.go; wsrsrc manifest/mirror guards |
| complexity.reuse_points | confirmed | Read existing item/assignment/detail/card due formatting, start projection and resource mirror guards |
| complexity.side_effect_risk | low | Read-only formatting and shipped reference edits; no store writes or event changes authorized |
| risk.correctness | moderate | Cover start-only, start with due and absent-start unchanged across separate item, assignment, detail and card formatters |
| risk.fit | low | Explicitly approved adjacent-due formatting and existing byte-identical resource mirror mechanism |
| risk.test | moderate | Existing coverage needs focused start assertions in multiple renderers and canonical/mirror documentation guards |
| risk.security_or_contract | low | T0 docs/additive text only; existing API, validation, storage, event fields and card prefix preserved |

## Phases

### Phase 1: Document and display the existing start date

Document the existing `start` create/edit attribute, `clear="start"`, validation relationship with `due`, and hook `detail.start` field in the canonical tracker reference and its standard mirrors. Include a present start date in existing tracker query and show-card text renderers, following the adjacent due-date format; leave absent-start output unchanged. Add focused documentation/mirror and renderer coverage. Do not change storage, validation, event or API argument semantics.
