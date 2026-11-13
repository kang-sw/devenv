---
title: "Tracker item window silently drops unsaved edits when it closes"
completed: 2026-11-13
---

# Tracker item window drops unsaved edits on close

## Observation (2026-11-10)

Reported out of scope by the delegate that implemented the tracker
`wiki:<name>/<path>` links: "The item window drops its own unsaved edits
silently when it closes; that is outside this ticket and was not changed."

A user editing an item's body or a field in the tracker item window, then
closing the window (close button, Escape, clicking away, or opening another
item), loses the edit with no prompt or draft.

## Decisions

- 2026-11-13 owner decision (board item (50) = a): keep unsaved edits as a client-side draft per store and item on close. Restore it on reopening with a small cue and a discard action; successful save clears it. Reject confirm-on-close and autosave to the store.
- Compatibility: T0, dashboard assets only; no shared store or protocol change.

## Phases

### Phase 1: Preserve item drafts

Preserve unsaved item-window edits client-side across close and reopen, with restoration and discard affordances. Verify dashboard behavior with local JavaScript tests and remote Go dashboard tests; server changes are out of scope.

### Result (3bb728edb) - 2026-11-13

- Per-store and item drafts restore with a discard action, retain merge metadata, and clear after an applied save. Browser storage failures fall back to page-lifetime memory.
- Local wrapped JavaScript syntax check and seven tests passed; remote w5090 `go test ./internal/wsdashboard/...` passed. Self-review led to durable local storage and save-race coverage.
- Forward: Localize the restored draft cue and discard action with the tracker translation catalog.

## Constraints

- Frontend change in the tracker dashboard assets; run JavaScript tests locally under `sys_lock.sh` and Go dashboard tests remotely on w5090.
