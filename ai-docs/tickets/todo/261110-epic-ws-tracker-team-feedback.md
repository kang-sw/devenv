---
title: "Tracker: features from the first team introduction feedback"
sage-review-design: required
related:
  261110-feat-ws-tracker-superior-settable-status-and-new-statuses: "sibling tracker follow-up from the same week; not a child"
  261110-feat-ws-tracker-item-move-reparent: "manual reorder within a priority level would live there if ever asked for"
---

# Tracker: features from the first team introduction feedback

## Scope

Features asked for when the tracker was first introduced to a team
(user, 2026-11-10 whiteboard, topic "tracker 팀 피드백"). The feedback
came from the introduction, not from use yet; it was mostly from the
manager side and partly about performance reviews.

- A lead-side workload balance view across members.
- A daemon-side tracker event hook. Mail integration and per-PC desktop
  notifications are built on it, but they belong to downstream setups, not
  to this repository.
- A way for a follower to take work themselves (self-assign, or ask to
  take someone else's assignment). The mechanism is still open on the
  whiteboard ((617)), and its child is filed once that is settled.

## Non-Scope

- **Priority levels.** The question at the introduction was whether an item
  can carry a priority. It already can: every item has a `priority`, and an
  admin sets the level names, up to 9, through `roles.set_priority_levels`
  (`internal/wstracker/check.go`). The defaults are Notice / Task / Urgent.
  No work comes from it ((612), recorded as the lead's reading).
- The mail integration itself, which is downstream work on top of the hook.

## Child Tickets

- `261110-feat-ws-tracker-workload-balance-view` - lead-side current
  workload per member, visible down the org tree ((613), (614)).
- `261110-feat-ws-tracker-daemon-event-hook` - the daemon runs a configured
  local command with the event JSON on stdin ((615), (616)).
- Planned: follower self-assign / take-request, after (617) settles.

## Cross-Child Decisions

- Workload visibility follows the org tree: a lead sees their whole subtree
  (direct and indirect reports), and an admin sees everyone ((613) a). Any
  child that exposes per-member work data, such as a claim record shown in
  the balance view, uses the same visibility.

## Completion Criteria

- Done: every child is done or dropped.
- Dropped: the team does not adopt the tracker.
- Deferred: historical metrics for performance review (throughput, cycle
  time). (614) a scoped the balance view to the current load first, so these
  need their own discussion.
