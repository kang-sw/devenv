---
title: "ws tracker: a review stage before todo, approval-by-default when assigning others, and done/blocked notices to the assigner"
parent: 261107-epic-ws-personal-common-store
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 99cb8a6d16897788
sage-review-completeness-reviewed: 99cb8a6d16897788
---

# ws tracker: a review stage before todo, approval-by-default when assigning others, and done/blocked notices to the assigner

## Background

The user (2026-11-09) described the lead/follower flow: when a follower
marks an assigned item done, the lead approves or rejects it ("완료 대기"),
and items need a "검토" (idea/review) stage before "할 일".

Today (`agents-plugin-tool/internal/wstracker`, mirrored by the mockup
reference engine `ai-docs/ref/tracker-mockup/engine.js`):
- The approval step exists: `item.assign` with `completion: "assigner"`
  makes `item.close` land on `awaiting`, and the assigner sends
  `item.confirm` (done) or `item.reject` (back to doing). The assign
  popover (`assets/tracker.js` `asgState`) defaults to `assignee`.
- There is no review or idea stage. Item status is derived from assignment
  records (`view.go` `itemStatus`); `counted` excludes pending, declined,
  returned, and topics from roll-ups.
- Body edit (`item.edit`) is creator-only, or an admin when the creator was
  removed (`check.go`).
- Notices (`view.go` `Notice`) are derived, with kinds accepted, handoff,
  declined, returned. The mockup README's notifications tab promises
  "completion or `blocked` of work I assigned", which Go does not derive.
- The kanban (`KCOLS`, `kanbanBody`) has no create control; items are made
  from list views by `createNew`, which self-assigns a task or todo to its
  creator at once (`selfAssigned`, decision 529).
- Go and the mockup engine are held equal by golden tests
  (`golden_test.go`, `testdata/gen-golden.js`).

## Decisions

- **Default completion mode** (user; board (20) b): assigning to someone
  else defaults to `assigner` (awaiting, then the assigner approves);
  assigning to oneself defaults to `assignee`. This supersedes the parent
  epic's Cross-Child Decisions -> Assignment line "The default is assignee
  closes"; the epic carries an "Amended 2026-11-09" bullet for it. Rejected:
  keep `assignee` as the default (a); always `assigner` (c).
- **Done/blocked notice** (board (21) ok): when work I assigned becomes done
  or blocked, I get a notice item, as the mockup README already promises.
- **Review stage** (user; board (22) a): an item-level stage "검토" before
  "할 일": an item can be created in it, it has its own kanban column, it is
  left out of todo roll-ups, and "할 일로 올리기" promotes it. Rejected: no
  stage, using unassigned items as the backlog (b); a label or tag only (c).
- **Assigning promotes** (user; board (32) a): assigning an item in review
  moves it to todo. Rejected: refuse assignment until promoted (b); assign
  while staying in review (c). The assign itself needs only the existing
  assign permission, not the (33) editor rule (completeness review,
  auto-confirmed): promotion is a consequence the fold applies, not a stage
  edit by the assigner.
- **Who moves between review and todo** (user; board (33) a; board (51)
  ok): whoever may edit the item's body, which under the current `item.edit`
  rule (`wstracker/check.go`) is the creator, or an admin when the creator
  was removed. Rejected: the creator only (b) as a separate rule.
- **Default stage of a new item** (user; board (34) c): an item created in
  the kanban's review column starts in review; everywhere else it starts in
  todo as now. Rejected: todo with an option (a); always review (b).
- **Stage field** (board (52) ok): the stage is an item field `stage`
  (`review`, or absent meaning todo) on `item.create` and `item.edit`,
  folded like the other item fields, for tasks and todos only (not topics).
  Rejected: a new `item.stage` event (`item.edit` already carries the (33)
  permission and the field-merge rule). Promotion is `item.edit` with
  `stage: null` under the existing null-clears rule (an absent field on an
  edit means untouched); `itemFields` and the "nothing to edit" check
  count `stage` (design review, auto-confirmed).
- **No self-assign in review** (board (53) ok): an item created in the
  review column is not self-assigned to its creator, as a direct
  consequence of (32) a and (34) c (decision 529's self-assign would
  promote it at once). This narrows the epic's (529) line "A `task` or
  `todo` created from the dashboard is assigned to its creator at once"; the
  epic's "Amended 2026-11-09" bullet records it.
- **Review column and its "+"** (board (54) ok): the "all" kanban board
  gets a review column first, with a "+" create control on that column
  only; every other creation path stays as is (todo plus self-assign).
  Rejected: create controls on every column (only the review column's
  creation was asked for).
- **Demotion needs no assignment** (board (55) ok): moving an item back to
  review is refused while it holds a live or pending assignment. Rejected:
  keeping assignments on a review item ((32) a makes "assigned" mean
  "todo"). The refusal is a write-time check of the author's causal past, so
  a demote concurrent with an assign can still reach the fold; the fold
  then lets the assignment win (design review, auto-confirmed): an item
  holding a live or pending assignment is never in review, whatever stage
  edit it folded, identically in Go and `engine.js`.
- **Status and controls** (board (56) ok): a review-stage item shows status
  `review`, is left out of roll-ups like pending (a review-stage subtask too,
  so its parent's progress ignores it), and is promoted or demoted
  from the item menu. Rejected: drag between kanban columns (drag today
  moves only the viewer's own assignment status).
- **Derived notices** (board (57) ok): done and blocked notices are two new
  derived `Notice` kinds sent to the assigner when someone else sets them
  (awaiting stays in the confirm list), mirrored in `engine.js`. Rejected:
  writing notice events (existing notices are derived, never events).
- **Popover default** (board (58) ok): the assign popover defaults to
  `assigner` when any target (person or rule) is not the viewer, and to
  `assignee` when the viewer is the only target. Rejected: judging by the
  first target (a mixed assignment still hands work to someone else).
- **Agents get the same default** (user; board (59) a): when an agent
  writes `item.assign` through the MCP tracker tool without `completion`,
  the MCP writer fills it by the (20) b rule and stores the value
  explicitly; the fold's absent-means-`assignee` rule is unchanged because
  old events rely on it. Rejected: only the dashboard popover changes (b).
- **Agents self-assign what they create** (user; board (66) a): when an
  agent creates a task or todo through the MCP tracker tool and the same
  batch carries no `item.assign` for it, the MCP writer adds the
  self-assignment the dashboard adds (README row 529: people = the
  creator, assignee closes); an item created in review is not
  self-assigned, as (53) holds for the dashboard. Rejected: agents assign
  only explicitly, as today (b).
- **Agent surface** (design review, auto-confirmed): the MCP tracker tool's
  `tk:item.create` and `tk:item.edit` accept a `stage` attribute (`review`),
  and `tk:item.edit` accepts `clear="stage"` for promotion, so agents can
  create in review, promote, and demote; the shipped tracker playbook
  (`agents-plugin/rsrc/tracker/tracker.md`, mirrored byte-for-byte in
  `agents-plugin-wsflow/rsrc/tracker/tracker.md`) gains `review` in its
  status list, the `stage` attribute, and the (20) b / (59) default in place
  of "(the default is `assignee`)", plus the (66) self-assignment.
- **Older clients** (design review, auto-confirmed): an older wstracker
  flags a stage-only `item.edit` ("nothing to edit") and shows a
  review-created item as todo; verdicts are recomputed on upgrade and the
  author's ref is not poisoned, so this is accepted and needs no format
  marker.
- **Lockstep change** (board (60) ok): Go, `engine.js`, `gen-golden.js`,
  the regenerated goldens, the mockup README decision rows, and i18n change
  together, held by `wstracker/golden_test.go` and
  `TestTrackerMockupI18nIsCurrent`.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- Every model change lands in Go, `engine.js`, `testdata/gen-golden.js`, the
  regenerated goldens, and the mockup README decision rows together; UI
  strings go into every dashboard i18n table and the mockup i18n file.
- Convention: ai-docs/manuals/skill-authoring.md and ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/` and `agents-plugin-wsflow/`; the tracker playbook changes)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/; the MCP tracker tool is internal/mcp/tracker_tools.go)

## Prior Decisions

- e2bdce49 (2026-11-08, commit): "Self-assign also applies to the Subtasks '+' / inline subtask row. This supersedes (474)'s 'unassigned task', since (529) says any task or todo created from the dashboard; recorded as README row 529." — bearing: constrains
- e2bdce49 (2026-11-08, commit): "The popover reuses the item menu's assign step (openAssign unchanged), so placement, Back, focus return and Esc come from the menu machinery instead of a second popup system." — bearing: supports
- e11589f5 (2026-11-08, commit): "Accept keeps the assigner and completion mode, which is how (214) 'requester confirms' falls out; v2 made acceptance a self-assignment." — bearing: supports
- 4771bc05 (2026-11-08, commit): "Status click applies at once with no note (setStatus...). 'done' is disabled while awaiting, and reads 'request done' for an assigner-confirmed item, as in the menu." — bearing: constrains
- 0a80fdbf (2026-11-08, commit): "Delegate executed plan 08-0951-tracker-mockup-notifications-mentions-attachments (board (331)-(341)); lead verified go build/vet, go test wstracker+wsledger, node --check before merge." — bearing: supports
- 261108-feat-ws-tracker-data-structure (2026-11-08, 4449ed91): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains
- 261108-feat-ws-tracker-caller (2026-11-08, 166fd45f): "Commands are named after the README event types so the table maps 1:1 onto Dispatch" — bearing: constrains
- 261109-feat-ws-tracker-shared-markdown-editor (2026-11-09, Decisions): parallel todo ticket editing `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` (item body, comment editor); no shared decision, overlapping file. — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, view.go, payload.go, testdata/gen-golden.js, testdata/golden; ai-docs/ref/tracker-mockup/engine.js, README.md, i18n-en.js; agents-plugin-tool/internal/wsdashboard/assets/tracker.js; agents-plugin-tool/internal/mcp/tracker_tools.go; wsi18n tables; agents-plugin/rsrc/tracker/tracker.md and its agents-plugin-wsflow/rsrc/tracker/tracker.md mirror |
| scope.surface | public-interface | ledger event payload field stage on item.create and item.edit; derived Notice kinds; MCP tracker tool item.assign completion default; MCP tracker tool stage attribute, clear="stage", and creator self-assignment on create |
| scope.new_public_symbol | yes | stage field on item.create and item.edit payloads in agents-plugin-tool/internal/wstracker/payload.go; Notice kinds done and blocked |
| scope.new_type_contract | yes | persisted event field stage with values review or absent; item status review; notice kinds done and blocked |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/golden_test.go TestGoldenParity, scenarios_test.go; agents-plugin-tool/internal/mcp/tracker_tools_test.go; agents-plugin-tool/internal/wsdashboard/tracker_mockup_i18n_test.go TestTrackerMockupI18nIsCurrent |
| complexity.reuse_points | confirmed | item.edit permission agents-plugin-tool/internal/wstracker/check.go#L336-L344; counted view.go#L410-L415; derived notices view.go#L847-L883; selfAssigned assets/tracker.js#L2646-L2651; KCOLS assets/tracker.js#L1180-L1181 |
| complexity.side_effect_risk | moderate | new field is written into shared git ledgers read by every member and older clients, and the MCP writer starts storing completion explicitly |
| risk.correctness | moderate | fold must clear stage on assign and refuse demotion under live or pending assignments, including concurrent edit and assign, identically in Go and engine.js |
| risk.fit | moderate | lockstep across Go, engine.js, gen-golden.js, goldens, mockup README, and every i18n table |
| risk.test | moderate | golden regeneration plus new scenario cases; kanban column and item menu need a browser hand check |
| risk.security_or_contract | moderate | extends the persisted tracker event protocol and the item.edit permission scope to stage changes |

## Phases

### Phase 1: review stage, completion default, done/blocked notices

- Model (`internal/wstracker` and `ai-docs/ref/tracker-mockup/engine.js`
  together): the `stage` item field on `item.create` and `item.edit` with
  validation (tasks and todos only; values `review` or absent) and the
  (33) permission, promotion as `stage: null`, and `stage` counted by
  `itemFields` and the "nothing to edit" check; assignment of a review item
  clears its stage in the same fold (assigning promotes, under the assign
  permission alone); demotion refused at write time while a live or pending
  assignment exists, and the fold lets a concurrent assignment win; `itemStatus` yields `review` for a review-stage item
  and `counted` leaves it out of roll-ups (review subtasks included); done and blocked derived
  `Notice` kinds to the assigner when someone else sets them.
- Writers: the MCP tracker tool fills an omitted `completion` on
  `item.assign` by the (20) b rule (`assigner` when any target is not the
  writer, `assignee` when the writer is the only target).
  It also appends the creator self-assignment to an `item.create` of a
  task or todo that the same batch does not assign and that is not created
  in review (66). `tk:item.create` and `tk:item.edit` take a `stage`
  attribute and `tk:item.edit` takes `clear="stage"`.
- Agent docs: the tracker playbook and its wsflow mirror as in the Agent
  surface decision; the parent epic gains an "Amended 2026-11-09 (board
  (20) b, (53) ok, (59) a, (66) a)" bullet under Cross-Child Decisions -> Assignment (already
  written at promotion).
- Page (`assets/tracker.js`): the "all" kanban's review column first, with
  a "+" create control creating a review item without self-assignment; the
  item menu's promote ("할 일로 올리기") and demote entries; the assign
  popover's default by (58); the notice list rendering the two new kinds.
  New UI strings go into every dashboard i18n table and the mockup i18n
  file; the mockup README gains decision rows for these decisions and its
  notifications-tab promise becomes true.
- Verification: `go test ./internal/wstracker/ ./internal/wsdashboard/`
  and the MCP tracker tool's package; regenerate goldens with
  `testdata/gen-golden.js` and keep `golden_test.go` green; tests cover
  create in review, promote and demote with the permission rule, demotion
  refused under an assignment, assignment promoting, roll-up exclusion,
  both notices (and none when the assigner sets the status), and the MCP
  `completion` default for self and other targets, and the MCP
  self-assignment on create (added when the batch has no assign, absent
  when it has one or the item is created in review), the MCP `stage`
  attribute and `clear="stage"`, and a concurrent demote and assign folding
  to the assignment (not review). The wsflow mirror test stays green. `node --check` on edited
  JS; i18n tests green. The Result lists the browser hand check of the
  kanban column and menu as unverified if not done.

### Result (9390fed99) - 2026-11-09

Landed in five commits: 9e0c77973 (model), f61a8b2a9 (MCP writer and
playbook), 64a206dfb (page and i18n), 94c5b75b3 (mockup README), 9390fed99
(review fixes).

- Model (Go `wstracker` and `engine.js`): `stage` on `item.create` /
  `item.edit` (tasks and todos; `review` or null), counted by `itemFields`;
  any applied assign clears it; demotion flagged while a live or pending
  record exists; a concurrent demote folded after an assign is dropped, and a
  register winner that recomputes to review while records are open is
  dropped too. Status `review` (only with no live or pending record), not
  counted. Derived notices `done` / `blocked` to the assigner when someone
  else set the status, keyed `w:<assignment>:<status>:<lastUpdate>` (the
  dashboard's former client-side feed key, so read state carries over); they
  also count on the handle badge. No `stage` field in the item view: status
  carries it.
- MCP writer: `stage="review"` on create/edit, `clear="stage"`; omitted
  `completion` filled (assignee only when the writer is the sole person
  target, no rules/levels); creator self-assignment inserted right after each
  unassigned non-review task/todo create, reported as
  `(self-assignment)` under the create's position; `tracker.query` status
  filter accepts `review`. Playbook updated, wsflow and pi rsrc mirrors
  regenerated/synced.
- Page: `review` status icon and status-filter option; review column first on
  the "all" kanban with a "+" creating an unassigned review task; item menu
  "Move to todo" / "Move to review" under `canEdit`; assign popover default
  by (58) until the radio is touched; done/blocked feed entries read from
  `VS.notices`. Six new i18n keys in all 11 tables; mockup i18n regenerated.
- Verification: `go test ./internal/wstracker/ ./internal/wstrackerhost/
  ./internal/wsi18n/ ./internal/wsrsrc/` ok; `go test ./internal/mcp/` ok
  (leaf run, 398s); `go test ./internal/wsdashboard/` ok (leaf run);
  `gen-golden.js` regenerates with no diff, 160 reason cases agree;
  `node --check` on tracker.js and engine.js; wsflow package tests OK. New
  golden case `review-stage` (create in review, promote, demote, assign
  promotes, review subtask out of roll-up, both orders of a concurrent demote
  and assign, blocked and done notices, none on the assigner's confirm) and
  seven flagged-reason cases.
- Browser check: the page leaf drove the mockup in headless Chromium (review
  column and its single "+", create without assign, promote/demote entries,
  popover default, assigning a review item, blocked notice). Unverified: a
  rendered done notice, visual look and dark mode, a real (non-mockup)
  dashboard session.
- Open (Minor, accepted): the MCP writer defaults any rule or `levels`
  target to `assigner`, though a rule `<writer>+:*` with no reports covers
  only the writer.
