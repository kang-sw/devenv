---
title: "Keep dashboard streams through transient board or session read failures"
related:
  261114-chore-ws-dashboard-index-removal-diagnostics: "missing timestamp/reason telemetry limits historical attribution"
  261114-bug-ws-shared-daemon-restarts-without-a-swap: "separate daemon-replacement investigation; the observed process did not restart"
---

# Keep dashboard streams through transient board or session read failures

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

## Decisions

- MAIN relayed the approved repair direction on 2026-11-14: distinguish
  authoritative not-found/identity mismatch from transient read failures such
  as lock timeout or I/O error. Genuine missing/mismatched sessions may be
  gone; a transient failure keeps the existing page and retries the stream
  instead of permanently terminating it.
- This owner-visible bug follows the current Important Pi startup and
  prompt-admission pair, ahead of remaining orphan bugs and the added backlog.
- Moving expensive render/publication outside the folder lock is a design
  review topic, not a selected implementation. Preserve ordered publication
  and atomic authoritative state if the lock boundary changes.

## Compatibility analysis

Target **T0** if error classification remains internal and existing stream,
registration and session identity contracts remain intact: `gone` continues
meaning genuine absence/mismatch, while transient reads use the existing
retry/keep-page path. No new wire field or event is required by the approved
direction. A new event/field would require additive old-reader handling and
explicit review; reinterpreting a current protocol, store or event is outside
this target and stops for MAIN's settlement before implementation.

## Constraints

- This is still a documentation/design capture under the load throttle. No source
  fix, new build/test, live lock hold, process action, restart or daemon swap
  is performed or authorized by this ticket.
- Preserve genuine missing/deleted-session handling. Retry timing, source-side
  last-good-record caching, lock changes and any wire changes need technical
  design review and an isolated deterministic reproduction; they are not
  chosen by the approved retry/keep-page behavior alone.
- Investigate render frequency and contention separately from CPU attribution:
  the short OS sample showed rendering work, not its share of sustained CPU.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before future tooling edits.
  Settle the compatibility tier before any changed stream/validity contract.

## Phases

### Phase 1: Distinguish transient reads and preserve the valid reader

First use isolated state and controlled contention to establish the
timeout-to-gone path and its timing without touching the shared daemon.
Through design review, classify currently collapsed lookup errors and define
how transient failures preserve the page and retry while genuine absence or
identity mismatch remains terminal. Evaluate whether expensive render and
publication can safely leave the folder lock without allowing older output to
overwrite newer state. Implement only after that review and the assigned heavy
slot; return any broader behavior/compatibility decision through MAIN.

Verification must distinguish actual deletion, malformed records, permission
errors and lock contention; prove no partial authoritative commit is required;
and include subsequent reader recovery. Record historical confidence limits,
exact commands and any acceptance gaps. No implementation Result is claimed.
