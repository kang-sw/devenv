---
title: "Tracker: a store-settings section for priority levels, the attachment limit, and an admin-defined label list; multi-valued labels on items"
related:
  261110-epic-ws-tracker-team-feedback: "its Non-Scope recorded priority levels as needing no work; this ticket's Decision 1 is the discoverability work the user asked for later"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 78039b04ff2d1959
sage-review-completeness-reviewed: 78039b04ff2d1959
completed: 2026-11-11
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
  Team tab's "Admin: roles event" dialog (button label
  `js.tk.team.adminbtn`, `agents-plugin-tool/internal/wsi18n/tables/en.json#L946`), which lists raw event names in a
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
   Label names are unique within the store list: an add or rename to a name
   the list already holds is rejected at write, so `label:<name>` names one
   label. A removed label frees its name; adding that name again creates a
   new label with a new id, and items that carried the removed one do not
   regain it (technical, review-raised). Two concurrent adds or renames to
   one name both pass the write check; the fold keeps the first in fold
   order and flags the later with a reason naming the duplicate, pinned by
   a golden case and a `reasons.json` case (technical, review-raised).
   Rejected: keying labels by name, which breaks every item on a rename and
   leaves dangling names after a removal; duplicate names with `label:`
   matching every id of that name, which brings back the scattered names
   Decision 3 exists to stop; restoring a removed label on re-add, which
   needs a second lifecycle for one name.
5. **Labels are an item field like `project` (technical, (209)).** They are
   set in the item window beside `project`, by anyone who may edit that
   item, under the same rules as `project`. Quick-add stays unchanged. The
   label write uses the same permission check as `item.edit` (a field of
   `item.edit`, or the shared creator-or-live-assignee helper), never a copy
   of today's creator-only check, so the widening in
   `261111-feat-ws-tracker-assignee-edit-and-body-merge` covers labels in
   whichever order the two tickets land (technical, review-raised).
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
- Golden files: view JSON fields carry no `omitempty`, so an item `labels`
  field changes every file under `testdata/golden`; regenerate them from
  the reference engine after each merge, never merge them by hand, and give
  new case files a distinct name prefix.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 4871a856 (2026-11-11, commit): "User answers: (195) a, a store-settings section plus per-type dialog fields plus a query hint; (196) a, a new multi-valued label layer, with project and kind unchanged; (205) a, the store defines the label list" — bearing: supports
- 25d11857 (2026-11-10, commit): "(612) recorded as the lead's assumed reading: the priority question was whether items carry a priority, which they already do (roles.set_priority_levels, up to 9 levels), so it is Non-Scope." — bearing: supports
- 261111-feat-ws-tracker-assignee-edit-and-body-merge (2026-11-11, Decisions): "`item.edit` opens to the item's creator or a member holding a live assignment on it, for every field, body included ((229) a)." — bearing: constrains
- 261111-feat-ws-tracker-cc-field (2026-11-11, Decisions): "A CC item field; assign stays creator-only ((235) a). CC is an item field like `project`, edited by whoever may edit the item." — bearing: constrains
- 757b956f (2026-11-08, commit): "wsledger is untouched: it treats non-protocol roles.* types as opaque payload, so roles.set_attachment_limit flows like roles.set_priority_levels." — bearing: constrains
- 261108-feat-ws-tracker-ledger-attachments (2026-11-08, Decisions): "the roles-file `attachments.max_bytes` caps an attachment newly added by a write; an attachment carried over unchanged ... is exempt, so lowering the limit never locks old posts." — bearing: constrains
- e11589f5 (2026-11-08, commit): "(271) no-marker rule is applied to every field register (status, lead, priority levels too), not only body: a member's two clones are one writer." — bearing: constrains
- 261108-feat-ws-tracker-data-structure (2026-11-08, Decisions): "Admin status, removal, and membership used by domain rules are folded by the domain from the identity events in each event's causal past ... not read from `Identity()`" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, query.go, payload.go; agents-plugin-tool/internal/wsdashboard/assets/tracker.js; agents-plugin-tool/internal/mcp/tracker_render.go; three rsrc/tracker/tracker.md copies; ai-docs/ref/tracker-mockup/engine.js; testdata/gen-golden.js |
| scope.surface | cross-module | tracker domain events, MCP query render, dashboard UI, shipped playbook in three packages |
| scope.new_public_symbol | yes | new label-list and item-label event types in wstracker, event names chosen in planning |
| scope.new_type_contract | yes | new event payload shapes and label colour format, chosen in planning; item view gains a labels field |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tracker_*_test.go, agents-plugin-tool/internal/wstracker/testdata golden cases |
| complexity.reuse_points | confirmed | admin gate needAdmin and priority-level field-register pattern in check.go#L856-L883; project field edit path in tracker.js#L2339 |
| complexity.side_effect_risk | moderate | new ledger event types fold into every store view and must match the reference engine golden output |
| risk.correctness | moderate | rename and removal by stable id must fold consistently across Go and engine.js |
| risk.fit | moderate | settings section moves existing controls out of the role-event dialog and adds a filter key to query grammar |
| risk.test | moderate | browser checks are manual; golden cases must be regenerated from the reference engine |
| risk.security_or_contract | moderate | new admin-only events and payloads become a persisted ledger contract that ships downstream |

## Phases

### Phase 1: store-settings section and discoverability

Scope:
- The Team tab's store-settings section with priority levels and the
  attachment limit, editable by an admin and read-only otherwise
  (Decision 1).
- The role-event dialog shows only the selected type's fields, under
  human-readable type names, and no longer carries the two settings.
- The `tracker.query` `priority:` line hint, and the tracker playbook's
  wording of where the levels are changed, in all three packages'
  `rsrc/tracker/tracker.md`.

Verification:
- A render test that the `priority:` line carries the hint.
- A browser check of the section as admin and as non-admin, and of the
  dialog showing only the selected type's fields under human-readable type
  names, with the priority-level and attachment-limit types gone from it.

### Result

- The Team card ends with a "Store settings" section (`settingsHtml` in
  `tracker.js`): priority levels (lowest first, default marked) and the
  attachment limit, each with an Edit button only for an admin; a
  non-admin reads the same rows with a read-only note. The Team summary
  line no longer repeats the two settings.
- Two dialogs write the settings: `sprio` (one name row per level plus
  blank rows up to the 9-level cap, a radio for the default; blank rows are
  dropped, an unchanged save is a no-op toast) and `satt` (a byte count).
- The roles-event dialog offers only the seven membership types, as
  readable names (`js.tk.rtype.*`), and shows only the selected type's
  fields (`rfld` wrappers with `data-rt`, toggled by `rtypeShow` on open and
  on type change).
- `tracker.query`'s `priority:` line ends with where the levels are
  changed; all three `rsrc/tracker/tracker.md` copies say the same for both
  settings.
- Tests: `TestTrackerQueryPriorityLineNamesWhereLevelsChange` (mcp) and
  `TestTrackerStoreSettingsLeaveTheRolesDialog` (wsdashboard, a source pin
  of the dialog and section). The browser check is pending and runs with
  Phase 2's.

### Phase 2: labels

Scope:
- The store label list (name, colour) managed in the store-settings
  section (Decision 3), and labels on items (Decision 2): row chips, the
  item window field, and `label:<name>` in the tracker query and the
  dashboard filter.

Verification:
- Tracker tests: a non-admin's label-list change is rejected; an item
  cannot carry a label outside the list; a duplicate label name is
  rejected; a label set by a non-editor is rejected by the `item.edit`
  rule; a re-added removed name gets a new id; a renamed label shows its new
  name on items; a removed label leaves items, chips, and `label:`
  results.
- Golden cases from the reference engine agree with the Go domain.
- A browser check of the chips, the item window field, and the label list
  editor.

### Result

- Domain (`wstracker`): three admin-only events, `roles.add_label {name,
  color}` (the label id is the event id), `roles.edit_label {label, name?,
  color?}`, and `roles.remove_label {label}`. A colour is `#rrggbb` in
  lowercase hex. A name is unique among live labels ignoring case and
  carries no surrounding spaces. A rename or recolour is a register, so
  concurrent edits resolve like every other field, and removal is
  terminal: re-adding the name makes a new id. `labels` (a list of label
  ids) is an item field on `item.create` and `item.edit`, so the creator
  edit rule covers it and `null` clears it. Each id must be a live label
  and may appear once. The view carries `items[].labels` (live ids only, so
  a removed label leaves its items) and `labels` (live labels in add
  order). `label:<name>` matches an exact name, ignoring case.
- Concurrent duplicate names: two concurrent adds of one name (or a rename
  to it) each pass their causal-past check. The verdict pass then flags
  every one after the first in fold order, as `label "X" already exists
  (<alias>, written concurrently; the first in fold order keeps the
  name)`. This is a fold-order exception to the stateBefore rule, written
  down in `doc.go`; the reference engine mirrors it.
- Reference engine (`ai-docs/ref/tracker-mockup/engine.js`): the same
  events, checks, and view. All goldens are regenerated, a new case
  `labels-concurrent-name` is added, and `reasons.json` has 22 label cases.
  A step's optional fifth element now sets `seen` explicitly, so a case
  can be concurrent; Go's `history()` and `gen-golden.js` both honour it.
- MCP: `labels=` (label refs, or `$N` for a `tk:roles.add_label` command
  of the same batch) on `item.create` and `item.edit`, plus the commands
  `roles.add_label`, `roles.edit_label`, and `roles.remove_label`.
  `tracker.query` prints a `labels:` header line when the store has labels
  and `labels a, b` on item rows and the item detail. All three
  `tracker.md` copies document the grammar.
- Dashboard (`tracker.js`, `tracker.css`):
  - Label chips (an outline chip with a colour dot) on item rows and in
    the item window. Clicking a chip sets the Items search to
    `label:"<name>"`.
  - The item window has a `labels` property row, and its edit mode has a
    toggle-chip picker that writes `labels` only when the set changed.
  - The store-settings section lists the labels. An admin gets "+ label",
    Edit (the `slabel` dialog: a name and an `<input type=color>`,
    emitting only the changed fields), and Remove (after a confirm).
  - Activity sentences cover the three events and item labels. A removed
    label is still named, from its add and rename events.
  - 21 new i18n keys, in all 11 tables.
- Tests: `TestItemLabelsFollowTheLabelList` and
  `TestConcurrentLabelNameKeepsTheFirstInFoldOrder` (wstracker), plus the
  `reasons.json` label cases (rejections for a non-admin, an
  outside-the-list label, a duplicate name, a non-editor's labels, a bad
  colour, and a removed label). In mcp: `TestTrackerQueryShowsLabels` and
  label steps in `TestTrackerUpdateWritesEveryEventCommand`. In
  wsdashboard: `TestTrackerLabelsInTheDashboard` (a source pin), plus
  label cases in `TestTrackerSearchRequests` for the substring fallback.
- Browser check (headless Chromium over CDP, against the mockup at 1280px
  and 390px), covering Phase 1's pending check too:
  - Store settings render read-only for a member (maya) and editable for
    an admin (ada).
  - The roles dialog lists the seven membership types by readable name
    and shows only the selected type's fields.
  - Priority-level and attachment-limit edits save.
  - Labels: an add, a duplicate name flagged, the edit picker
    toggling, row and window chips, a chip click filtering to
    `label:"docs"`, a rename and a recolour shown on rows, a removal that
    takes the chip off the item, and the activity sentences.
  - The 390px page's horizontal overflow (401px) comes from the mockup's
    own stub row and is the same at the base commit.

#### Edition (ddf65188d) - 2026-11-11

- Review fix: an add or rename flagged only by the fold-order name clash
  still counts in later events' causal-past checks, because their writers
  saw it. An item created or edited with the losing label therefore stays
  applied, and the view drops the lost label from it. Before the fix the
  whole item write was flagged. Pinned in
  `TestConcurrentLabelNameKeepsTheFirstInFoldOrder` and the
  `labels-concurrent-name` golden case, in both engines.
- Also fixed: the chip tooltip was double-escaped, and the edit picker
  compared label order rather than the label set.
