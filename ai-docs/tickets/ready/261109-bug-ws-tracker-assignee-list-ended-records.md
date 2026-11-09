---
title: Tracker assignee list stacks ended records on reassign
related:
  261108-feat-ws-tracker-drawer-ui: the item window's assignee section this fixes
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c1e26acebf1b8b55
sage-review-completeness-reviewed: c1e26acebf1b8b55
---

# Tracker assignee list stacks ended records on reassign

## Background

User report (board, 2026-11-09): removing an assignee and adding the same
person again stacks rows. The item window's assignee section showed "담당자 3":
the member's live row ("할 일") plus two rows labelled "미배정", each carrying
an `item.unassign` event alias (`...-128`, `...-130`).

The fold keeps ended assignment records by design: a record's id is
`<assign event>><member>`, so every assign starts a new record and an unassign
ends it without deleting it. The engine parity and golden tests rely on this.
The defect is in the browser: `assigneesHtml` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js` renders every
record, ended ones included, and the header count is `rows.length`. The ended
row's state pill comes from `recChip`, which for a non-live record calls
`stChip(r.state)`; for an ended record that renders the item-status label
"미배정" (an item with no assignee) rather than the end-state text that
`END_KEY` (`js.tk.end.*`, e.g. "배정 해제됨") already holds.

## Decisions

1. **Display rule (board 135 a).** In the assignee list, an ended record is
   hidden when the same member holds a live or pending record on the item.
   Otherwise only the member's most recent ended record is kept, shown dimmed
   (the existing `gone` row style). The header count counts only the rows
   shown. Repeating unassign and reassign therefore leaves at most one row per
   member.
   Rejected: hiding every ended record (b: a just-removed member disappears
   with no trace in the item window); folding ended records under a "past
   assignments N" disclosure (c: more UI for history the activity feed
   already holds).
2. **Browser-only fix (board 136).** The fix is a pure filter function in
   `tracker.js` applied to the item's records before `assigneesHtml` builds
   rows; the Go fold, `view.go`, the reference engine, and the golden files
   stay unchanged. Rejected: changing the fold (breaks engine parity, the
   golden tests, and the concurrency paths that read ended records such as
   `onEnded` / `reverseHanded`); a flag computed in `view.go` (needs the
   golden files regenerated for a display-only rule).
3. **End-state pill text (board 137).** An ended record's pill shows its
   end-state text from `END_KEY` (`endText`) instead of `stChip(r.state)`.
   Rejected: keeping `미배정`, which is the item-status icon table's key for an
   item without assignees and misreads on a member row.

## Constraints

- Only the assignee section of the item window changes. Activity, inbox,
  notifications, and the fold keep every ended record.
- Row actions on live and pending rows (unassign, withdraw, reopen, handoff)
  keep their current conditions.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- ad198a0f (2026-11-08, commit): "Tie-break for the assignee order: lead-chain depth ascending (own walk of `leads[0]`...), then live/pending before ended, then display name (localeCompare), then assignment order." — bearing: constrains
- 4e66fb8e (2026-11-08, commit): "The minus badge keeps data-act unassign (the QUICK path) and the existing labels as tooltip and aria-label: asg.unassign, asg.unassign.admin, and asg.withdraw for the pending-withdraw row" — bearing: constrains
- 78af82e6 (2026-11-08, commit): "Notification mapping uses event ids already in the view state: ... declined / returned -> `ended.id`" — bearing: constrains
- 46731727 (2026-11-08, commit): "Handoff records do not inherit `rule`; itemStatus ties pick the first ended record in both engines (the engine now matches Go's order)." — bearing: constrains
- 261108-feat-ws-tracker-data-structure (2026-11-08, Decisions): "Pending handoff whose original ended ((295) a). When the original assignment of a pending handoff ends another way (unassign, removal, return), the pending handoff is ended as `withdrawn` in the derived state" — bearing: supports
- 9aa78d50 (2026-11-08, commit): "Data tables (status set, modes, scopes, Today chips, tabs, families, kinds, fields, end states) hold key literals and translate at the use site, so every key stays a literal the keys_test sweep can see" — bearing: constrains
- 1e1a91a0 (2026-11-09, commit): "The relay narrowing test executes relayHref under node, skipping when node is absent, and string-pins the gate and the fetch shape." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker.js plus a new or extended Go test in agents-plugin-tool/internal/wsdashboard |
| scope.surface | internal | browser-internal functions assigneesHtml and recChip in tracker.js#L293-L296 and #L1606-L1638; no Go export changes |
| scope.new_public_symbol | no | visibleRecs is a closure-local JS helper, not exported |
| scope.new_type_contract | no | reads existing view fields state and ended {id,type,ts,by} from wstracker/view.go#L141-L146 |
| scope.test_surface | new-files | node-executed Go test following agents-plugin-tool/internal/wsdashboard/relay_test.go#L51-L84; no existing test covers assigneesHtml |
| complexity.reuse_points | confirmed | endText and END_KEY at tracker.js#L335-L336; gone row class at tracker.js#L1628; relayHref node harness in relay_test.go |
| complexity.side_effect_risk | low | display-only filter inside the item window's assignee section; fold, view.go, and other renderers untouched |
| risk.correctness | moderate | per-member grouping, latest-ended selection, and hid/header count must all switch to the filtered list consistently |
| risk.fit | low | Decisions 1-3 fix rule, location, and pill text; reuses existing helpers and styles |
| risk.test | moderate | node-skip harness means CI without node does not execute the cases; headless browser check is manual |
| risk.security_or_contract | low | no protocol, event, or golden-file change |

## Phases

### Phase 1: Filter ended assignee records and fix the ended pill

- Add a pure function (for example `visibleRecs(recs)`) that returns the
  records the list shows per Decision 1: keep every live or pending record;
  for each member without one, keep only that member's latest ended record
  (latest by `r.ended.ts` compared as parsed times, `Date.parse`, not as
  strings; ties broken by the later position in the record list), dropping
  the rest. The member is the record's `to` field (the member half of the
  `<assign event>><member>` id).
- `assigneesHtml` uses the filtered list for its rows, its empty check, its
  fold count (`hid`), and its header count.
- `recChip` (or the ended branch of the row) renders the end-state text for
  an ended record per Decision 3. `stChip` keys its class on a status
  (`chip st-<state>`), and only `unassigned` has a status entry, so give the
  end-text pill a chip class that does not depend on the end state being a
  status key (the implementer picks the class).
- `focusTarget` (tracker.js, the `item.decline` / `item.return` /
  `item.unassign` branch that looks up the ended record by `data-rec`) falls
  back to the member's visible row (`data-hmember`) when the filter hid that
  record; update its header comment to match.
- Test: execute the filter under node from a Go test, following the
  `relayHref` precedent in `agents-plugin-tool/internal/wsdashboard/relay_test.go`
  (extract the function source, run it with node when installed, skip
  otherwise). Cases: live + two ended for one member -> the live row only;
  two ended for one member -> the latest ended only; ended members across two
  people -> one each; pending + ended for one member -> pending only.
- Verify in a headless browser against a dev daemon with isolated
  `WS_CACHE_HOME` / `WS_CONFIG_HOME` (never the user's daemons on ports 29417
  / 45743): assign, unassign, reassign the same member twice; the list shows
  one row and the header count is 1. Then unassign once more: the one row is
  dimmed, its pill shows the end-state text (`배정 해제됨` in Korean, not
  `미배정`), and the header count is 1.
