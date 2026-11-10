---
title: "Tracker quick-add row lets the user set status, assignee, and schedule before Enter"
related:
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "sets the quick-add defaults (creator-held, status idea, its Decision 11) this ticket extends"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f73d1bc2dea89865
sage-review-completeness-reviewed: f73d1bc2dea89865
completed: 2026-11-10
---

# Tracker quick-add: status, assignee, and schedule at entry time

## Background

User, 2026-11-10 whiteboard, note on (560): "빠른 추가 UI도 지금은 제목
정도만 입력하게 되어 있는데, (그 파란 박스요) 조금 확장해서 상태랑 할당,
일정 같은 걸 입력 시점에 정할 수 있으면 좋겠다 싶긴 하네요" - the quick-add
row takes only a title today; they would like status, assignment, and
schedule to be settable when the item is entered.

Today the Items panel's new row (`newRowHtml`, `createNew` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L3377-L3416`)
sends a task as `item.create` plus `item.status $1 idea` in one write
(`asIdea`, no assign: the store's fallback makes the creator its holder) and
a topic as `item.create` plus a self-assign (`heldTopic`); the "all"
kanban's idea-column row (`knewRowHtml`, `createKnew`,
`tracker.js#L3426-L3445`) creates a creator-held idea task the same way
(`selfAssigned` no longer exists: grep of `tracker.js` returns nothing).
`261110-feat-ws-tracker-idea-status-and-assignment-invariant` (done)
retired the review stage and made quick-add create a creator-held task with
status idea (its Decisions 10, 11): no create-time status attribute, so a
status is a `tk:item.status` in the same batch.

## Decisions

1. **The quick-add row gains optional status, assignee, and schedule (due)
   controls (user, note on (560)).** Left untouched, the row behaves as
   the idea-status ticket's Decision 11 says.
2. **The values travel as ordinary commands in the create's batch
   (`item.create`, `item.assign`, `item.status`; due per Decision 5) ((573)).**
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
5. **A due date set at quick-add travels in `item.create`'s existing `due`
   field (`ItemCreate.Due`, `internal/wstracker/payload.go`), not as a
   separate `item.edit`.** The field already exists, so this adds no API
   surface and saves one command; it refines Decision 2's command list.
   Rejected: an extra `item.edit` due command.
6. **When the row's kind is topic, the status and due chips are hidden and
   the assignee chip picks the topic's single holder, defaulting to me
   (replacing the default self-assign).** Topics carry no status
   (`canStatus` excludes them) and the UI shows no due for them (`dueChip`
   and the item window skip topics), and a topic has exactly one holder.
   Rejected: disabled-but-visible chips (they offer nothing a topic can
   take); a multi-select assignee for topics.
7. **After each create the row keeps the chips' last values until the row
   closes; closing the row restores idea / me / none ((608)).** The row stays
   open after a create (e2bdce49) so several similar items can be entered in
   a row. Rejected: resetting to defaults after every create; keeping status
   and assignee but resetting due.
8. **The status chip shows only while the assignee chip is me alone
   ((609)).** Picking anyone else hides it and sends no `item.status`; each
   assignee then starts in the store's own state (todo for a subordinate,
   pending for anyone else), since `item.status` changes only the author's
   own assignment (`wstracker/check.go` `needRec`), subordinates included.
   With me alone the batch is `item.create` plus one `item.status $1
   <chosen>`, the chosen status replacing Decision 11's idea (never two
   status commands). When the status chip reappears after the assignee
   returns to me alone, it shows its last value (Decision 7). Switching the
   kind select to topic collapses a multi-person assignee to me unless
   exactly one person is chosen. Phase 1's
   "lands with that status" check applies to the me-alone case; with other
   assignees it checks their store start state. Rejected: status applied to
   my own record only and labelled so; reset to idea with an inert chip.

## Constraints

- Lands after `261110-feat-ws-tracker-idea-status-and-assignment-invariant`
  Phase 2, whose quick-add defaults and status set it builds on.
- Only the Items panel's new row (`newRowHtml` / `createNew`) gains the
  chips; the "all" kanban's idea-column row (`knewRowHtml` / `createKnew`)
  stays title-only. Update the comment above `newRowHtml` ("Priority, due,
  and assignees are set afterwards") when touching it.
- The item-menu pickers read `VS.items[MN.item]`; the worker adapts them to
  a draft state for a row with no item yet (implementation choice). Subtree
  assignment rules (`asgOpts`) are left out of the quick-add assignee
  picker.
- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Result): "Both quick-add rows send create plus `item.status $1 idea`. A topic still self-assigns." — bearing: supports
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, Decisions): "No create-time status attribute ((559)). Creating straight into idea is `tk:item.create` plus `tk:item.status status=\"idea\"` in one batch. Rejected: a new `status` attribute on create" — bearing: constrains
- eb3a1634 (2026-11-10, commit): "i18n-en.js regenerated with a script verified to reproduce HEAD's file byte for byte from HEAD's en.json (local go regen is not allowed); TestTrackerMockupI18nIsCurrent passes remotely." — bearing: constrains
- c1ade9b1 (2026-11-10, commit): "Single-select is enforced in asgToggle (clear before push) instead of a separate topic picker, keeping one assign popover; rules are dropped from asgOpts for topics." — bearing: constrains
- 679b006e (2026-11-08, commit): "submenus (priority, due, handoff, note, assign) are drill-in steps in the same panel with a Back row, not cascading flyouts, so one panel holds focus trapping and works on touch/narrow widths." — bearing: supports
- e2bdce49 (2026-11-08, commit): "The popover reuses the item menu's assign step (openAssign unchanged), so placement, Back, focus return and Esc come from the menu machinery instead of a second popup system." — bearing: supports
- e2bdce49 (2026-11-08, commit): "After a create, the row stays open in place and the next item goes after the one just made (prep receives commit results for that)." — bearing: constrains
- 70ba4eab (2026-11-10, commit): "User (whiteboard 2026-11-10) asked the tracker status picker to lay out horizontally without wrapping; .mst was a 4-column grid so the fifth option wrapped." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker.js, tracker.css likely, agents-plugin-tool/internal/wsi18n/tables/*.json, ai-docs/ref/tracker-mockup i18n-en.js regenerated when en keys are added |
| scope.surface | internal | dashboard frontend only; no Go export, MCP tool, or tk command change |
| scope.new_public_symbol | no | none; new i18n keys only |
| scope.new_type_contract | no | reuses item.create (incl. its due field), item.assign, item.status payloads per Decisions 2 and 5 |
| scope.test_surface | existing | wsi18n/keys_test.go and i18n_test.go parity, wsdashboard/tracker_mockup_i18n_test.go; behavior checked by headless browser, no JS unit tests |
| complexity.reuse_points | unconfirmed | item menu steps due/assign and status in itemMenu tracker.js#L3018-L3042 and setStatus#L3258 are bound to an existing item id MN.item, so reuse for a not-yet-created row needs adaptation |
| complexity.side_effect_risk | moderate | createNew batch with $1 refs and the row-stays-open placement after commit results must keep working; topic kind path heldTopic differs |
| risk.correctness | moderate | batch ordering with $1 refs, default-left-alone must stay identical to asIdea, assignee other than me interacts with the creator fallback holder |
| risk.fit | moderate | picker machinery is item-anchored menu state; chips must fit the one-row layout and the kind select |
| risk.test | moderate | verification is a headless browser check plus i18n parity; no automated JS test for the batch contents |
| risk.security_or_contract | low | no new API surface; commands go through existing store validation |

## Phases

### Phase 1: quick-add controls

The controls on the Items panel's new row, sent with the create in one
batch. Verification: `go test ./...` under `agents-plugin-tool` (i18n
parity, including `TestTrackerMockupI18nIsCurrent` after regenerating the
tracker-mockup `i18n-en.js` for new English keys) and a headless-browser
check on an isolated daemon that:
- a quick-add with each control set lands with that status, assignee, and
  due date;
- chips left at their defaults send exactly the idea-status ticket's
  Decision 11 batch (Decision 2);
- with kind topic, the status and due chips are hidden and the chosen single
  holder is sent (Decision 6);
- with assignees other than me alone, no `item.status` is sent and each
  assignee starts in the store's state (todo for a subordinate, pending
  otherwise) (Decision 8);
- after a create the chips keep their values, and closing the row restores
  the defaults (Decision 7).

### Result (bf059c46) - 2026-11-10

Landed: the Items panel's new row has three chips (status, assignee, due). Each chip opens the item menu's own picker over a draft of the row (`MN.kind 'new'`, read through `mnItem()`/`draftItem()`). The status picker offers idea, todo, doing, and blocked. The due picker is the item menu's due step. The assign picker lists people only, no subtree rules.

What the row sends:
- Defaults: the same batches as before. A task sends `asIdea`, which is create plus `item.status $1 idea`. A topic sends `heldTopic` with the creator.
- A due date goes in `item.create`'s `due` field.
- With the viewer alone as assignee: one `item.status $1 <pick>`.
- With any other assignee set: one `item.assign {targets, completion}` and no status. The completion is the one picked in the picker, else assigner.
- Topic kind: the status and due chips are hidden. The assignee chip picks the one holder. Switching the kind to topic collapses several people to the viewer.

Chip state: chips keep their values across creates and across hover-insert moves. Closing the row restores idea / me / none. The kanban idea-column row is still title-only.

i18n: five `js.tk.new.*` keys in every table. The mockup `i18n-en.js` was regenerated.

Decisions:
- `done` is not in the quick-add status picker, because `item.status` refuses it (`wstracker/check.go`).
- For non-me assignees, the completion defaults to assigner, following `asgCompl`'s rule.

Verification:
- `go test ./...` under `agents-plugin-tool`, run remotely through ws-rgo: pass, including `TestTrackerMockupI18nIsCurrent` and the wsi18n parity tests.
- Headless browser (CDP, chrome-headless-shell) on isolated daemons: separate WS_CACHE_HOME/WS_CONFIG_HOME, ports 47811-47813, ws-mcp built remotely. Members: ada (admin), bob (lead ada), cy.
  - Defaults sent exactly create + status idea.
  - doing + due sent create{due} + status doing, and the store shows ada's record at doing with the due date set.
  - bob + cy sent create + assign{targets, assigner} with no status. In the store bob is live at todo and cy is pending.
  - Topic kind hid status and due, collapsed two people to ada, and sent bob as the single holder.
  - Chips kept their values after creates and reset on close. A default topic sent the creator assign.

Review: lite, clean. One minor observation: no automated JS test pins the batch shapes.

## Sage Review Round 1 (2026-11-10)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Status chip has no defined meaning when the assignee is not the creator | important | missing |
| 2 | Pickers are bound to an existing item | minor | autonomous |
| 3 | Unclear whether the kanban idea-column row is in scope | minor | autonomous |
| 4 | Phase verification omits the mockup i18n regeneration | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Chip state after a create is not specified | minor |
| 2 | Kanban idea-column row is not explicitly in or out of scope | minor |
| 3 | Decision 2's command list and Route Facts still name item.edit due | minor |
| 4 | Holder outcome for a non-default assignee is not in the acceptance check | minor |
| 5 | Verification leaves out the default and topic cases | minor |
| 6 | Pickers are tied to an existing item | minor |
