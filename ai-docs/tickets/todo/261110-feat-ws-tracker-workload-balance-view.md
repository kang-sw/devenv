---
title: "Tracker: a lead-side workload balance view of current load per member"
parent: 261110-epic-ws-tracker-team-feedback
---

# Tracker: workload balance view

## Background

From the team introduction feedback (user, 2026-11-10 whiteboard, topic
"tracker 팀 피드백"): leads and managers asked to monitor the balance of
work across team members. The interest was partly about performance
reviews. Today no view aggregates load per member. The view model already
derives per-assignment status and a `stale` flag: a live assignment in
doing or blocked with no status write for `DefaultStaleDays` (3) days
(`internal/wstracker/view.go`, `state.go`).

## Decisions

1. **Visibility follows the org tree (user, (613) a).** A lead sees their
   whole subtree, direct and indirect reports. An admin sees every member.
   Rejected: direct reports only (b); every member sees everyone (c).
2. **The first scope is the current load only (user, (614) a).** Per member:
   live assignment counts by status, stale count, and overdue count (due
   date passed and not done). Rejected: historical throughput and cycle-time
   statistics in the same ticket (b). They are deferred, because which
   numbers a review should count is sensitive and needs its own discussion;
   they can be derived later from the ledger.

## Constraints

- To settle at promotion: where the view lives (a Members panel, a mode of
  the Items panel, or its own drawer tab); whether idea-status assignments
  count, since idea is left out of progress roll-ups by
  `261110-feat-ws-tracker-idea-status-and-assignment-invariant` Decision 3;
  whether creator-fallback holdings count as load; and the drill-down from a
  count to the items behind it.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.

## Phases

### Phase 1: current-load view

Scope and verification are settled at promotion, after the Constraints'
open items.
