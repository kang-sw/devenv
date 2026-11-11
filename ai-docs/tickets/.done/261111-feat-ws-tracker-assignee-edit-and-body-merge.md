---
title: "Tracker: the live assignee edits an item, and body edits merge three-way from the base the editor saw"
related:
  261110-feat-ws-tracker-item-move-reparent: "sibling from the same board round; its Decision 2 widens moves to the live assignee the same way"
  261111-feat-ws-tracker-cc-field: "follow-up from (235) a: related members track an item through a CC field instead of a relaxed assign"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 08604559db8fcdc0
sage-review-completeness-reviewed: 08604559db8fcdc0
completed: 2026-11-11
---

# Tracker: assignee editing and body merge

## Background

User, 2026-11-11 board, note on (222) (topic `tracker-edit`): editing an
item body is inconvenient in daily use because only the creator may edit.
They asked that the assignee may edit the body too, that a conflict resolve
last-writer-wins, and whether an edit must dump the whole body or could
apply "the last version the editor saw -> the editor's delta" so that
offline edits that diverged stay safe when they overlap. In the same note:
relax most creator-only task rules to the assignee.

Current state (read 2026-11-11, `agents-plugin-tool/internal/wstracker/check.go`):

- Creator-only operations (with the creator-gone fallback): `item.edit`
  (title, body, project, due, priority, stage), `item.hide`, and
  `item.assign`. An assignee passes their own assignment on with
  `item.handoff`.
- Concurrent field writes go through the register in `state.go`
  (`s.write`): the shown value is `winner()` of the concurrent heads, not
  the last in fold order: an agent head drops out beside a concurrent human
  head of the same member (ac71f46d4), then the later ts / member / clone /
  seq wins (`state.go` `winner`, engine.js `winner`). Writes by different
  members are marked as a "field" conflict showing both values. A body edit
  replaces the whole body.
- Body mentions notify once; there is no watcher or CC concept.

## Decisions

1. **Separate ticket from item.move ((228)).** Assignee editing and the body
   merge are captured here, related to
   `261110-feat-ws-tracker-item-move-reparent`.
   Rejected: one ticket; the merge needs one deterministic diff in Go and
   the reference engine pinned by golden cases, which is independent of
   moving, and neither blocks the other.
2. **`item.edit` opens to the item's creator or a member holding a live
   assignment on it, for every field, body included ((229) a).** `item.hide`
   and `item.assign` stay creator-only. "Every field" includes `stage`, so
   the live assignee may move an item between review and todo; the
   creator-gone fallback is unchanged. The creator-or-live-assignee test is
   one helper in Go and in the reference engine with one `reasons.json`
   string, shared with `261110-feat-ws-tracker-item-move-reparent`; any
   item-field event that another ticket adds under `project`'s rules (such
   as labels from `261111-feat-ws-tracker-store-settings-and-labels`) is
   widened with it (technical, review-raised).
   Rejected: relaxing edit and hide ((229) b); relaxing edit, hide, and
   assign ((229) c). Hide removes the item for everyone; assign is the
   lead's delegation and the assignee already has `item.handoff`. The
   user then asked whether assign should relax too, or related people get
   a CC field or an assignment tier; answered (235) a: assign stays
   creator-only, and related people track an item through a separate CC
   field instead (`261111-feat-ws-tracker-cc-field`, (236)). Rejected there: the live assignee
   assigning too ((235) b), since delegation already works through
   subtasks the assignee creates and assigns; an "assignee | CC"
   assignment tier ((235) c), which would have to define accept, decline,
   and done for a watcher; both a CC field and assignee assign ((235) d).
3. **A body edit carries the full new text and a base ((230)).** `base` is
   the body revision the editor saw: the body write heads of the value they
   loaded. The base text is the body folded over the causal closure of
   those heads, which is exactly the value the editor saw. The fold decides
   from causal structure (technical, review-raised): when every body write
   the fold has applied lies inside the base's causal closure, the edit is a
   plain replace; otherwise it merges three-way (base text, the current
   folded value, new text). Old events without a base, and a base naming no
   body write of this item in the edit's causal past, fold as whole
   replaces. The closure test checks direction as well as causality
   (`state.hb` assumes its first argument folds first, `replay.go`), so a
   write counts as inside the closure only when it happens before or is a
   base head (technical, review-raised). Attachments and refs of a merged
   body are the union over the body writes whose lines survive in it;
   mentions notify from the write that carried them, as today; a golden
   case pins it (technical, review-raised).
   Rejected: storing a delta, which needs the base to apply anyway, while a
   full text keeps each event readable alone; deciding plain replace by
   equality of the base id with the shown head id, which loses edits: member
   1 writes A (base X); member 2, offline, writes B (base X) and then C
   (base B); folding A, B, C: B merges into M under head id B, and C would
   then replace M and drop A's change; taking the base text from the head's
   current value, which may be a merge the editor never saw.
4. **Line-granular merge with one diff algorithm in both implementations
   ((232)).** Myers line diff in Go and in the reference engine, pinned by
   golden cases. Overlapping hunks (line ranges in the base that intersect or touch): the later write in fold order wins,
   except that a human write beats a concurrent agent write of the same
   member, keeping the ac71f46d4 precedence for overlaps ((287) b);
   non-overlapping changes from both survive; an overlap keeps the
   existing "field" conflict mark ((231)); a non-overlapping merge carries
   no conflict mark, even between different members (technical,
   review-raised). The body register's shown value is the running fold
   value, not `winner()`: the ts / member / clone / seq winner no longer
   applies to the body, since a merged value is no single head's value and
   under clock skew `winner()` could show a head the merge already folded
   in ((287)). A whole replace (no base, or an unknown base) overlaps
   every line of the current value, so the same-member human-over-agent
   exception applies to it too (technical, review-raised). An overlap
   keeps the register's existing marking: a "field" conflict between
   different members, the quiet superseded note within one member
   (technical, review-raised).
   Rejected: following fold order throughout, which drops the
   human-over-agent precedence for the body ((287) a); keeping the ts
   winner for the body, which cannot coexist with merging. Known cost:
   an old base-less ledger whose fold order and ts winner disagree can show
   a different body after upgrade.
   Rejected: a character-level merge, larger to build and mirror. Known
   cost: a markdown paragraph is often one line, so two edits in one
   paragraph resolve as one overlapping hunk. Whole-body LWW, which would
   drop the non-overlapping edits the base exists to keep ((231)).
5. **`base` is optional ((233)).** The dashboard always sends it;
   `tracker.update` takes the body revision `tracker.query` printed; an
   absent or unknown base is a whole replace.
   Rejected: a required base, which breaks old clients and agent calls.

## Constraints

- The relaxed permission uses the same live-assignment test as
  `item.create`'s parent check (`liveRec`); its rejection text names the
  creator-or-assignee rule.
- The body merge and the base field are mirrored in the reference engine
  (`ai-docs/ref/tracker-mockup/engine.js`) and its golden cases
  (`agents-plugin-tool/internal/wstracker/testdata`, `gen-golden.js`), and
  documented in the tracker playbook of all three packages, as the existing
  event types are ((210)).
- Labels (`261111-feat-ws-tracker-store-settings-and-labels`) landed on
  develop in 7023382fd, so Decision 2's widening covers its label event.
- Quiet dashboard tone: `--wb-*` tokens.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Golden files: view JSON fields carry no `omitempty`, so a body-revision
  view field changes every file under `testdata/golden`; regenerate them
  from the reference engine after each merge, never merge them by hand, and
  give new case files a distinct name prefix.
- Field names, the revision format `tracker.query` prints, and the conflict
  mark wording are chosen in planning.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 261110-feat-ws-tracker-item-move-reparent (2026-11-10, Decisions): "Who may move: the item's creator or a member holding a live assignment on it ((222) b). The creator-gone fallback of `item.edit` applies." — bearing: supports
- 261111-feat-ws-tracker-cc-field (2026-11-11, Decisions): "A CC item field; assign stays creator-only ((235) a)." and Decision 3: "Adding any store member to an item's CC is open to every member, not only to those who may edit the item" — bearing: supports
- 261111-feat-ws-tracker-store-settings-and-labels (2026-11-11, Decisions): "Labels are an item field like `project` (technical, (209)). They are set in the item window beside `project`, by anyone who may edit that item, under the same rules as `project`." — bearing: constrains
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Decisions): "Every task always has an assignee (user, (552) a; tasks only, (557)). When an item's last live or pending assignment ends ... the creator holds it" — bearing: constrains
- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Decisions): "Who moves between review and todo: whoever may edit the item's body, which under the current `item.edit` rule (`wstracker/check.go`) is the creator, or an admin when the creator was removed." — bearing: constrains
- 4449ed91 (2026-11-08, commit): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains
- 7a15afbd (2026-11-08, commit): "item.edit with mentions or attachments but no body is flagged (\"mentions and attachments go with a body edit\"), since decision 2/3 attach both to body changes only." — bearing: constrains
- 261108-feat-ws-tracker-wiki-git-branch (2026-11-09, Result): "`WikiSave` requires an explicit `Base` ... merges three-way against a moved tip" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, state.go, payload.go, testdata/gen-golden.js and cases; ai-docs/ref/tracker-mockup/engine.js; agents-plugin-tool/internal/mcp/tracker_tools.go, tracker_render.go; agents-plugin-tool/internal/wsdashboard/assets/tracker.js; agents-plugin*/rsrc/tracker/tracker.md in three packages |
| scope.surface | cross-module | item.edit event payload and tracker.update / tracker.query MCP surface plus dashboard and three playbook copies |
| scope.new_public_symbol | yes | base field on the item.edit payload in payload.go and the tracker.update argument; names chosen in planning |
| scope.new_type_contract | yes | item.edit event gains optional base; tracker.query prints a body revision |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/tracker_test.go, golden_test.go, reasons_test.go with testdata/cases and testdata/golden |
| complexity.reuse_points | confirmed | liveRec in state.go#L493 already used by item.create parent check check.go#L407; register write state.go#L905-L930 with field conflict mark |
| complexity.side_effect_risk | high | fold change alters how every ledger body write replays, and the permission check gates every item.edit in Go and engine.js |
| risk.correctness | high | a Myers line diff and three-way merge must be deterministic and byte-identical between Go and engine.js across fold order |
| risk.fit | moderate | touches the register model in state.go that currently stores whole values per key |
| risk.test | moderate | golden parity cases and a browser two-window check are needed beyond unit tests |
| risk.security_or_contract | high | widens a write permission and adds a persisted event field that old and new clients must both fold |

## Phases

### Phase 1: assignee editing and body merge

Scope:
- `item.edit` permission widened to the live assignee (Decision 2), in the
  Go domain, the reference engine, the dashboard's edit affordances, and
  the playbook.
- The `base` field, the three-way line merge in the fold (Decisions 3-5),
  `tracker.update` and `tracker.query` support, and the dashboard sending
  the base it loaded.

Verification:
- Tracker tests: a live assignee's edit succeeds; a non-assignee's is
  rejected naming the rule; the assignee changes `stage`; hide and assign
  stay creator-only; an edit whose base closure covers every applied body
  write replaces; two edits from one
  base touching different lines both survive; two edits from one base with
  overlapping hunks resolve to the later in fold order with the "field"
  conflict mark, and a non-overlapping merge between different members
  carries no conflict mark; a same-member human write beats a concurrent
  agent write that folds later, both on an overlapping hunk and against an
  agent whole replace; an edit without a base, or with an unknown base,
  replaces whole.
- Golden cases from the reference engine agree with the Go domain for the
  merge, including diverged offline edits, the A / B-then-C sequence of
  Decision 3, an old base-less event, the human-over-agent overlap
  exception, and the attachment / ref union and mention source of a merged
  body.
- A browser check: an assignee edits a body; two windows editing one body
  from the same base merge.

### Result (140b46fac) - 2026-11-11

Landed in ee8ec74dd (domain + reference engine), c33416658 (MCP and
playbook), 15bbccb67 (dashboard), 140b46fac (lite-review fixes).

- `item.edit` is judged by `needCreatorOrAssignee` with the creator-gone
  fallback, in `check.go` and `engine.js`; hide and assign stay
  creator-only. The team-alpha fixture's maya.pc-10 edit of ada.pc-1, which
  had been refused, now applies, so the golden title becomes "Ship search v2
  by Friday" and the flagged count drops from 17 to 16.
- New `merge.go`, mirrored in `engine.js`, adds three things. A Myers line
  diff, with common prefix and suffix trimmed and a compact trace. A
  three-way merge whose groups are hunks that intersect or touch. A running
  body fold with line provenance, whose base text is folded over the
  causal closure of the base heads and memoized per write. A group both
  sides changed to the same lines is not an overlap.
- Each overlapping group names its rivals: the concurrent writes whose lines
  the folded value holds in that group, or every concurrent write when the
  value only dropped lines there. Human-over-agent precedence for the same
  member is settled per group. Markers and superseded notes go only to the
  rivals.
- Attachments and refs are the union over the writes that still have
  surviving lines (each oid once). Mentions come from the latest such write.
- The view gains `bodyRev`. `base` must go with a body and must be a list
  of id strings; both checks have reasons.json cases.
- `tk:item.edit` takes `base` (space-separated ids in either form; `$N`,
  an empty value and base without a body are refused). The item view prints
  `body revision: <aliases>`. tracker.md documents both in all three
  packages, and the manifest hashes are updated.
- Dashboard: `canEdit` now allows the creator or a live assignee, while the
  new `canHide` stays creator-only. The edit form pins the body it loaded
  and its `bodyRev`, compares the edited body against the pinned one, and
  sends `base` with the body.

Verification (ws-rgo): `test ./internal/wstracker/ ./internal/mcp/` ok;
`test ./internal/wsrsrc/` ok; `test ./internal/wsdashboard/` ok; vet for
wstracker, mcp and wsdashboard clean. `gen-golden.js` reports reasons.json
261 cases agree, and the new case-body-merge-rivals golden passes parity.
Browser check over CDP on the mockup: a live assignee who is not the
creator gets edit and no hide, and saving a body edit sends `base`, which
the store applies. The two-window same-base merge was not exercised in the
browser. The domain golden and Go cases cover that merge.

Open (Minor, from lite review): a cur-side change from a write in the
edit's causal past but outside its base closure resolves to the later
write without a mark, which the playbook's "both are marked" overstates.
`diffLines` keeps an O(D^2) trace and bodies have no size cap. A based edit
that leaves the text unchanged but adds attachments contributes no
surviving lines in a merge, so those attachments drop.
