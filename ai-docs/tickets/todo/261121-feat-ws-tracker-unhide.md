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

## Decisions

- Unhide is allowed to whoever may hide: an item's creator, or another member
  when the creator is gone; a comment's author - mirrors the existing hide
  checks (`check.go:903-920, 1460-1473`) ((1489), owner ok 2026-11-21).
- New events `item.unhide` and `comment.unhide`; `item.unhide` writes `false`
  to the same `it:<id>:hidden` register, so a concurrent hide/unhide resolves
  by the register's existing rule ((1490), owner ok).
  Rejected: a toggle field on the hide events (breaks the symmetric,
  append-only event vocabulary).
- Unhiding a child whose parent is still hidden is rejected with "unhide the
  parent first" ((1491), owner ok).
  Rejected: allowing it (the child would stay unreachable under the hidden
  parent).
- Compatibility T1 (owner (1492) ok; scope widened by (1494)=a,
  2026-11-21 18:45). An older binary rejects the unknown `*.unhide` event on
  replay (`check.go:1590-1591`), so on that binary the item stays hidden and
  later edits, comments, and replies on it are shown flagged, because causal
  replay skips the rejected event (`replay.go:401-419`; `check.go:846-857,
  1447-1474`). Those verdicts are computed in memory at replay and never
  written (`replay.go:295-321`, `view.go:594`), the older binary refuses its
  own new writes to the still-hidden target, and the new binary replays all
  of it correctly: display-degraded, no data change.
  Rejected: a T3 boundary with a minor bump (locks out every previous-minor
  install for a downgrade-only symptom); deferring the feature.
- Entry point: a "show hidden" toggle on the tracker list. When on, hidden
  items and comments render dimmed in place, and those allowed to unhide see
  an "unhide" action on them ((1493)=a, owner 2026-11-21 18:41).
  Rejected: a separate hidden-items dialog in the store menu (loses the
  in-place context); an unhide button only on the "not visible" toast (no way
  to discover hidden items).

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
- Dashboard: the "show hidden" list toggle, dimmed hidden items and
  comments, an "unhide" action for those allowed, i18n strings.

Verification:
- Narrow Go tests for `./internal/wstracker` and the tracker-related
  dashboard tests (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, narrow
  `-run`; never the whole `./internal/wsdashboard` package).
- Node tracker tests.
- A compatibility check that replays a log containing `item.unhide` through
  the pre-change fold (or reasons it from source with line citations) for
  the T1 verdict.
