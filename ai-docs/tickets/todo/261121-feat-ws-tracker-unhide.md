---
title: "tracker: 숨긴 항목·댓글 되돌리기"
---

# Tracker: unhide hidden items and comments

## Background

Users complained that a hidden tracker item or comment can never be brought
back (owner, board (1478)=c, 2026-11-21). Verified on develop 872a7833b:

- The event types include `item.hide` and `comment.hide` but no reverse
  (`agents-plugin-tool/internal/wstracker/payload.go:17,32`).
- `item.hide` folds into the register `it:<id>:hidden` written `true`
  (`wstracker/state_fold.go:107-110`); `comment.hide` sets
  `c.hidden, c.hiddenBy` directly (`state_fold.go:341-344`).
- Hide permissions: an item's creator, or another member when the creator is
  gone (`check.go:903-920`, `creatorGone`); a comment's author only
  (`check.go:1460-1473`). Hiding an item first requires its open
  assignments to be unassigned (`check.go:912-920`).
- A hidden item rejects edits, child creation, and moves (`check.go:134,
  152, 833, 855`); the dashboard menu offers hide only to the creator
  (`tracker-menu.js:222`, `tracker-item-rows.js:280` `canHide`). Opening a
  hidden item shows only a "not visible" toast (`tracker-render.js:887`).
- Replay checks every event and rejects an unknown type
  (`check.go:1590-1591`, `replay.go:305`).

## Open Decision Queue

Last ID: (1493)

(1489) [open] [announced] Unhide is allowed to whoever may hide: an item's
creator, or another member when the creator is gone; a comment's author -
mirrors the existing hide checks (`check.go:903-920, 1460-1473`).
(1490) [open] [technical] New events `item.unhide` and `comment.unhide` over
a toggle field on the hide events - the event log stays append-only and
symmetric, and `item.unhide` can write `false` to the same
`it:<id>:hidden` register so concurrent hide/unhide resolves by the
register's existing rule.
(1491) [open] [technical] Unhiding a child whose parent is still hidden is
rejected with "unhide the parent first" over allowing it - an unhidden
child under a hidden parent would still be unreachable.
(1492) [open] [announced] Compatibility T1, to be confirmed against source
in the phase: an older binary rejects the unknown `*.unhide` event on replay
(`check.go:1591`), so on that binary the item simply stays hidden; no data
changes and the new binary shows it again - same class as earlier
tracker event additions. Verify that a rejected event does not disturb the
fold of later events before settling.
(1493) [open] [policy] Where the user finds hidden items to unhide.

## Constraints

- Unhide restores visibility only; assignments cleared before hiding are not
  restored.
- Read before editing: `ai-docs/manuals/release-compatibility.md` (new
  tracker event type), `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/shipped-surface-boundary.md` (dashboard strings ship).
- New user-visible strings go through `wsi18n` in every shipped language.
- UI work: the owner chooses the worker tier before the UI part starts.

## Phases

### Phase 1: Unhide events and dashboard entry point

- `wstracker`: `item.unhide`, `comment.unhide` payload types, checks, folds,
  view fields; tests next to the existing hide tests.
- Dashboard: the unhide entry point settled by (1493), an "unhide" action on
  a hidden item or comment for those allowed, i18n strings.

Verification:
- Narrow Go tests for `./internal/wstracker` and the tracker-related
  dashboard tests (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, narrow
  `-run`; never the whole `./internal/wsdashboard` package).
- Node tracker tests.
- A compatibility check that replays a log containing `item.unhide` through
  the pre-change fold (or reasons it from source with line citations) for
  the T1 verdict.
