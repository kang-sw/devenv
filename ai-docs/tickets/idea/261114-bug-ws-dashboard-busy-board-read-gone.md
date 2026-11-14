---
title: "Dashboard can treat a busy board read as a permanently gone session"
related:
  261114-chore-ws-dashboard-index-removal-diagnostics: "missing timestamp/reason telemetry limits historical attribution"
  261114-bug-ws-shared-daemon-restarts-without-a-swap: "separate daemon-replacement investigation; the observed process did not restart"
---

# Dashboard can treat a busy board read as a permanently gone session

## Background

During read-only dogfooding on 2026-11-14, MAIN repeatedly saw the localized
`js.banner.gone` message while shared daemon PID 94122 remained unchanged.
Session removal/readdition pairs survive in the dashboard log, but their
missing timestamps and reasons do not identify a failing record or writer.
The later bounded registration/key snapshot was valid and matched.

Source inspection establishes a concrete failure path without malformed data:

- `internal/wswhiteboard/folders.go:101-123` reads an authoritative board
  through `FolderStore.locked`. `:169-190` bounds acquisition of the folder
  lock to two seconds and returns an error on timeout.
- `FolderStore.UpdateAndPublish` holds that same lock through mutation,
  commit and publication (`:139-164`). `internal/mcp/session_state.go:625-650`
  wires the board-output publication callback.
- `internal/mcp/whiteboard_tools.go:252-274` performs
  `renderWhiteboard(..., true)` within the locked update callback;
  `:390-407` does the same for deferred `refreshWhiteboardAfter`.
  The board-output publisher also writes derived HTML/stamp/embed files
  while the lock remains held.
- `internal/mcp/session_layout.go` `loadRecordWith(partAll)` propagates a
  board-open error. `internal/mcp/dashboard_sessions.go:879-895`
  `lookupRecord` skips any errored candidate, retries and returns false;
  it does not distinguish a busy board from a missing session or retain a
  last-good whole-record fallback.
- Failed page validity sends `gone` (`internal/wsdashboard/sse.go:468-474`),
  and `assets/dashboard.js:6341-6345` closes that page stream permanently
  and displays the banner. Ordinary transport loss instead reconnects.

This is a source-backed busy-board failure hypothesis, not a reproduced
historical root cause. Two lookup attempts may both fail under continued
contention; no timed incident trace establishes that they did. Normal record
writes are per-file atomic, and authoritative folder reads/writes share their
lock. Cross-part generations or an unlocked guard read alone are not proof of
a partial-file or partial-board race on macOS.

## Constraints

- Capture only under MAIN's read-only investigation authorization. No source
  fix, new build/test, live lock hold, process action, restart or daemon swap
  is performed or authorized by this ticket.
- Preserve genuine missing/deleted-session handling. Do not silently choose a
  retry, stale-page fallback, lock change or protocol change before design
  settlement and an isolated deterministic reproduction.
- Investigate render frequency and contention separately from CPU attribution:
  the short OS sample showed rendering work, not its share of sustained CPU.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before future tooling edits.
  Settle the compatibility tier before any changed stream/validity contract.

## Phases

### Phase 1: Reproduce and distinguish busy-board reads from missing sessions

Use isolated state and controlled contention to establish the timeout-to-gone
path and its timing without touching the shared daemon. Identify which errors
are currently collapsed by session lookup and whether an existing recovery
boundary can preserve a valid reader without hiding deletion. Return the
behavior/compatibility decision through MAIN before implementation.

Verification must distinguish actual deletion, malformed records, permission
errors and lock contention; prove no partial authoritative commit is required;
and include subsequent reader recovery. Record historical confidence limits,
exact commands and any acceptance gaps. No implementation Result is claimed.
