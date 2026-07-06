---
title: Durable workbench sessions - keep-alive across work-root switches and layout/terminal-visual restore on reload
related:
  260523-research-ws-dashboard-persistable-ui-state-map: source of the workbench-layout and terminal-visual-state persistence candidates implemented by this ticket's later phases
related-mental-model:
  - ws-web-dashboard
---

# Durable workbench sessions: keep-alive across work-root switches and layout/terminal-visual restore on reload

## Background

### In-session work-root switching destroys terminal panes

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

### Reload/reconnect loses browser presentation state, not daemon terminal identity

A separate but related investigation into full page-reload behavior found the
daemon-side terminal continuity is already correct: on mount, the frontend
calls `listTerminals()` and reattaches by terminal id to any daemon-alive
sessions (`frontend/src/App.tsx:3529-3541`, `mergeListedTerminalSessions`).
The `terminalRestoreIntents` localStorage fallback
(`frontend/src/terminals.ts:67-73`, key `ws-dashboard.terminalRestore.v1`)
only runs when the daemon reports zero live sessions for a root, and by
design spawns brand-new PTY sessions (it carries no `terminalId`) — this
matches the existing spec's `WorkRoot IO Restore Model`
(`ai-docs/spec/ws-web-dashboard/index.md:1530-1548`), which already documents
that old daemon terminal ids are not treated as resumable across daemon
restart.

What the spec explicitly scopes out, and what reload actually loses today, is
*browser presentation state*: "exact browser workbench arrangement remain
outside the restore model." Concretely:

- Dockview layout — no `toJSON`/`fromJSON` call exists anywhere in the
  frontend; `workbenchGroupsByRoot` and `paneOrderByRoot`
  (`frontend/src/App.tsx:378-383`) are plain in-memory `useState`, lost on
  reload. Group arrangement, tab order, split proportions, and active pane
  are all rebuilt from scratch on every reload.
- Terminal visual buffer — even when the frontend correctly reattaches to a
  still-alive daemon terminal by id, the pane-mount effect
  (`frontend/src/App.tsx:5964` onward) always creates a brand-new xterm
  `Terminal` instance and replays only the plain-text `pane.output` string
  (lines 5989-5993). Scrollback styling, cursor position, and scroll
  viewport offset are never captured or restored — a reload always lands the
  user scrolled to the bottom of an un-styled text replay, even though the
  underlying PTY session and its real output history are untouched.

This gap is broader than terminals: `260523-research-ws-dashboard-persistable-ui-state-map`
maps many more candidate persistence surfaces (file explorer tree state,
Activity Console local state, command palette preferences, dashboard chrome
preferences, root picker history). This ticket implements only two of that
research ticket's "Split Candidates" — workbench layout and terminal visual
state — because they are the surfaces this session's design work directly
grounded; the rest remain open under that research ticket.

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
- Browser workbench layout (dockview groups, tab order, split proportions,
  active pane) per work root becomes persisted and restored on reload. This
  revises the current `WorkRoot IO Restore Model` spec claim that exact
  browser arrangement stays outside the restore model. Restore must validate
  every referenced pane target against currently-available resources and
  degrade (drop the reference, do not error) when a target is gone —
  consistent with the existing model's "browser arrangement remains
  presentation state, daemon state is authoritative" principle.
- Terminal visual state (scrollback buffer, cursor position, styles, scroll
  viewport offset) is captured and restored across reload for terminals the
  frontend reattaches to by id. This layers richer visual restore on top of
  the already-correct id-based reattach path; it does not change how
  reattach-vs-fresh-spawn is decided. New sessions spawned via the
  restore-intent fallback (daemon has no live terminal) still start with an
  empty buffer, as today.
- The same layout/terminal-visual capture-and-restore primitive built for
  reload is reused when a user reopens a work root they previously closed via
  the explicit "close work root" action within the same browser session, so
  an explicit close is not punished with a worse restore experience than a
  full page reload would give.
- Not in scope: the other `260523-research-ws-dashboard-persistable-ui-state-map`
  candidates (file explorer tree state, Activity Console local state, command
  palette/keybinding preferences, dashboard chrome preferences, root picker
  history). Persisted layout/terminal-visual state must not store host paths,
  daemon-private ids, raw unbounded transcripts, or anything treated as more
  authoritative than daemon/resource state, matching that research ticket's
  Risk Boundaries.

## Constraints

- Persisted keys (layout, terminal visual snapshots) must include
  `serverRoute` + `workRootId` + pane/terminal id to avoid collisions across
  linked servers and work roots, consistent with the collision-safety
  constraint already established for server-scoped identity elsewhere in the
  dashboard.
- Serialized terminal buffer snapshots must be size-bounded and writes
  debounced; this is a bounded visual cache tied to one pane's lifecycle, not
  a general raw-output persistence store.
- Restore must never treat persisted browser state as authoritative over live
  daemon/resource state; every restored reference is revalidated on load.

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

### Phase 5: Persist and restore per-work-root dockview layout across reload

Serialize each work root's dockview arrangement (groups, tab order, split
proportions, active panel) on change (debounced) into browser-local storage,
keyed by `serverRoute` + `workRootId`. On mount, before creating any panes for
a work root, read the persisted layout if present and use it to drive
`buildWorkbenchEditorGroups`/pane creation order instead of falling back to
the default single-group arrangement.

Every pane reference in a persisted layout must be revalidated against
currently-available resources on restore: a file pane restores only if the
file remains previewable (matching the existing `WorkRoot IO Restore Model`
file-pane rule), and a terminal pane restores only if it maps to a listed
daemon-alive terminal (Phase 4's `listTerminals` reattach) or a valid restore
intent. Unavailable references are dropped from the restored layout, not
shown as errors.

This phase revises the `WorkRoot IO Restore Model` spec's current claim that
exact browser workbench arrangement stays outside the restore model — spec
update lands in this phase.

Verification should cover layout serialization round-trip tests, restore
behavior when a referenced file/terminal is no longer available, and
collision tests for the same `workRootId` on two different `serverRoute`
values.

### Phase 6: Capture and restore terminal visual buffer state across reload

Capture each terminal pane's visible scrollback buffer, cursor position, text
styles, and scroll viewport offset (e.g. via `@xterm/addon-serialize` or an
equivalent buffer walk), persisted debounced alongside Phase 5's layout state
and bounded in size.

When the frontend reattaches to a still-alive daemon terminal by id (the
already-correct path from `listTerminals`), replace the current plain-text
`pane.output` replay with: write the serialized buffer snapshot into the
freshly created xterm instance, restore the scroll viewport offset, then apply
Phase 4's delta-cursor mechanism to catch up on any output the daemon produced
after the snapshot was taken. New sessions spawned via the restore-intent
fallback (no daemon-alive terminal to reattach to) still start from an empty
buffer, as today — there is nothing to restore.

This phase updates the `WorkRoot IO Restore Model` spec's terminal-tab-restore
anchor to describe the richer visual restore alongside the existing
new-session-on-daemon-restart behavior, which is unchanged.

Verification should cover buffer-serialize/restore round-trip tests, scroll
offset restore, size-bound enforcement, and a check that the delta-cursor
catch-up after a restored snapshot does not duplicate or drop output.

### Phase 7: Reuse layout/terminal-visual restore for in-session close/reopen

Wire Phase 2's explicit "close work root" teardown and its later reopen path
through the same capture-and-restore primitive built in Phases 5-6, instead of
starting a reopened root from a blank workbench. Closing a work root should
snapshot its layout and terminal visual state at close time (or reuse the
last debounced snapshot); reopening within the same browser session restores
from that snapshot the same way a page reload would.

This is why the two lines of work in this ticket share one ticket: the
persistence primitive built to fix reload is the same mechanism that makes the
explicit-close affordance from Phase 2 not feel like a worse regression than
just leaving the root open (which Phase 1 makes safe by default).

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md`, primarily the
`WorkRoot IO Restore Model` anchor (`#260516-ws-web-dashboard-workroot-io-restore-model`)
and its `#260523-ws-dashboard-terminal-tab-restore` sub-anchor.

- Phases 1-4, 7: Contract-first: no. These change internal lifecycle and
  reconnect-protocol behavior without changing the documented restore-model
  contract (work-root switching becoming non-destructive is an
  implementation-quality fix, not a new persisted contract).
- Phase 5: Contract-first: yes. Revises the explicit "exact browser workbench
  arrangement remain outside the restore model" claim; spec update lands in
  this phase.
- Phase 6: Contract-first: yes. Extends the terminal-tab-restore anchor to
  describe visual-buffer restore for id-reattached terminals; spec update
  lands in this phase.
