---
title: "Tracker: a superior may set a set of wrap-up statuses on a subordinate's assignment, and new statuses dropped / in-review"
related:
  261110-feat-ws-dashboard-tracker-quick-add-fields: "its Decision 8 keeps the status chip to the creator alone because a superior cannot set a subordinate's status today; this ticket may relax that"
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "defines the current status set (idea, todo, doing, blocked, done) and the assignment invariant this extends"
---

# Tracker: superior-settable statuses and new statuses

## Background

User, 2026-11-10 whiteboard, note on topic "tracker 빠른 추가 필드" after
(609): "todo 티켓 수준으로, 상위에서 지정할 수 있는 상태 세트를 몇 개 두는 걸로
하죠. 뭐 blocked랑 done으로는 상위에서 지정할 수 있다던지(마무리 취급).
그리고 해당 티켓에 새 가능한 상태 추가하는 것도요. done과 비슷한 취급의
dropped, 그리고 in-review상태 정도? 이건 TBD로 구현 시점에 추가 논의"

Today `item.status` changes only the author's own assignment record
(`agents-plugin-tool/internal/wstracker/check.go`, `TypeItemStatus` ->
`needRec(it, a)`), subordinates included, and the settable set is idea,
todo, doing, blocked (done goes through `item.close`). Status ranking and
roll-up live in `internal/wstracker/view.go` (`rank`, item-level status
derivation) and the query filter set in `query.go` (`QueryStatuses`).

## Decisions

1. **A superior may set a defined subset of statuses on a subordinate's
   assignment (user).** The user's example subset is blocked and done,
   treated as wrap-up actions by the superior. The exact subset is settled
   at promotion (see Constraints).
2. **New statuses are in scope for this ticket (user): a `dropped` status
   treated like done (terminal), and an `in-review` status.** Their exact
   semantics are TBD and settled at promotion.

## Constraints

- TBD, to settle with the user before promotion to `ready/` (user: "구현
  시점에 추가 논의"):
  - the exact superior-settable subset and whether "superior" means direct
    superior only or any ancestor in the org tree;
  - how a superior's done relates to `item.close` and to the approval step
    for a follower's done ("완료 대기" / awaiting) already in the store;
  - `dropped`: terminal like done; how it ranks and rolls up, whether it
    counts as complete for parents, how it is reopened, and its query and
    UI treatment;
  - `in-review`: who sets it, where it ranks between doing and done, and
    whether it overlaps the existing awaiting state;
  - whether the quick-add status chip (`261110-feat-ws-dashboard-tracker-quick-add-fields`
    Decision 8) then offers the superior-settable subset for subordinate
    assignees.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.

## Phases

### Phase 1: superior-settable statuses and new statuses

Scope and verification are settled at promotion, after the TBD items above.
