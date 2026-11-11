---
title: "Tracker: the live assignee edits an item, and body edits merge three-way from the base the editor saw"
related:
  261110-feat-ws-tracker-item-move-reparent: "sibling from the same board round; its Decision 2 widens moves to the live assignee the same way"
  261111-feat-ws-tracker-cc-field: "follow-up from (235) a: related members track an item through a CC field instead of a relaxed assign"
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
  (`s.write`): last writer in fold order wins, and writes by different
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
   and `item.assign` stay creator-only.
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
   the id of the body write the editor last saw. The fold merges: a base
   equal to the shown body head is a plain replace; otherwise it merges
   three-way (base, shown head, new text). Old events without a base fold
   as whole replaces, as today.
   Rejected: storing a delta, which needs the base to apply anyway, while a
   full text keeps each event readable alone.
4. **Line-granular merge with one diff algorithm in both implementations
   ((232)).** Myers line diff in Go and in the reference engine, pinned by
   golden cases. Overlapping hunks: the later write in fold order wins;
   non-overlapping changes from both survive; an overlap keeps the
   existing "field" conflict mark ((231)).
   Rejected: a character-level merge, larger to build and mirror. Known
   cost: a markdown paragraph is often one line, so two edits in one
   paragraph resolve as one overlapping hunk. Whole-body LWW, which would
   drop the non-overlapping edits the base exists to keep ((231)).
5. **`base` is optional ((233)).** The dashboard always sends it;
   `tracker.update` takes the body revision `tracker.query` printed; an
   absent base is a whole replace.
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
- Quiet dashboard tone: `--wb-*` tokens.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Field names, the revision format `tracker.query` prints, and the conflict
  mark wording are chosen in planning.

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
  rejected naming the rule; hide and assign stay creator-only; an edit on
  the shown head replaces; two edits from one base touching different
  lines both survive; an edit without a base replaces whole.
- Golden cases from the reference engine agree with the Go domain for the
  merge, including diverged offline edits and an old base-less event.
- A browser check: an assignee edits a body; two windows editing one body
  from the same base merge.
