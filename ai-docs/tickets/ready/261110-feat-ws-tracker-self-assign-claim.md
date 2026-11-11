---
title: "Tracker: a follower takes work by claiming an opened item or requesting someone else's assignment"
parent: 261110-epic-ws-tracker-team-feedback
related:
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "the creator-fallback hold this ticket's claim ends (Decisions 2 and 7)"
  261110-feat-ws-tracker-workload-balance-view: "may show claim records, under the epic's org-tree visibility"
  261110-feat-ws-tracker-daemon-event-hook: "a claim or take request is an event the hook may deliver"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1399950f881604c3
sage-review-completeness-reviewed: 1399950f881604c3
---

# Tracker: self-assign claim and take request

## Background

From the team introduction feedback (user, 2026-11-10 whiteboard, topic
"tracker 팀 피드백"): work nobody is assigned to, and requests to take over
someone else's assignment, with an incentive angle; or a follower assigning
themselves an item a lead created.

The tracker has no unassigned state any more. A task always has a holder:
when its last assignment ends, the creator holds it by fallback, computed in
the fold and replaced by any explicit assign
(`261110-feat-ws-tracker-idea-status-and-assignment-invariant` Decisions 2
and 7). Only the creator assigns (`internal/wstracker/check.go`,
`TypeItemAssign`); an assignee can hand off. So "work nobody is assigned to"
reads as "work the creator holds by fallback", and a follower has no way
today to take either kind of work themselves.

## Decisions

1. **Both ways of taking work exist (user, (617) c).**
   - The creator can open an item they hold by fallback to a scope. A member
     inside that scope takes it without approval, and the creator's
     fallback hold ends.
   - An item someone holds through an explicit assignment is taken by
     request: the follower asks, and the request is approved before the
     assignment moves.
   Rejected: request only, approved by the creator or the current assignee
   (a); opening only, with no way to ask for someone else's assignment (b).
2. **The opener picks the scope (user, (254) b).** When opening, the creator
   chooses their own org subtree or the whole store.
   Rejected: a scope fixed to the opener's subtree ((254) a); naming the
   members ((254) c).
3. **Only the creator opens (announced, (255)).** Opening delegates like
   assigning, and `item.assign` stays creator-only
   (`261111-feat-ws-tracker-assignee-edit-and-body-merge` Decision 2,
   (229) a, (235) a). When the creator was removed, the `creatorGone`
   fallback applies as it does to edit and assign (technical,
   review-raised).
4. **Either the creator or the current assignee approves a take request
   (user, (256) c).** An assignee's approval is an agreed handoff; a
   creator's is the lead reassigning. Both powers exist today (`item.handoff`,
   `item.assign`).
   Rejected: the current assignee only ((256) a); the creator only
   ((256) b).
   Any store member except the holder of the named assignment may file a
   take request (user, (286) a); a member who already holds a live or
   pending record on the item has nothing to take and is rejected, as
   `item.handoff`'s target check does. A request takes effect only through
   an approval, so an open filing leaks no authority, and a team-scoped
   filing would block the "I want that team's work" signal the incentive
   angle exists for; unwanted requests are declined or withdrawn.
   Rejected: members of the creator's org subtree only ((286) b); members
   under the holder's org superior only ((286) c).
5. **Approval moves the assignment (announced, (257)).** The current
   assignee's assignment ends and the requester's starts; nothing is kept
   alongside, per Decision 1's "approved before the assignment moves". The
   requester's new record starts live and inherits status and note as an
   accepted handoff does (technical, review-raised). When the requester
   already holds a live or pending record on the item by the time the
   approval folds (a concurrent or earlier creator assign), the approval
   still ends the named assignment and adds no second record for the
   requester (technical, review-raised).
6. **Finding opened items (technical, (258)).** An opened item shows an
   "open" chip on its row, and the Items filter and `tracker.query` gain a
   claimable condition. The balance view shows nothing extra in this ticket.
   Rejected: no finder, which leaves opened items undiscoverable; balance-view
   display, which widens `261110-feat-ws-tracker-workload-balance-view`.
7. **Bounds (announced, (259)).** The incentive angle stays a visible claim
   record. Claiming is an exception to "only the creator assigns", and the
   `check.go` check bounds it to an opened item and an in-scope member.
   Event names and payloads are chosen in planning.
8. **Lifecycle (technical, review-raised).**
   - An item is claimable while it is opened and still fallback-held. The
     opening ends when the item is claimed or assigned, and the creator may
     withdraw it. A fallback-held record cannot be handed off (`check.go`
     refuses it), so a handoff of an opened item is rejected as today. Scope membership is evaluated against the
     claim event's causal past, at claim time.
   - A take request names one assignment and lapses when that assignment
     ends. "The current assignee" of Decision 4 is the holder of that named
     assignment, not any assignee of the item. The holder or the creator may
     decline it, and the requester may withdraw it. A request on an item held only by fallback is rejected:
     such an item is taken by opening and claiming.
   - Opening, claiming, requests, approvals, declines, and withdrawals are
     recorded in the item's history, which is the visible claim record of
     Decision 7.
   Rejected: an opening that outlives the fallback hold, which would let a
   later claim displace an explicit assignment; a request that stays
   pending forever.
9. **Concurrency: one winner in fold order (user, (283) a).** An opening
   admits one claimer. Two concurrent claims of one item: the first in fold
   order takes it, and the later is flagged with a reason naming that
   another member already claimed it. An approval lapses only when the
   assignment it names has already ended earlier in fold order: by a
   concurrent approval, a handoff accept, an unassign, or a return. A
   concurrent creator assign ends no assignment (`applyAssign` only adds
   records) and makes no approval lapse. A claim folding after the
   creator's assign or withdrawal lapses, since the opening has ended; when
   the claim folds first, a withdrawal is a no-op and an assign adds its
   records beside the claimer as `item.assign` always does, so no assign
   ever lapses (technical, review-raised). Each flag reason has a
   `reasons.json` case. This matches the concurrent-cycle clause of
   `261110-feat-ws-tracker-item-move-reparent` Decision 3.
   Rejected: letting every concurrent claimer become an assignee ((283) b),
   which blurs who took the work and leaves the incentive record
   ambiguous.

## Constraints

- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.
- `261110-feat-ws-tracker-daemon-event-hook` may land first: if it has,
  this ticket adds a take request on the member's assignment or item, and a
  claim or approval that affects the member, to the hook's "concerns me"
  set; if it has not, the hook ticket does so when it lands.
- A later concurrent claim is flagged during the fold, not by `check`:
  replay `verdicts` (`replay.go`) judges an event only against its causal
  past, where a concurrent claim cannot fail. This needs a fold-time flag
  in Go replay and engine.js, the same mechanism the move ticket's
  concurrent-cycle flag needs; whichever lands second reuses the first's
  mechanism and its `reasons.json` path, and `TestFlaggedReasons` covers
  fold-time reasons.
- Golden files: view JSON fields carry no `omitempty`, so a new open or
  claim field changes every file under `testdata/golden`; regenerate them
  from the reference engine after each merge, never merge them by hand, and
  give new case files a distinct name prefix.
- The tracker MCP tool grammar (`rsrc/tracker/tracker.md`) and its wsflow
  mirror change with any new command: `ai-docs/manuals/wsflow-mirroring.md`
  and `ai-docs/manuals/shipped-surface-boundary.md` apply. (A third copy,
  `agents-plugin-pi/rsrc/tracker/tracker.md`, also exists; the assignment
  invariant updated tracker.md "in all three packages", per its Result.)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 4890b640 (2026-11-10, commit): "User answered (617) c on the whiteboard: both mechanisms - creator opens a fallback-held item to a scope for approval-free claim, and an explicitly held assignment is taken by request plus approval." — bearing: supports
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Decisions): "The creator fallback is derived in the fold, not written as an assign event ((555)). ... a derived holder cannot conflict or double-assign" — bearing: constrains
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Result): "Fold (Go and engine.js in lockstep): a task nobody else holds gets a derived fallback record ... held by the creator or, when the creator was removed, the first admin in the roles admin order." — bearing: constrains
- e11589f5 (2026-11-08, commit): "Handoff is a separate `item.handoff` event rather than `item.assign` by an assignee, so the creator-only rule on item.assign stays simple." — bearing: constrains
- 261111-feat-ws-tracker-assignee-edit-and-body-merge (2026-11-11, Decisions): "`item.edit` opens to the item's creator or a member holding a live assignment on it ... `item.hide` and `item.assign` stay creator-only." — bearing: constrains
- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Decisions): "Agents self-assign what they create ... the MCP writer adds the self-assignment the dashboard adds" — bearing: constrains
- c536206a (2026-11-10, commit): "The topic one-holder check runs after the creator and completion-shape checks, so every malformed topic payload gets the single topicOne reason rather than a mode-specific one." — bearing: constrains
- 4449ed91 (2026-11-08, commit): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, state.go, view.go, query.go, testdata; ai-docs/ref/tracker-mockup/engine.js; agents-plugin-tool/internal/mcp/tracker_tools.go; agents-plugin-tool/internal/wsdashboard/assets/tracker.js; agents-plugin-tool/internal/wsi18n/tables/*.json; three rsrc/tracker/tracker.md copies |
| scope.surface | public-interface | new tracker MCP commands in the shipped tracker.md grammar and new ledger event types |
| scope.new_public_symbol | yes | open, claim, and take-request event types and tracker.update commands; names chosen in planning (Decision 7) |
| scope.new_type_contract | yes | ledger payloads for open, withdraw, claim, request, approve, decline; the view gains an open state; shapes chosen in planning |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/*_test.go, testdata/reasons.json and golden, internal/wsi18n/keys_test.go |
| complexity.reuse_points | confirmed | fallback record (state.go fallbackHolder), the item.handoff check path, and the assignee filter shape in query.go |
| complexity.side_effect_risk | high | adds an exception to creator-only item.assign authorization in check.go and must end a fold-derived fallback hold across Go and engine.js |
| risk.correctness | high | concurrent claims and approvals in a replicated ledger fold with Go and engine.js golden parity |
| risk.fit | moderate | an exception to creator-only assign inside check.go, bounded by Decisions 7-8 |
| risk.test | moderate | golden parity and reasons.json cases must cover new rejection reasons |
| risk.security_or_contract | high | widens who may assign; check must bound claims to opened items and in-scope members |

## Phases

### Phase 1: claim and take request

Scope:
- Opening an item to a scope, claiming it, and the take request with its
  approval (Decisions 1-5), in the Go domain, the reference engine
  (`ai-docs/ref/tracker-mockup/engine.js`) with its golden cases, the
  `tracker.update` grammar, the playbook in all three packages, and the
  dashboard (open action, claim, request and approve, the "open" chip, the
  claimable filter; Decision 6), with the lifecycle and concurrency of Decisions 8-9.

Verification:
- Tracker tests: an in-scope member claims an opened item and the creator's
  fallback hold ends; an out-of-scope member's claim and a claim on an item
  not opened are rejected; only the creator opens; a take request approved
  by the creator or by the current assignee moves the assignment and ends
  the old one; a request approved by anyone else is rejected; any store
  member may file a request, except the named assignment's holder and a
  member already holding a live or pending record on the item, whose
  requests are rejected.
- Lifecycle tests: the creator withdraws an opening; an opening ends on
  claim and on assign; a handoff of an opened item is rejected; a request lapses when its assignment ends; the
  holder and the creator decline and the requester withdraws a request; a
  request on a fallback-held item is rejected; the history records each
  step; with the creator removed, the fallback admin opens.
- Golden cases from the reference engine pin two concurrent claims, two
  concurrent approvals, an approval concurrent with a handoff, the creator
  assigning the requester directly while the holder approves, and a claim
  concurrent with the creator's assign and with a withdrawal (each order).
- Opening an item that has a live explicit assignment is rejected; on a
  multi-assignee item only the named assignment's holder (or the creator)
  approves.
- Golden cases from the reference engine agree with the Go domain.
- `tracker.query` claimable filter test; a browser check of open, claim,
  request, and approve.
