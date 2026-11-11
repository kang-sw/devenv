---
title: "Tracker item.move: reparent and reorder an existing item, by drag, selection, and a move picker"
related:
  261111-feat-ws-tracker-assignee-edit-and-body-merge: "sibling from the same board round; both widen creator-only rules to the live assignee"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a18352117bd14bfb
sage-review-completeness-reviewed: a18352117bd14bfb
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
  (the viewer created the item or holds a live assignment on it, not a
  topic, not hidden; `internal/wsdashboard/assets/tracker.js#L686`). The
  topic exclusion is dashboard-only: `item.create`'s write check has no
  topic-parent rejection (`agents-plugin-tool/internal/wstracker/check.go#L399-L410`).
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
   The topic rule is new to `item.move`: `item.create`'s write check has no
   topic-parent rule in Go or the reference engine (`check.go#L399-L410`,
   `engine.js#L460`; only the dashboard's `canSplit` keeps children off
   topics), so `item.move` alone refuses a topic parent, with its rejection
   reason kept in `reasons.json` parity, and `item.create` is unchanged
   (technical, (274)).
   The destination rule applies only when the parent changes (user,
   (280) a): a reorder under the same parent checks only the item rule
   (creator or live assignee), so children already under a topic reorder in
   place, and a move to top level checks only the item rule. The
   creator-or-live-assignee test is one helper in Go and in the reference
   engine with one `reasons.json` string, shared with
   `261111-feat-ws-tracker-assignee-edit-and-body-merge` (technical,
   review-raised).
   Rejected: the creator only ((222) a); any member passing the destination
   rule ((222) c); adding the topic rule to `item.create` too, which would
   change what an existing event type accepts ((274)); applying the
   destination rule to a same-parent reorder, which would freeze children
   already under a topic ((280)).
3. **Cycles and concurrency ((217), (218)).** A move under the item's own
   descendant is refused at write. Two concurrent moves that together form
   a cycle: the later in fold order is flagged and ignored. Concurrent
   moves of one item: the register's winner rule decides, as for concurrent
   field edits (`s.write`, `winner()`), and the other is marked
   overwritten; goldens follow that rule, not fold order. Write checks (item rule, destination rule) are evaluated
   against the event's causal past, as `item.create`'s parent check is: a
   destination hidden, or its creator removed, concurrently with the move
   does not flag the move (technical, review-raised).
   A hidden item is not moved, and an `after` that is not a child of the
   new parent is rejected at write; their reason texts are chosen with the
   other flag reasons in planning (technical, review-raised).
   Rejected: leaving a concurrent cycle undefined; re-checking the
   destination against concurrent events at fold, which no other write
   check does.
4. **A drag always moves the whole subtree ((223) a).** A leaf moves alone
   because it has no subtree, which covers moving a single ticket.
   Rejected: an Alt-held "this item only" mode that lifts the item's
   children into its old slot ((223) b, (220)); the user asked whether it
   was meaningful, and it only serves taking a grouping item out while
   leaving its children.
5. **Drop targets ((221), (219)).** A row's middle band makes the dragged
   item its last child, anchored after the last visible child; the top and bottom bands place it as the sibling
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
   each anchored after the previous. Roots the drop target refuses are
   named up front with the reason, and the valid ones move (technical,
   review-raised).
   Rejected: a multi-item event; each move is valid alone, so an
   interleaved concurrent write only affects order.
8. **A "Move to..." picker in the item menu ((224) a, (227)).** It lists
   the parents that pass the destination rule, plus top level, in a
   searchable tree, and places the item as the last child of the choice;
   the item's own subtree is left out. Opened on a selected row it applies
   to the whole selection, listing destinations valid for at least one
   selected root and naming refused roots as Decision 7 does for a drag
   (technical, review-raised). It is the
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
  fold, and a golden case pins the chosen outcome.
- The kanban status drag stays as it is.
- Golden files: view JSON fields carry no `omitempty`; regenerate them from
  the reference engine after each merge, never merge them by hand, and give
  new case files a distinct name prefix.
- Quiet dashboard tone: `--wb-*` tokens; drop indicators and checkboxes
  inside the existing row style.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the playbook and
  `tracker.update` ship downstream), `ai-docs/manuals/skill-authoring.md`
  and `ai-docs/manuals/wsflow-mirroring.md` (the tracker playbook),
  `ai-docs/manuals/ws-mcp.md` (the MCP command).
- Payload field names and the flag reason texts are chosen in planning.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261111-feat-ws-tracker-assignee-edit-and-body-merge (2026-11-11, Decisions): "`item.edit` opens to the item's creator or a member holding a live assignment on it, for every field, body included ((229) a). `item.hide` and `item.assign` stay creator-only." — bearing: supports
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Items carry a sibling position: a create records an `after` anchor (a sibling id, or none for first), so an item inserted between two rows shows there; reorder and reparent later extend the same field." — bearing: supports
- 46731727 (2026-11-08, commit): "a hidden anchor keeps its slot instead of falling back to append. Falling back would move every row anchored on it (and their followers) to the end when a row is hidden" — bearing: constrains
- e2bdce49 (2026-11-08, commit): "Hover insert depth options span from the row-below's depth up to child of the row above. ... A child option appears only where canSplit allows, so never under a topic" — bearing: supports
- 4449ed91 (2026-11-08, commit): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains
- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Decisions): "Rejected: drag between kanban columns (drag today moves only the ..." — bearing: constrains
- ab754b67 (2026-11-07, commit): "A subtask is an independent item with a parent link, re-delegation goes through it, rather than in-place reassignment of the same item." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, state.go, view.go, testdata/gen-golden.js, internal/wsdashboard/assets/tracker.js, internal/mcp/tracker_tools.go, ai-docs/ref/tracker-mockup/engine.js, agents-plugin/rsrc/tracker/tracker.md, agents-plugin-wsflow/rsrc/tracker/tracker.md, agents-plugin-pi/rsrc/tracker/tracker.md |
| scope.surface | cross-module | new ledger event type spans wstracker domain, MCP tracker.update, dashboard, reference engine, and three shipped playbooks |
| scope.new_public_symbol | yes | item.move event type and its tk:item.move command in tracker.update; exact names chosen in planning |
| scope.new_type_contract | yes | item.move payload naming item, parent, and after; field names chosen in planning |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/golden_test.go, scenarios_test.go, reasons_test.go, testdata/cases and golden; dashboard has no test suite, browser check only |
| complexity.reuse_points | confirmed | item.create parent checks check.go#L399-L410, s.write state.go#L905, insOpts hover-insert slots tracker.js#L3776-L3790, view.go (528) sibling ordering |
| complexity.side_effect_risk | high | a new fold changes derived parent and order for every replayed store and interacts with the concurrent-unassign subtask mark state.go#L1000-L1008 |
| risk.correctness | high | cycle refusal at write, concurrent-cycle flagging, last-write-wins moves, and move-versus-unassign fold must agree across Go and engine.js |
| risk.fit | moderate | new drag, checkbox selection mode, and picker added to a 5011-line tracker.js alongside the kanban drag that must stay unchanged |
| risk.test | moderate | golden parity covers the domain, but the drag, selection, and picker UI rely on a manual browser check |
| risk.security_or_contract | moderate | new permission rule and ledger event type that old clients and shipped playbooks must handle |

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
  is hidden, a topic, or not the mover's is rejected when the parent
  changes; a reorder in place under a topic and a move to top level pass on
  the item rule alone; a move under the item's own descendant is rejected;
  reorder within one parent; a move keeps comments, assignments, and
  history.
- Golden cases from the reference engine agree with the Go domain,
  including two concurrent moves forming a cycle, two concurrent moves of
  one item, a move concurrent with an unassign, a move concurrent with a
  hide of its destination, and `after` anchored on a hidden sibling.
- A browser check in the MY view: subtree drag onto a row's middle and
  edge bands, a leaf drag, range and toggle selection, Esc and empty
  selection leaving the mode, a selection drag keeping tree order, a
  selection drop with a refused root, and the picker on one item and on a
  selection.

### Result (eb1164232) - 2026-11-11

Landed in 55cfaff3f (domain + engine), c9c4578e2 (tracker.update +
playbooks), 90cd1c3ee (dashboard), eb1164232 (review fixes).

- Payload `item.move {item, parent, after?}`: `parent` is required (an item
  id, or null for top level); `after` follows `item.create` (absent = last,
  `""`/null = first, id = after that sibling, which must be a child of the
  new parent at write).
- Reason texts (all in `reasons.json`, engine parity): the item and
  destination ownership rule reuse `item.create`'s "`<parent >X` is neither
  assigned to nor created by `<m>`" through one helper
  (`needCreatorOrAssignee`, Go and engine) for 261111 to share; plus "X is
  hidden", "parent required (...)", "X cannot move under itself", "parent X
  is a topic; a topic takes no children", "parent X is in Y's own subtree",
  "X cannot follow itself", "after X is not a child of <parent | the top
  level>". The concurrent-cycle flag reads "moving X under Y would close a
  cycle with a concurrent move; ignored".
- Fold: parent and after are one position register (`it:<id>:place`), so
  concurrent moves of one item follow the register's winner rule with the
  usual field / superseded marks. A placement is replayed by the view only
  when the shown winner changes; sibling order now replays creates and shown
  moves in fold order, and a moved item leaves a vacated slot so rows
  anchored on it stay (anchor lookup: the live slot, else the last vacated).
  Stores without moves produce identical goldens.
- Move vs unassign (the Constraints' planning item): the move stays applied
  and is marked "subtask under <parent>" against the unassign in either fold
  order (`endRec` keys on the event that set the parent; `applyMove` marks an
  already-ended record).
- Check-time fold (`stateBefore`) for an `item.move` also folds every create
  and move, since the subtree rule walks ancestors outside the named threads.
- Dashboard: list-view row drag with 25/50/25 bands (halves when the row
  takes no children), reusing `insOpts` for the edge bands; checkbox
  selection mode per Decision 6; selection drag moving only selection roots
  in tree order, chained by `after`, refused roots named in the drop marker
  and the toast; "Move to..." picker in the item menu (single item or
  selection); `item.move` thread line; 24 i18n keys in every locale.
  Client rules mirror `checkMove`, so a refused move is never offered.
- Verification: `ws-rgo test ./internal/wstracker/` ok (reasons.json 193
  cases agree with the engine; new `case-move-tree`, `case-move-concurrent`
  goldens cover the concurrent cycle, two concurrent moves of one item incl.
  the winner rule beating fold order, move vs unassign in both orders, move
  vs hide of its destination, `after` on a hidden sibling, vacated slots;
  `TestMoveByCreatorAndLiveAssignee`, `TestMoveSubtreeTravels`,
  `TestMoveVerdictsEqualFullReplay`); `ws-rgo test ./internal/mcp/` ok
  (`tk:item.move` in the every-command walk); `./internal/wsi18n/...`,
  `./internal/wsdashboard/...`, `./internal/wsrsrc/...` ok; `node --check
  tracker.js`; lite review: no Critical or Important, three Minor fixed.
- Not done: the browser check in the MY view (no browser available to the
  worker). Drag is wired for the list view only (the picker is the
  keyboard/touch path); rows of a collapsed closed section and the top of a
  later section take no drop.
