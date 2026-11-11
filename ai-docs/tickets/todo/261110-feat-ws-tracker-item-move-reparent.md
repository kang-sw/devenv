---
title: "Tracker item.move: reparent and reorder an existing item, by drag, selection, and a move picker"
related:
  261111-feat-ws-tracker-assignee-edit-and-body-merge: "sibling from the same board round; both widen creator-only rules to the live assignee"
---

# Tracker item.move: reparent and reorder an existing item

## Background

User question on the 2026-11-09 board (item (151) note): "can a task be
reparented at all?" Answer (152) b captured this as an idea. On 2026-11-11
the user asked for it as a needed feature (topic `tracker-move`): in the MY
view, drag a whole branch, or a single ticket, to a new place, even if the
data structure has to change radically.

Current state (read 2026-11-11):

- An item already stores its own parent (`item.parent` in
  `agents-plugin-tool/internal/wstracker/state.go`, a child-to-parent
  pointer); children are derived when the view is built
  (`internal/wstracker/view.go`, sibling order (528) from each item's
  `after` anchor). No restructure is needed.
- `parent` and `after` are set only by `item.create`. `item.edit` folds
  title, body, project, due, priority, and stage, and is creator-only. A
  misplaced item can only be recreated, losing its history, comments, and
  assignments.
- `item.create` under a parent requires the parent not hidden and the
  writer to be the parent's creator or hold a live assignment on it
  (`check.go`); the dashboard offers children only where `canSplit` holds
  (not a topic, not hidden; `internal/wsdashboard/assets/tracker.js`).
- The dashboard's only drag today moves kanban cards between status columns.

## Decisions

1. **One new `item.move` event; no data-model restructure ((212), (216)).**
   The event names an item and sets its `parent` (an item id, or none for
   top level) and `after` (a sibling id, or `""` for first). Reorder is the
   same event with the same parent and a new `after` ((214)).
   Rejected: adding `parent`/`after` to `item.edit` ((216)), which is
   creator-only and folds field registers, while a move needs its own
   permission and cycle check, and old edit events must keep their meaning;
   flipping the parent/child storage direction, which is already
   child-to-parent ((212)).
2. **Who may move: the item's creator or a member holding a live assignment
   on it ((222) b).** The creator-gone fallback of `item.edit` applies.
   The destination must also pass `item.create`'s parent rules: the parent
   is not hidden, the mover is the parent's creator or holds a live
   assignment on it, and a topic takes no children ((213)).
   Rejected: the creator only ((222) a); any member passing the destination
   rule ((222) c).
3. **Cycles and concurrency ((217), (218)).** A move under the item's own
   descendant is refused at write. Two concurrent moves that together form
   a cycle: the later in fold order is flagged and ignored. Concurrent
   moves of one item: the last write in fold order wins and the earlier is
   marked overwritten, the same rule as concurrent field edits
   (`s.write`).
   Rejected: leaving a concurrent cycle undefined.
4. **A drag always moves the whole subtree ((223) a).** A leaf moves alone
   because it has no subtree, which covers moving a single ticket.
   Rejected: an Alt-held "this item only" mode that lifts the item's
   children into its old slot ((223) b, (220)); the user asked whether it
   was meaningful, and it only serves taking a grouping item out while
   leaving its children.
5. **Drop targets ((221), (219)).** A row's middle band makes the dragged
   item its last child; the top and bottom bands place it as the sibling
   before or after, reusing the hover-insert slot computation
   (`tracker.js` ~L3778). In a filtered view (MY), `after` anchors on the
   visible row above; hidden siblings in between keep their place, as
   (528) keeps a hidden anchor's slot.
   Rejected: a new hit-test model; computing a position among rows the
   user cannot see.
6. **Checkbox selection, user design ((225), (234)).** Hovering a row shows
   its checkbox; clicking it enters selection mode. There, Shift-click
   selects a range, Ctrl-click toggles cumulatively, Shift+Ctrl-click adds
   a range from the last selected position, and a plain row click selects
   only that row; the item window opens from the menu or Enter. Ranges
   follow visible row order, collapsed descendants excluded. An empty
   selection or Esc returns to normal mode.
   Rejected: a "select" toggle in the panel tools that shows every
   checkbox (the lead's first proposal, replaced by the user's design); a
   plain click opening the item window in selection mode, which misfires
   while selecting.
7. **Dragging a selection ((226)).** Dragging a selected row moves the
   whole selection: the items land consecutively at the drop point in tree
   order, and a selected item whose ancestor is also selected travels with
   that ancestor. The dashboard sends one `item.move` per selected root,
   each anchored after the previous.
   Rejected: a multi-item event; each move is valid alone, so an
   interleaved concurrent write only affects order.
8. **A "Move to..." picker in the item menu ((224) a, (227)).** It lists
   the parents that pass the destination rule, plus top level, in a
   searchable tree, and places the item as the last child of the choice;
   opened on a selected row it applies to the whole selection. It is the
   keyboard and touch path and uses the same event and rules as a drag.
   Rejected: drag only, with the picker later ((224) b).

## Constraints

- Moves stay within one store; each store is its own ledger ((215)).
- `item.move` is mirrored in the reference engine
  (`ai-docs/ref/tracker-mockup/engine.js`) and its golden cases
  (`agents-plugin-tool/internal/wstracker/testdata`, `gen-golden.js`),
  exposed through `tracker.update`, and documented in the tracker playbook
  of all three packages (`agents-plugin/`, `agents-plugin-wsflow/`,
  `agents-plugin-pi/` `rsrc/tracker/tracker.md`), as the existing event
  types are ((215), (210)).
- Assignment rules (`{root, include_root, levels?}`) target the member org
  tree, not the item tree, so a move changes no assignment. The
  concurrent-unassign "subtask" mark (`state.go`, the fold that marks an
  item an assignee created under the item they were unassigned from)
  depends on `parent`; planning decides how a concurrent move and unassign
  fold, with golden cases.
- The kanban status drag stays as it is.
- Quiet dashboard tone: `--wb-*` tokens; drop indicators and checkboxes
  inside the existing row style.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the playbook and
  `tracker.update` ship downstream), `ai-docs/manuals/skill-authoring.md`
  and `ai-docs/manuals/wsflow-mirroring.md` (the tracker playbook),
  `ai-docs/manuals/ws-mcp.md` (the MCP command).
- Payload field names and the flag reason texts are chosen in planning.

## Phases

### Phase 1: item.move end to end

Scope:
- The `item.move` event in the Go domain and the reference engine, with
  its write checks (Decisions 1-3) and fold.
- `tracker.update` support and the playbook text in three packages.
- Dashboard: subtree drag with the drop bands (Decisions 4-5), checkbox
  selection and selection drag (Decisions 6-7), and the "Move to..."
  picker (Decision 8).

Verification:
- Tracker tests: a move by the creator and by a live assignee succeeds; a
  move by another member is rejected naming the rule; a destination that
  is hidden, a topic, or not the mover's is rejected; a move under the
  item's own descendant is rejected; reorder within one parent; a move
  keeps comments, assignments, and history.
- Golden cases from the reference engine agree with the Go domain,
  including two concurrent moves forming a cycle, two concurrent moves of
  one item, a move concurrent with an unassign, and `after` anchored on a
  hidden sibling.
- A browser check in the MY view: subtree drag onto a row's middle and
  edge bands, a leaf drag, range and toggle selection, Esc and empty
  selection leaving the mode, a selection drag keeping tree order, and the
  picker on one item and on a selection.
