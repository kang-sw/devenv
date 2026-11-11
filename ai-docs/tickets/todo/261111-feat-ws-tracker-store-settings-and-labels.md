---
title: "Tracker: a store-settings section for priority levels, the attachment limit, and an admin-defined label list; multi-valued labels on items"
related:
  261110-epic-ws-tracker-team-feedback: "its Non-Scope recorded priority levels as needing no work; this ticket's Decision 1 is the discoverability work the user asked for later"
---

# Tracker: store settings and labels

## Background

User, 2026-11-11 whiteboard, topic `tracker-taxonomy`: agents could not
tell them where the priority labels are changed, even after
`tracker.query`. They also asked whether a work category such as
"문서화 / 개발 / 잡무" should be a separate label layer or an extension of
the existing topic/task split.

Current state (read 2026-11-11):

- Priority levels are an admin event, `roles.set_priority_levels`
  (`names` lowest to highest, at most 9, and a `default` index;
  `internal/wstracker/check.go` `TypeSetPriorityLevels`). The attachment
  limit is `roles.set_attachment_limit` (`max_bytes`). Both sit in the
  Team tab's "admin: role events" dialog, which lists raw event names in a
  `<select>` (`ROLE_TYPES`) and shows every field for every type at once
  (`internal/wsdashboard/assets/tracker.js` `dialogHtml` case `admin`).
- `tracker.query` prints a `priority:` line with the levels and the
  default and nothing about how to change them
  (`internal/mcp/tracker_render.go`). The tracker playbook lists the event
  only in its admin command table (`agents-plugin/rsrc/tracker/tracker.md`).
- `kind` (topic or task) is a lifecycle split: a task has a status and an
  assignee, a topic is a stateless discussion one person holds.
- `project` is one free-text field per item, shown on the row as
  `· <project>` and matched exactly by `project:<p>`
  (`internal/wstracker/query.go`). There is no predefined list or chip.

## Decisions

1. **A store-settings section in the Team tab (user, (195) a).** Store-wide
   settings that are not roles move out of the role-event dialog into
   their own section of the Team tab: the priority levels (names and
   default), the attachment limit, and the label list (Decision 3). An
   admin edits them there; a non-admin sees them read-only. The role-event
   dialog shows only the fields of the selected type, with human-readable
   type names instead of raw event names. The `priority:` line of
   `tracker.query` gains a hint naming how to change the levels (the
   Team tab's store-settings section, or the admin event).
   Rejected: keeping the dialog and only tidying its fields, type names,
   and the query hint ((195) b); leaving it as is ((195) c).
2. **Work category is a new multi-valued label layer (user, (196) a).** An
   item carries any number of labels. Each shows as a chip on the item's
   row, and `label:<name>` filters by it. `project` stays as the "which
   project" axis, and `kind` stays the lifecycle split. The user's reason
   for a separate layer: the category is orthogonal to `kind` ("a
   documentation topic" and "a documentation task" both make sense), and
   one item often belongs to more than one category.
   Rejected: extending `project` into a predefined single-valued list
   ((196) b), which cannot express a project and a category at once;
   extending `kind` ((196) c), which mixes lifecycle rules with category;
   using free-text `project` as the category with no code change ((196) d).
3. **The store defines the label list (user, (205) a).** An admin manages
   the list (name and colour) in the store-settings section; members pick
   only from it. The reason: in a team store one meaning otherwise
   scatters across names ("문서", "문서화", "docs"). A personal store's
   owner is its admin and can add to the list directly, and labels are
   managed the same way as priority levels.
   Rejected: free entry with autocomplete ((205) b); an admin list plus
   free entry ((205) c).
4. **Labels have stable ids (technical, (208)).** Each label in the store
   list has a stable id. Renaming or recolouring it updates every item that
   carries it; removing it from the list hides it from items, chips, and
   `label:` results while the event history keeps it.
   Rejected: keying labels by name, which breaks every item on a rename and
   leaves dangling names after a removal.
5. **Labels are an item field like `project` (technical, (209)).** They are
   set in the item window beside `project`, by anyone who may edit that
   item, under the same rules as `project`. Quick-add stays unchanged.
   Rejected: a separate label permission, which adds a rule for no stated
   need; a quick-add label field, which is the quick-add ticket's decision.
6. **Two phases (technical, (207)).** Phase 1 the store-settings section,
   dialog cleanup, and query hint; Phase 2 labels, which builds on the
   Phase 1 section that manages the list. Phase 1 alone fixes the reported
   discoverability problem.
   Rejected: one phase.

## Constraints

- The settings and label events are admin-only, like
  `roles.set_priority_levels` and `roles.set_attachment_limit`; the
  rejection for a non-admin names the admin requirement.
- Quiet dashboard tone: `--wb-*` tokens; label chips use the store-defined
  colour inside the existing chip style.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the playbook and
  the `tracker.query` hint ship downstream), `ai-docs/manuals/skill-authoring.md`
  and `ai-docs/manuals/wsflow-mirroring.md` (the tracker playbook), and
  `ai-docs/manuals/ws-mcp.md` (the render change).
- The new events (label list and item labels) are mirrored in the
  reference engine (`ai-docs/ref/tracker-mockup/engine.js`) and its golden
  cases (`agents-plugin-tool/internal/wstracker/testdata`, `gen-golden.js`),
  and the tracker playbook documents them in all three packages
  (`agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-pi/`
  `rsrc/tracker/tracker.md`), as the existing event types are ((210)).
- Event names, payload shapes, and the label colour format are chosen in
  planning.

## Phases

### Phase 1: store-settings section and discoverability

Scope:
- The Team tab's store-settings section with priority levels and the
  attachment limit, editable by an admin and read-only otherwise
  (Decision 1).
- The role-event dialog shows only the selected type's fields, under
  human-readable type names, and no longer carries the two settings.
- The `tracker.query` `priority:` line hint, and the tracker playbook's
  wording of where the levels are changed.

Verification:
- A render test that the `priority:` line carries the hint.
- A browser check of the section as admin and as non-admin, and of the
  dialog showing only the selected type's fields.

### Phase 2: labels

Scope:
- The store label list (name, colour) managed in the store-settings
  section (Decision 3), and labels on items (Decision 2): row chips, the
  item window field, and `label:<name>` in the tracker query and the
  dashboard filter.

Verification:
- Tracker tests: a non-admin's label-list change is rejected; an item
  cannot carry a label outside the list; a renamed label shows its new
  name on items; a removed label leaves items, chips, and `label:`
  results.
- Golden cases from the reference engine agree with the Go domain.
- A browser check of the chips, the item window field, and the label list
  editor.
