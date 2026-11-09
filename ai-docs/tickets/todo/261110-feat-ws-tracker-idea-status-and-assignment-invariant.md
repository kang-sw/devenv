---
title: "Tracker: an idea assignment status replaces the review stage; every task and todo always has an assignee; rule-request fixes"
related:
  261109-feat-ws-tracker-review-stage-and-assignment-defaults: "introduced the review stage (its board (22) a, (52) ok); this ticket retires it"
  261110-feat-ws-tracker-topic-ownership-and-kind-merge: "owned by another session; changes topic assignment and merges task/todo - coordinate, do not edit it from here"
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
2. **Every task and todo always has an assignee (user, (551) a).** When an
   item's last live or pending assignment ends (unassign, decline, the
   lead's return, member removal), the creator holds it; when the creator
   was removed, an admin does. Rejected: leaving `unassigned` as a visible
   exception state ((551) b).
3. **Idea items are left out of progress roll-ups (user, (552)),** as the
   review stage and pending are today (`view.go:418`).
4. **`item.unassign` takes an optional rule id (user, (534) a).** With it,
   only that rule ends (a pending rule request is withdrawn); without it,
   the current behavior stays. The dashboard's withdraw on a pending rule
   request passes the id. Rejected: one open rule per author and root
   ((534) b); README-only fix ((534) c).
5. **A pending rule request does not count as status `pending` (user,
   (535) b).** Go `itemStatus` and the mockup `engine.js` follow the README.
   Rejected: changing the README to match the code ((535) a).

## Constraints

- Stores are event logs shared across machines. Old events carrying
  `stage: "review"` must still fold; how they read is queued as (562).
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

## Open Decision Queue
Last ID: (567)

(554) [open] [policy] An item whose only open assignment is a pending rule request is held by its creator until the request is accepted or declined.
- Context: (535) b says a rule request does not count as `pending`, because it assigns nobody yet; (551) a forbids an item with no assignee. Together the item needs a holder while the request waits.
- Alternative a: the creator holds it (status idea/todo as they set it); it shows in the creator's "mine".
- Alternative b: a pending rule request counts as an assignment for the invariant, with a new derived status (e.g. "requested") left out of roll-ups.
> a: consistent with (535) b's reason, and adds no new derived status.
(555) [open] [technical] The creator fallback is derived in the fold, not written as an assign event - ends come from other writers' events (member removal by an admin, a lead's return, concurrent machines), and a derived holder cannot conflict or double-assign; old unassigned items gain a holder with no migration write. Uncertain: the fold must give the fallback assignment a deterministic record id so later status events find it.
(556) [open] [announced] Item-level statuses `unassigned`, `declined`, and `returned` stop occurring, and the kanban "all" columns for them and for `review` go; the answer still shows in "received requests" and notifications - direct consequence of (551) a, with the scope list in the mockup README line 152.
(557) [open] [announced] The invariant covers tasks and todos only; topics follow `261110-feat-ws-tracker-topic-ownership-and-kind-merge` - user, 2026-11-10: capture tickets only and do not touch the other session's work.
(558) [open] [technical] `tk:item.create` and `tk:item.edit` drop the `stage` attribute and refuse it, over accepting it as an alias for idea - an alias keeps a retired concept in the API; old events still fold.
(559) [open] [technical] No create-time status attribute: creating straight into idea is `tk:item.create` plus `tk:item.status status="idea"` in one batch, over a new `status` attribute on create - no new API surface.
(560) [open] [announced] The dashboard quick-add (Title + Enter, `tracker.js:3317-3333`) creates the task assigned to its creator with status idea - keeps 261109's (53) intent (quick capture lands before todo) under (551) a.
(561) [open] [technical] The fallback assignment starts at status todo, over idea or the ended assignment's status - nobody is working on it any more, and demoting it to idea would hide committed work. Old stage events are the exception ((562)).
(562) [open] [technical] Old events carrying `stage: "review"` fold as "held by the creator, status idea", over folding them as todo - it keeps their pre-todo meaning, and the fold already derives the holder per (555).
(567) [open] [announced] The 2026-11-10 tickets land in `todo/`, standalone with `related:` links - user, 2026-11-10: "다 todo로요"; ready promotion needs fact population and Sage review, not asked for tonight.
