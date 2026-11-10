---
title: "Tracker: a follower takes work by claiming an opened item or requesting someone else's assignment"
parent: 261110-epic-ws-tracker-team-feedback
related:
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "the creator-fallback hold this ticket's claim ends (Decisions 2 and 7)"
  261110-feat-ws-tracker-workload-balance-view: "may show claim records, under the epic's org-tree visibility"
  261110-feat-ws-tracker-daemon-event-hook: "a claim or take request is an event the hook may deliver"
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

## Constraints

- To settle at promotion:
  - what an opened scope is (org subtree depth, named members, everyone in
    the store) and who may open an item besides its creator;
  - who approves a take request (the creator, the current assignee, or
    either), and what happens to the current assignee's assignment on
    approval (ended, or kept alongside);
  - the ledger event types and the `check.go` authorization rules; claiming
    is a new exception to "only the creator assigns", so the check must
    bound it to an opened item and an in-scope member;
  - how an opened item is shown and found (an Items filter, a chip), and
    how claims and requests appear in the balance view under the epic's
    org-tree visibility;
  - whether the incentive angle from the feedback needs anything beyond a
    visible claim record; the first scope assumes it does not.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.
- The tracker MCP tool grammar (`rsrc/tracker/tracker.md`) and its wsflow
  mirror change with any new command: `ai-docs/manuals/wsflow-mirroring.md`
  and `ai-docs/manuals/shipped-surface-boundary.md` apply.

## Phases

### Phase 1: claim and take request

Scope and verification are settled at promotion, after the Constraints'
open items.
