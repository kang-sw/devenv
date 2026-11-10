---
title: "Tracker quick-add row lets the user set status, assignee, and schedule before Enter"
related:
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "sets the quick-add defaults (creator-held, status idea, its Decision 11) this ticket extends"
---

# Tracker quick-add: status, assignee, and schedule at entry time

## Background

User, 2026-11-10 whiteboard, note on (560): "빠른 추가 UI도 지금은 제목
정도만 입력하게 되어 있는데, (그 파란 박스요) 조금 확장해서 상태랑 할당,
일정 같은 걸 입력 시점에 정할 수 있으면 좋겠다 싶긴 하네요" - the quick-add
row takes only a title today; they would like status, assignment, and
schedule to be settable when the item is entered.

Today the Items panel's new row (`newRowHtml`, `createNew` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js` near L3318-L3333)
sends `item.create` plus a self-assign (`selfAssigned`) in one write; the
kanban review-column row (`knewRowHtml`) creates a review-stage task.
`261110-feat-ws-tracker-idea-status-and-assignment-invariant` retires the
review stage and makes quick-add create a creator-held task with status
idea (its Decisions 10, 11): no create-time status attribute, so a status
is a `tk:item.status` in the same batch.

## Decisions

1. **The quick-add row gains optional status, assignee, and schedule (due)
   controls (user, note on (560)).** Left untouched, the row behaves as
   the idea-status ticket's Decision 11 says.
2. **The values travel as ordinary commands in the create's batch
   (`item.create`, `item.assign`, `item.status`, `item.edit` due) ((573)).**
   The idea-status ticket's Decision 10 already chose no create-time status
   attribute, and no API surface is added. A control left at its default
   adds no command beyond the idea-status ticket's Decision 11 batch (the
   creator is already the derived holder there, its Decision 7). Rejected:
   new create-time attributes.
3. **The controls are compact chips on the row (status, assignee, due) that
   open the same pickers the item window uses, defaulting to idea / me /
   none ((574)).** One row stays the quick path, and the pickers already
   exist. Rejected: a second, expanded form.
4. **This lands as its own ticket with a `related:` link to the idea-status
   ticket, not a phase there ((572)).** The idea-status ticket's scope is
   the assignment invariant.

## Constraints

- Lands after `261110-feat-ws-tracker-idea-status-and-assignment-invariant`
  Phase 2, whose quick-add defaults and status set it builds on.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.

## Phases

### Phase 1: quick-add controls

The controls on the Items panel's new row, sent with the create in one
batch. Verification: `go test ./...` under `agents-plugin-tool` (i18n
parity) and a headless-browser check on an isolated daemon that a quick-add
with each control set lands with that status, assignee, and due date.
