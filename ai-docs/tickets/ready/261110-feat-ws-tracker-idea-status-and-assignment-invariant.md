---
title: "Tracker: an idea assignment status replaces the review stage; every task always has an assignee; rule-request fixes"
related:
  261109-feat-ws-tracker-review-stage-and-assignment-defaults: "introduced the review stage (its board (22) a, (52) ok); this ticket retires it"
  261110-feat-ws-tracker-topic-ownership-and-kind-merge: "done (bbb14e07c): topics have one holder and no status; the todo kind merged into task"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 35c40847db279759
sage-review-completeness-reviewed: 35c40847db279759
---

# Tracker: idea status, the assignment invariant, and rule-request fixes

## Background

User, 2026-11-10 whiteboard ((543) note, then (550), (551), (552)): the
review stage was never what they meant. Their original proposal was an
"idea" level (구상, "착수 검토") in front of todo among the statuses a holder
sets, distinct from post-completion review (the `awaiting` status). They
also stated that no item should ever be assigned to nobody.

Today the review stage is an item field (`stage`, `wstracker/state.go:114-118`)
meaning "waiting before todo with no assignment": moving to review is refused
while the item holds a live or pending assignment (`check.go:191-214`, `checkStage`), and
any assign promotes it to todo (`state.go:1358`). `itemStatus`
(`wstracker/view.go:364-413`) derives `review`, `pending`, `declined`,
`returned`, or `unassigned` for an item with no live assignment. The holder
can set only todo, doing, blocked, done.

Two carried-over decisions from the 2026-11-09 handoff (board (534), (535))
touch the same derivation:

- `item.unassign` with `root` ends every open rule of the author rooted
  there (`state.go:1154-1158`), so withdrawing a pending rule request from
  the dashboard can also end a live rule.
- An item whose only open assignment is a pending rule request derives
  status `pending` (`view.go:385-387`), while the mockup README says it
  should not (`ai-docs/ref/tracker-mockup/README.md`, `engine.js`).

All Go paths are under `agents-plugin-tool/internal/`.

## Decisions

1. **`idea` is a holder-settable assignment status before todo; the review
   stage is retired (user, (543) note and (550) a).** Settable statuses become idea, todo,
   doing, blocked, done (UI "구상" first). The `stage` item field is no
   longer written. Rejected: keeping `stage` and only renaming it to idea
   ((543) a), and adding idea as a second stage before review ((543) b).
   Both kept unassigned pre-todo items, which Decision 2 forbids.
2. **Every task always has an assignee (user, (552) a; tasks only, (557)).**
   When an item's last live or pending assignment ends (unassign, decline,
   the lead's return, member removal), the creator holds it; when the
   creator was removed, an admin does (which admin: Decision 7). The todo kind merged into task
   (bbb14e07c), and a topic has no status and a single holder who may
   release it (`261110-feat-ws-tracker-topic-ownership-and-kind-merge`
   Decisions 4, 5, 5a), so a topic may be held by nobody and this invariant
   does not touch topics. Rejected: leaving `unassigned` as a visible
   exception state ((552) b).
3. **Idea items are left out of progress roll-ups (user, (551)),** as the
   review stage and pending are today (`view.go:419-421`). An item's
   derived status is idea only when every live holder is at idea; with
   holders at mixed statuses, idea ranks below todo in `itemStatus`'s
   precedence (`view.go:364-413`), so one holder at todo or doing makes the
   item count. The mockup engine and goldens follow the same rule.
4. **`item.unassign` takes an optional rule id (user, (534) a).** With it,
   only that rule ends (a pending rule request is withdrawn); without it,
   the current behavior stays. The dashboard's withdraw on a pending rule
   request passes the id. Rejected: one open rule per author and root
   ((534) b); README-only fix ((534) c).
5. **A pending rule request does not count as status `pending` (user,
   (535) b).** Go `itemStatus` and the mockup `engine.js` follow the README.
   Rejected: changing the README to match the code ((535) a).
6. **An item whose only open assignment is a pending rule request is held
   by its creator until the request is accepted or declined (user, (554) a).**
   Its status is the idea or todo the creator set, todo when the creator
   set none (Decision 12), and it shows in the creator's "mine". Rejected: counting the pending rule request as an
   assignment for the invariant, with a new derived status such as
   "requested" left out of roll-ups ((554) b).
7. **The creator fallback is derived in the fold, not written as an assign
   event ((555)).** Assignment ends come from other writers' events (an
   admin's member removal, a lead's return, concurrent machines); a derived
   holder cannot conflict or double-assign, and old unassigned items gain a
   holder with no migration write. The fold gives the fallback assignment a
   deterministic record id so later status events find it; the id is
   derived from the event that starts the fallback period (the item's
   create for a never-assigned task, else the event that ended the last
   assignment), so each period has its own status register and a second
   fallback period starts at todo per Decision 12, not at the first
   period's last status. When the creator was removed, the fallback holder
   is the first admin in the store's roles admin order at fold time; that
   admin's later removal re-derives the holder the same way. An applied
   `item.assign` of anyone, the creator included, displaces the derived
   fallback, and the assign check's "already holds a live or pending
   assignment" test ignores the fallback, so an explicit assign is never
   refused because of it. The two writers that self-assign right after a
   create stop doing so, since the fallback already makes the creator the
   holder: the dashboard's `selfAssigned` assign (`tracker.js:3332-3334`)
   and the MCP writer's creator self-assignment insertion
   (`mcp/tracker_tools.go:738-750`). Rejected: writing an assign event when
   an assignment ends; keeping both self-assign insertions (redundant
   records that only work because assign displaces the fallback).
8. **Item-level statuses `unassigned`, `declined`, and `returned` stop
   occurring, and the kanban "all" columns for them and for `review` go
   ((556), a consequence of Decision 2).** A decline or return answer
   still shows in the *received requests* scope (the mockup README's Items
   panel table, that scope's row) and in notifications. The "all" kanban gains an idea column before
   todo, and the review column's "+" quick-create row (`createKnew`,
   `knewRowHtml`, `ui.knew`) moves to the idea column, creating a
   creator-held idea task as Decision 11 does. Rejected: removing the
   column row (it is the kanban's quick capture).
9. **`tk:item.create` and `tk:item.edit` drop the `stage` attribute and
   refuse it ((558)).** Old events still fold (Decision 13). Rejected:
   accepting `stage` as an alias for idea, which keeps a retired concept in
   the API.
10. **No create-time status attribute ((559)).** Creating straight into idea
    is `tk:item.create` plus `tk:item.status status="idea"` in one batch.
    Rejected: a new `status` attribute on create (new API surface).
11. **The dashboard quick-add (Title + Enter) creates the task assigned to
    its creator with status idea ((560)),** in both quick-add rows: the
    Items panel row (`createNew` and `selfAssigned`, `tracker.js:3319-3334`,
    which today self-assign at todo) and the kanban column row (`createKnew`,
    `tracker.js:3335-3353`, today the review-stage create of (53)),
    keeping 261109's (53) intent (quick capture lands before todo) under
    Decision 2. Choosing status, assignee, and schedule at quick-add time is
    a separate follow-up
    (`261110-feat-ws-dashboard-tracker-quick-add-fields`).
12. **The fallback assignment starts at status todo ((561)).** Nobody is
    working on it any more, and demoting it to idea would hide committed
    work. Rejected: idea, or the ended assignment's status. Old stage events
    are the exception (Decision 13).
13. **Old events carrying `stage: "review"` fold as "held by the creator,
    status idea" ((562)),** keeping their pre-todo meaning; the holder comes
    from Decision 7's fold derivation. Rejected: folding them as todo.

## Constraints

- Stores are event logs shared across machines. Old events carrying
  `stage: "review"` must still fold, as Decision 13 states.
- `tk:` command docs and tool descriptions are shipped text (command specs in
  `internal/mcp/tracker_tools.go#L128-L129`; the docs in
  `agents-plugin/rsrc/tracker/tracker.md`, mirrored byte-for-byte in
  `agents-plugin-wsflow/rsrc/tracker/tracker.md` and copied in
  `agents-plugin-pi/rsrc/tracker/tracker.md`): read
  `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
  before editing `internal/mcp/`.
- Topic assignment belongs to
  `261110-feat-ws-tracker-topic-ownership-and-kind-merge`; do not change
  topic behavior here.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`
  (`js.tk.st.review` today).
- `261110-feat-ws-tracker-search-query-grammar` runs alongside and touches
  the same tracker.js filter code, `mcp/tracker_tools.go`, and the three
  `tracker.md` copies; neither depends on the other. This ticket owns the
  `trackerStatuses` list (`mcp/tracker_tools.go:1009`): if search's
  `status:` filter validates against it, the list this ticket updates is
  the one it reads.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Decisions): "Review stage (user; board (22) a): an item-level stage before todo ... Rejected: no stage, using unassigned items as the backlog (b)" — bearing: constrains
- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Result 9390fed99): "MCP writer: stage=review on create/edit, clear=stage; ... creator self-assignment inserted right after each unassigned non-review task/todo create" — bearing: constrains
- 261110-feat-ws-tracker-topic-ownership-and-kind-merge (2026-11-10, Decisions): "The mockup engine moves in lockstep. The Go fold is pinned against golden files that internal/wstracker/testdata/gen-golden.js writes from the mockup engine" — bearing: constrains
- 261110-feat-ws-tracker-topic-ownership-and-kind-merge (2026-11-10, Decisions): "Existing topics are not backfilled (lead, assumed). Topics created before this change stay unheld; their creator can assign them later." — bearing: supports
- 261110-feat-ws-tracker-topic-ownership-and-kind-merge (2026-11-10, Result 381e94371): "Topic records keep the internal assignment status todo (an empty status ranks differently in the engine's and Go's my.assignments sort)" — bearing: constrains
- 46731727 (2026-11-08, commit): "Added item.unassign {root} to end one rule; without it a rule request could only be withdrawn by unassigning everything." — bearing: supports
- b7dd9839 (2026-11-09, commit): "Accepted without a fix: the JS harness gap (tracker.js/dashboard.js have no executing test) and state.go:1128 unassign{root} (needs a protocol choice; board (534))" — bearing: supports
- 261110-feat-ws-dashboard-tracker-quick-add-fields (2026-11-10, Decisions): "The quick-add row gains optional status, assignee, and schedule (due) controls. Left untouched, the row behaves as the idea-status ticket's Decision 11 says." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wstracker/state.go, check.go, view.go, payload.go; mcp/tracker_tools.go; wsdashboard/assets/tracker.js; wsi18n/tables/*.json; ai-docs/ref/tracker-mockup/engine.js and README.md; rsrc/tracker/tracker.md in three packages; wstracker/testdata goldens |
| scope.surface | public-interface | tk:item.unassign gains a rule id; tk:item.create and tk:item.edit refuse stage; item status set gains idea and loses review, unassigned, declined, returned |
| scope.new_public_symbol | yes | item.unassign rule-id attribute; idea status value |
| scope.new_type_contract | yes | item.unassign payload gains a rule id; item.status accepts idea; stage removed from create/edit payloads; view statuses change |
| scope.test_surface | existing | wstracker/scenarios_test.go, golden_test.go with goldens from testdata/gen-golden.js, reasons_test.go, topic_test.go; tracker.js has no executing harness per b7dd9839 |
| complexity.reuse_points | confirmed | itemStatus view.go#L364-L413, counted view.go#L419-L421, unassign root branch state.go#L1154-L1158, checkStage check.go#L191-L214, selfAssigned tracker.js#L3332-L3334 read |
| complexity.side_effect_risk | high | a derived fallback holder changes every view, roll-up, kanban column, notification, and golden output over shared multi-machine event logs |
| risk.correctness | high | fold must derive a deterministic fallback record id that later status events resolve, fold old stage events as idea, and handle every assignment-ending path including member removal |
| risk.fit | moderate | Go fold, mockup engine, goldens, MCP writer, dashboard, i18n tables, and three playbook copies must move in lockstep |
| risk.test | high | goldens are regenerated from the engine being changed, so engine and Go can drift together unnoticed; dashboard JS has no executing test |
| risk.security_or_contract | moderate | shipped tk: API refuses a previously accepted stage attribute and changes the status vocabulary agents read |

## Phases

### Phase 1: rule-request fixes

Decisions 4 and 5: the `item.unassign` rule id (fold, check, `tk:` command,
docs, dashboard withdraw), and the `pending` derivation in `view.go` and
`ai-docs/ref/tracker-mockup/engine.js`. Verification: `go test ./...` under
`agents-plugin-tool`, with scenario tests for withdrawing one of two rules
on a root and for a rule-request-only item's status.

### Result (18941160e, cd44f8610) - 2026-11-10

- `item.unassign` takes an optional `rule` id in the Go fold/check and the
  mockup engine. Only that rule of the author's ends: a pending request is
  withdrawn, an accepted one unassigned. The rule and root branches share
  one `unassignRule` helper. `rule` next to `assignee` is refused, and so is
  a `root` that is not the rule's root. reasons.json has six new refusal cases.
- A pending rule request no longer makes the item `pending` (Go
  `itemStatus` and the engine). TestAssignRuleOutsideReach now expects
  `unassigned`; Phase 2's fallback changes that to `todo`.
- `tk:item.unassign` takes `rule` in either printed form. The item view
  prints each rule's ID after its label, so an agent can name the rule.
  The dashboard's withdraw on a pending rule request passes `rule`.
- New tests in wstracker/holder_test.go: withdrawing one of two rules on
  the same root by id leaves the other live, while ending by root ends
  both; a rule-request-only item is not pending.
- Verified: ws-rgo `go test` on mcp, wsrsrc, wsdashboard and wstracker
  all ok; gen-golden reports 175 reason cases agree, with goldens unchanged.

### Phase 2: idea status and the assignment invariant

Decisions 1 to 3 and 6 to 13 on top of Phase 1's derivation: the idea
status in the fold, check, `tk:` docs, dashboard status picker, kanban
columns, quick-add rows, and i18n; retiring the stage field everywhere the
dashboard still sends it (menu promote/demote, `createKnew`, the status
filter's review option, the kanban column list); the creator fallback and
the end of the two self-assign insertions (Decision 7). Lockstep surfaces
to change together: `trackerStatuses` (`mcp/tracker_tools.go:1009`: add
idea, drop review, unassigned, declined, returned), the MCP stage writer
(`mcp/tracker_tools.go:493-497`), the mockup `engine.js` and README, and
the goldens regenerated with `internal/wstracker/testdata/gen-golden.js`.
Verification: `go test ./...`, with scenario tests for each
assignment-ending path landing on the creator (and on the first admin when
the creator was removed), a second fallback period starting at todo, an
explicit creator assign right after create accepted, idea excluded from
roll-ups and mixed idea/todo holders counting, `stage` refused on create
and edit, and an old `stage: "review"` event folding as specified; plus a
headless-browser check on an isolated daemon that both quick-add rows
create a creator-held idea task and the "all" kanban shows the idea column
with no review, unassigned, declined, or returned column.

### Result (2dfd0f9ed, 6392106d1, eb3a16346) - 2026-11-10

- Fold (Go and engine.js in lockstep): a task nobody else holds gets a
  derived fallback record `<start event>@<item>><holder>`, held by the
  creator or, when the creator was removed, the first admin in the roles
  admin order. Each period starts at todo. The record is settled at the
  end of every apply, and in `stateBefore` after `refreshLive`. Any live
  or pending record other than a rule request displaces it. The assign
  check ignores it, handoff refuses it ("assign it instead"), and return
  and unassign skip or refuse it. A status write that had not seen the
  period's start folds onto the ended record. The holder is re-derived
  only on member removal; losing the admin role keeps it. Ended fallbacks
  are left out of the view; `Assignment.Fallback` marks a live one.
- `idea` is settable. An item is idea only when every live holder is, and
  idea is left out of roll-ups. The item statuses review, unassigned,
  declined and returned no longer occur, and `QueryStatuses` follows. New
  writes that carry `stage` are refused (`retiredStage`, synthetic only).
  Older stage events fold onto the live fallback: review becomes idea,
  null becomes todo.
- MCP: `tk:item.create` and `tk:item.edit` drop `stage`, and `clear="stage"`
  goes too. `tk:item.status` takes idea. The creator self-assignment is
  kept for topics only; a topic has no fallback. tracker.md is updated in
  all three packages, with the manifest hashes edited by hand.
- Dashboard:
  - The idea status replaces review.
  - The "all" kanban columns are pending, idea (with the "+" row), todo,
    doing, blocked, awaiting, done. The assignment kanban gains idea too.
  - Both quick-add rows send create plus `item.status $1 idea`. A topic
    still self-assigns. The inline subtask row sends a bare create.
  - The promote/demote menu entries are gone.
  - A fallback holding shows no unassign or hand-off control.
  - i18n is updated in all 11 tables, and the mockup `i18n-en.js` is
    regenerated.
- Verified:
  - ws-rgo `go test ./...` passes after merging develop 88326de57, and
    `go vet` on wstracker and mcp is clean.
  - gen-golden: 178 reason cases agree.
  - Headless check (chromium headless shell over CDP): Items quick-add and
    the idea column's "+" each created a creator-held fallback record at
    idea. The "all" kanban had no review, unassigned, declined or returned
    column. This check ran on the mockup page (shared tracker.js over
    engine.js), not on an isolated daemon, because a daemon binary cannot
    be built locally (go runs only on the remote server). The Go path for
    create plus status idea is covered by
    `TestTrackerUpdateCreatorHoldsAnUnassignedTask`.
- Lite review: no Critical or Important findings. Minor 4 (README wording)
  is fixed in 497c04bd8. Left:
  - The `hb` guard on fallback status writes and the `stateBefore`
    `settleAll` have no direct Go test.
  - `settleAll` walks every item on each assign/roles event.
  - A status write routed to an ended fallback names a record the view
    omits.
