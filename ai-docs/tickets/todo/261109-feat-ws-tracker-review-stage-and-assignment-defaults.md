---
title: "ws tracker: a review stage before todo, approval-by-default when assigning others, and done/blocked notices to the assigner"
parent: 261107-epic-ws-personal-common-store
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
  assigning to oneself defaults to `assignee`. Rejected: keep `assignee` as
  the default (a); always `assigner` (c).
- **Done/blocked notice** (board (21) ok): when work I assigned becomes done
  or blocked, I get a notice item, as the mockup README already promises.
- **Review stage** (user; board (22) a): an item-level stage "검토" before
  "할 일": an item can be created in it, it has its own kanban column, it is
  left out of todo roll-ups, and "할 일로 올리기" promotes it. Rejected: no
  stage, using unassigned items as the backlog (b); a label or tag only (c).
- **Assigning promotes** (user; board (32) a): assigning an item in review
  moves it to todo. Rejected: refuse assignment until promoted (b); assign
  while staying in review (c).
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
  permission and the field-merge rule).
- **No self-assign in review** (board (53) ok): an item created in the
  review column is not self-assigned to its creator, as a direct
  consequence of (32) a and (34) c (decision 529's self-assign would
  promote it at once).
- **Review column and its "+"** (board (54) ok): the "all" kanban board
  gets a review column first, with a "+" create control on that column
  only; every other creation path stays as is (todo plus self-assign).
  Rejected: create controls on every column (only the review column's
  creation was asked for).
- **Demotion needs no assignment** (board (55) ok): moving an item back to
  review is refused while it holds a live or pending assignment. Rejected:
  keeping assignments on a review item ((32) a makes "assigned" mean
  "todo").
- **Status and controls** (board (56) ok): a review-stage item shows status
  `review`, is left out of roll-ups like pending, and is promoted or demoted
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

## Phases

### Phase 1: review stage, completion default, done/blocked notices

- Model (`internal/wstracker` and `ai-docs/ref/tracker-mockup/engine.js`
  together): the `stage` item field on `item.create` and `item.edit` with
  validation (tasks and todos only; values `review` or absent) and the
  (33) permission; assignment of a review item clears its stage in the
  same fold (assigning promotes); demotion refused while a live or pending
  assignment exists; `itemStatus` yields `review` for a review-stage item
  and `counted` leaves it out of roll-ups; done and blocked derived
  `Notice` kinds to the assigner when someone else sets them.
- Writers: the MCP tracker tool fills an omitted `completion` on
  `item.assign` by the (20) b rule (`assigner` when any target is not the
  writer, `assignee` when the writer is the only target).
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
  `completion` default for self and other targets. `node --check` on edited
  JS; i18n tests green. The Result lists the browser hand check of the
  kanban column and menu as unverified if not done.
