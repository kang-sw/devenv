---
title: Keep terminal panes alive across work-root switches with visibility-gated sockets
---

# Keep terminal panes alive across work-root switches with visibility-gated sockets

## Background

Switching the active work root in the dashboard frontend currently destroys
every terminal pane belonging to the previously active root: a single shared
`DockviewReact` instance recomputes its panel set from
`buildWorkbenchEditorGroups` filtered to the active root only, and
`syncDockviewWorkbench` (`frontend/src/workbench/dockviewLayout.tsx:315-328`)
calls `api.removePanel` on every panel not in that filtered set. This unmounts
`TerminalPaneBody`, which disposes the xterm.js instance
(`frontend/src/App.tsx:6210-6220`) and closes its WebSocket
(`frontend/src/App.tsx:6297-6300`). Returning to the root recreates a fresh
xterm instance and replays the buffered `pane.output`, which produces visible
flicker and loses xterm-internal state (scroll position, selection, search).

Within a single work root, tab switching already avoids this — dockview keeps
inactive panels mounted (hidden via CSS), so this destructive path only fires
at the work-root level and on explicit tab close.

Investigation also found no existing visibility-based throttling of terminal
WebSockets anywhere in the stack: every mounted pane, visible or not, keeps a
live, unthrottled socket for as long as it stays mounted
(`frontend/src/App.tsx:6235-6306`, dependency array `[terminalId]` only), and
the daemon serves every connection without subscriber-count awareness
(`crates/daemon/src/terminal.rs:686-755`). Keeping more panes mounted
long-term (this ticket's goal) makes this an active resource concern, not just
a latent one.

The existing reconnect wire protocol is already delta-based — the daemon
filters output chunks by `sequence > after`
(`crates/daemon/src/terminal.rs:563`, `output_after`) and the frontend passes
`pane.nextSequence - 1` as the resume cursor
(`frontend/src/terminals.ts:162-164`) — but two gaps make it unsafe to rely on
for a background-and-resume flow: `output` frames never update
`pane.nextSequence` client-side (only trailing `status`/`exit` frames do,
`frontend/src/App.tsx:4194-4212`), and the daemon's output ring buffer
(`MAX_OUTPUT_CHUNKS = 1024`, `crates/daemon/src/terminal.rs:27`) evicts by
chunk count, not by time, with no signal to the client when a requested
cursor already fell outside the retained window.

## Decisions

- Work-root switching becomes non-destructive by default: each open work root
  gets its own `DockviewReact` instance, kept alive and hidden (not removed)
  while another root is active. Destruction becomes an explicit, user-owned
  action via a new "close work root" affordance in the left panel, reusing the
  current teardown path.
- No automatic LRU/eviction policy for backgrounded work roots — the user is
  responsible for closing roots they no longer need. This was an explicit
  scope decision to avoid building an eviction heuristic in the same pass.
- Socket liveness is decoupled from mount liveness: a pane can stay mounted
  (xterm instance + buffered state retained) while its WebSocket is closed
  because it isn't currently visible, then resume via the existing
  cursor-based reconnect once visible again.
- Silent data loss on buffer eviction is unacceptable — a reconnect that lands
  outside the retained buffer window must surface an explicit gap marker in
  the terminal output, not a seamlessly-stitched but incomplete stream.

## Phases

### Phase 1: Per-work-root workbench instances (no destroy on root switch)

Replace the single shared `DockviewWorkbenchLayout` mount in `WorkbenchShell`
(`frontend/src/App.tsx:3201`, `frontend/src/App.tsx:4561`) with one instance
per open work root, each independently holding its own dockview panel set
computed from that root's panes only. Inactive roots' instances stay mounted
in the DOM with `display:none` (or equivalent) rather than being torn down.

Constraints:
- `terminalPanes`, `workbenchGroupsByRoot`, `paneOrderByRoot` are already
  root-scoped or filterable by root id — reuse them per-instance rather than
  introducing new state shapes.
- Verify dockview does not require a single global instance for cross-root
  drag/drop; if it does, scope that interaction out for this phase rather than
  redesigning dockview usage.
- No socket-visibility changes in this phase — a pane in a hidden root instance
  keeps its live socket for now (Phase 3 addresses this).

### Phase 2: Explicit "close work root" action

Add a close affordance for open work roots in the left panel. Triggering it
runs the current (pre-Phase-1) destructive teardown for that root's panes
only: dockview panel removal, xterm dispose, socket close, and any
`terminalPanes`/`workbenchGroupsByRoot`/`paneOrderByRoot` entries scoped to
that root. Switching among still-open roots must not trigger this path.

### Phase 3: Visibility-gated terminal WebSocket lifecycle

Extend the terminal socket effect (`frontend/src/App.tsx:6235-6306`) so the
WebSocket closes when its pane becomes invisible — its work root is not the
active one, or (within the active root) its dockview group is not frontmost —
and reopens using the existing cursor-resume mechanism when the pane becomes
visible again. The xterm instance and `TerminalPaneState` must remain mounted
and untouched across this close/reopen; only the socket lifecycle changes.

Depends on Phase 1 (multiple simultaneously-mounted, non-visible panes) to
have a meaningful population of sockets to gate. Should land after Phase 4's
cursor-accuracy fix, or explicitly accept minor duplicate-output-on-resume
until Phase 4 lands.

### Phase 4: Cursor accuracy and gap signaling for reconnect

Two independent fixes to the existing delta-reconnect path:

1. Frontend: update `pane.nextSequence` directly from each `output` frame's
   own chunk sequence (`frontend/src/App.tsx:4194-4212` currently no-ops on
   `output`), instead of waiting for a trailing `status` frame. Removes the
   race where a socket closed mid-batch leaves the cursor stale and causes
   duplicate output on resume.
2. Backend: in `terminal_socket_task`/`send_output_backfill`
   (`crates/daemon/src/terminal.rs`), detect when a client-supplied `after`
   cursor is older than the oldest chunk currently retained in the ring
   buffer (i.e., the requested range was already evicted) and include an
   explicit truncation signal in the response (e.g. a `truncated: true` field
   on the status frame) instead of silently serving only the still-retained
   tail. Frontend renders this as a visible gap marker in the terminal output
   rather than stitching the stream together as if nothing were missing.

No buffer retention policy change (chunk-count vs. byte-size vs. time-based)
is in scope for this ticket — flag as a candidate follow-up if Phase 4's gap
signal fires often in practice.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/` (frontend workbench /
terminal session behavior). Expected caller-visible change: work-root
switching no longer resets terminal UI state; a new explicit "close work
root" action exists; backgrounded terminal sockets disconnect and
silently resume: Contract-first spec: no (spec entry should follow
implementation once the per-phase behavior is settled, since the exact
visibility-boundary and gap-marker UX may shift during implementation).
