---
title: "Tracker: an idea assignment status replaces the review stage; every task always has an assignee; rule-request fixes"
related:
  261109-feat-ws-tracker-review-stage-and-assignment-defaults: "introduced the review stage (its board (22) a, (52) ok); this ticket retires it"
  261110-feat-ws-tracker-topic-ownership-and-kind-merge: "done (bbb14e07c): topics have one holder and no status; the todo kind merged into task"
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
while the item holds a live or pending assignment (`check.go:189-202`), and
any assign promotes it to todo (`state.go:1353`). `itemStatus`
(`wstracker/view.go:362-395`) derives `review`, `pending`, `declined`,
`returned`, or `unassigned` for an item with no live assignment. The holder
can set only todo, doing, blocked, done.

Two carried-over decisions from the 2026-11-09 handoff (board (534), (535))
touch the same derivation:

- `item.unassign` with `root` ends every open rule of the author rooted
  there (`state.go:1149-1151`), so withdrawing a pending rule request from
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
   creator was removed, an admin does. The todo kind merged into task
   (bbb14e07c), and a topic has no status and a single holder who may
   release it (`261110-feat-ws-tracker-topic-ownership-and-kind-merge`
   Decisions 4, 5, 5a), so a topic may be held by nobody and this invariant
   does not touch topics. Rejected: leaving `unassigned` as a visible
   exception state ((552) b).
3. **Idea items are left out of progress roll-ups (user, (551)),** as the
   review stage and pending are today (`view.go:418`).
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
   Its status is the idea or todo the creator set, and it shows in the
   creator's "mine". Rejected: counting the pending rule request as an
   assignment for the invariant, with a new derived status such as
   "requested" left out of roll-ups ((554) b).
7. **The creator fallback is derived in the fold, not written as an assign
   event ((555)).** Assignment ends come from other writers' events (an
   admin's member removal, a lead's return, concurrent machines); a derived
   holder cannot conflict or double-assign, and old unassigned items gain a
   holder with no migration write. The fold gives the fallback assignment a
   deterministic record id so later status events find it. Rejected:
   writing an assign event when an assignment ends.
8. **Item-level statuses `unassigned`, `declined`, and `returned` stop
   occurring, and the kanban "all" columns for them and for `review` go
   ((556), a consequence of Decision 2).** The answer still shows in
   "received requests" and in notifications; the scope list follows the
   mockup README line 152.
9. **`tk:item.create` and `tk:item.edit` drop the `stage` attribute and
   refuse it ((558)).** Old events still fold (Decision 13). Rejected:
   accepting `stage` as an alias for idea, which keeps a retired concept in
   the API.
10. **No create-time status attribute ((559)).** Creating straight into idea
    is `tk:item.create` plus `tk:item.status status="idea"` in one batch.
    Rejected: a new `status` attribute on create (new API surface).
11. **The dashboard quick-add (Title + Enter, `tracker.js:3317-3333`)
    creates the task assigned to its creator with status idea ((560)),**
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
- `tk:` command docs and tool descriptions are shipped text: read
  `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
  before editing `internal/mcp/`.
- Topic assignment belongs to
  `261110-feat-ws-tracker-topic-ownership-and-kind-merge`; do not change
  topic behavior here.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`
  (`js.tk.st.review` today).

## Phases

### Phase 1: rule-request fixes

Decisions 4 and 5: the `item.unassign` rule id (fold, check, `tk:` command,
docs, dashboard withdraw), and the `pending` derivation in `view.go` and
`ai-docs/ref/tracker-mockup/engine.js`. Verification: `go test ./...` under
`agents-plugin-tool`, with scenario tests for withdrawing one of two rules
on a root and for a rule-request-only item's status.

### Phase 2: idea status and the assignment invariant

Decisions 1 to 3 on top of Phase 1's derivation: the idea status in the
fold, check, `tk:` docs, dashboard status picker, kanban columns, and i18n;
retiring the stage field; the creator fallback. Verification: `go test ./...`,
scenario tests for each assignment-ending path landing on the creator (and
on an admin when the creator was removed), and for an old `stage: "review"`
event folding as specified.
