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

### Result (1ba87971) - 2026-07-06

Replaced the single shared `DockviewWorkbenchLayout` mount in `WorkbenchShell`
with one instance per visited work root. Added `openWorkRootKeys`/
`openWorkRootRefs` state (an append-only, de-duplicated membership list keyed
by `serverScopedIdentity`) to track which roots have been visited, and a pure
`findOpenWorkRoot` resolver (extracted to
`ws-dashboard/frontend/src/workbench/openRootLookup.ts`) that re-resolves each
open root against `resources` every render, independent of the tree-walk
selection state. Factored the existing `buildWorkbenchEditorGroups` +
`applyWorkbenchPaneOrder` call into a `buildEditorGroupsForRoot` helper,
invoked once per open root per render; live Activity state
(`activityPaneOpenByRoot`/`workRootActivityState`/`activityTranscriptRefresh`)
is passed only for the selected root, with inert defaults for background
roots. Render now `.map()`s over resolved open roots, each wrapped in a
`.workbench-root-instance` div toggling `display:none` inline instead of
unmounting; `terminalPanes`, `workbenchGroupsByRoot`, and `paneOrderByRoot`
were reused unchanged, per the ticket's Constraints. The pre-existing CONTRACT
`data-workbench-layout-owner="dockview"` locator was left untouched (latent,
not current risk, since no existing test opens two roots mid-flow); the new
wrapper carries `data-workbench-root-active` as the escape hatch for future
multi-root test scoping.

Commits: `c7a1f59c` (feat: mount one dockview workbench instance per open work
root), `1ba87971` (test: add unit coverage for `findOpenWorkRoot`, closing a
review-found coverage gap).

Review: partitioned correctness/fit/test. Correctness clean (3 minors, all
accepted as-is: a plan-sanctioned `mainInstances[0]` display simplification
for the agent tab on multi-main-instance roots; a one-frame empty-workbench
flash on first opening a not-yet-visited root, smoothness-only with no
terminal state loss; a pre-existing, not-introduced-by-this-diff keep-alive
hole when `selection` itself becomes null, noted as a forward-reference for
later phases). Fit clean (1 minor accepted as-is: a new
`.workbench-root-instance` CSS class for flex sizing, a necessary and
well-justified deviation from the plan's "no styles.css change" framing).
Test initially found 2 Important issues — the claimed
`test:workbench`/`test:terminals` pass results provided zero regression
coverage for this diff's actual changed code (neither suite touches
`App.tsx`), and the pure `findOpenWorkRoot` helper was a realistic, missed
unit-test opportunity — both fixed in `1ba87971` (extracted `findOpenWorkRoot`
into a testable module, added coverage for its no-match and
`serverRoute`-mismatch collision branches, corrected the report's
test-coverage framing) and re-verified clean.

Verification: `npm run build` (tsc -b + vite build) and
`npm run test:workbench`/`npm run test:terminals` all pass. Playwright e2e
(`dashboard-acceptance.spec.ts`) could not run in this sandbox
(`libasound.so.2` missing, no Chromium binary), matching the pre-existing
environment gap already documented in the sibling
`260525-feat-ws-dashboard-server-scoped-operation-forwarding` ticket's Phase
3-7 Results; manual/structural verification confirmed inactive root instances
are hidden via `display:none` rather than unmounted, so their dockview/xterm
DOM nodes are never destroyed on a root switch.

Spec Impact: none, per this ticket's own Spec Impact classification
(Contract-first: no for Phases 1-4/7) — this phase is an internal render
restructuring with no new browser-visible contract; no spec entry added.

### Phase 2: Explicit "close work root" action

Add a close affordance for open work roots in the left panel. Triggering it
runs the current (pre-Phase-1) destructive teardown for that root's panes
only: dockview panel removal, xterm dispose, socket close, and any
`terminalPanes`/`workbenchGroupsByRoot`/`paneOrderByRoot` entries scoped to
that root. Switching among still-open roots must not trigger this path.

### Result (d05f5fc0) - 2026-07-06

Added an `X`-iconed close affordance to left-panel work-root rows (no
confirmation dialog, since this action does not touch daemon state), wired
through a new `workRoot.close` command. Closing a root removes its key from
Phase 1's `openWorkRootKeys` (lifted from `WorkbenchShell` to `App()` to give
the left panel read/close access, mirroring the existing
`workbenchGroupsByRoot`/`paneOrderByRoot` lift pattern), which stops rendering
that root's `DockviewWorkbenchLayout` instance and triggers its existing
unmount-driven dispose/socket-close cleanup — no new dispose/close code was
needed. `workbenchGroupsByRoot`/`paneOrderByRoot` are cleared in the
App-level command handler; `terminalPanes` (via a new pure
`removeTerminalPanesForWorkRoot` filter in `terminals.ts`) and
`activityPaneOpenByRoot`/`activePaneByRoot`/`closedAgentPaneByRoot` are
cleared in a `WorkbenchShell`-local cleanup effect. The daemon `closeTerminal()`
API is never called, so the daemon terminal stays alive for a later reattach
(matching the ticket's `WorkRoot IO Restore Model` framing). A same-commit
deletion-ordering hazard (the close handler deletes a key from both
`openWorkRootKeys` and `openWorkRootRefs` in one React commit, which would
otherwise lose the `{rootId, serverRoute}` needed to filter `terminalPanes`)
was resolved by snapshotting both into refs that only advance from inside the
cleanup effect, and the pure key-diff computation at the center of that fix
was extracted into `resolveClosedWorkRootRefs` (`workbench/openRootLookup.ts`,
alongside Phase 1's `findOpenWorkRoot`) rather than left inline.

Commits: `7591d9f5` (feat: add `workRoot.close` command and pane-filter
helper), `8af5405c` (feat: wire left-panel close action for open work roots),
`d05f5fc0` (fix: fix compact-root close no-op, guard selected-root close,
extract key-diff helper).

Review: partitioned correctness/fit/test. Fit clean (1 minor accepted as-is:
inline `serverScopedIdentity` duplication across the compact/non-compact row
branches, purely stylistic). Correctness initially found 1 Critical and 1
Important issue: the compact work-root row's close button was a no-op
because it dispatched the close command with `workspace.id` instead of the
work-root id (`compactRoot.id`), so the membership-key filter never matched;
and closing the currently-selected root did not stick, because nothing
prevented closing the selected root and the App-level population effect
re-added it on the next resources poll. Both fixed in `d05f5fc0` (route the
concrete work-root id into `buildWorkRootCloseCommand` at both row call
sites, keeping `actionEntityId` as `workspace.id` for `gitWorktreeAdd`/
`workspace.remove` on the same compact row; disable the close affordance for
the currently-selected root rather than attempting to move selection) and
re-verified clean, with 2 minors remaining (a pre-existing `closedAgentPaneByRoot`
keying-by-plain-rootId scheme, accepted as unrelated to this diff; two now-dead
`onOpenWorkRootKeysChange`/`onOpenWorkRootRefsChange` props left on
`WorkbenchShell` after the population effect moved to `App()`, cleanliness-only).
Test initially found 1 Important issue — the cleanup effect's pure key-diff
computation (the crux of the same-commit deletion-ordering fix) had zero unit
coverage despite being extractable without a React harness — fixed in
`d05f5fc0` (extracted `resolveClosedWorkRootRefs`, added coverage for a
dropped-key resolution, a still-present key being excluded, and a
missing-`previousRefs`-entry skip) and re-verified clean.

Verification: `npm run build` and `npm run test:terminals`/`test:commands`/
`test:workbench` all pass. Playwright e2e remains not runnable in this
sandbox (`libasound.so.2` missing, no Chromium binary), the same
pre-existing gap as Phase 1.

Spec Impact: none, per this ticket's own Spec Impact classification
(Contract-first: no for Phase 2) — internal lifecycle change only, no new
browser-visible contract.

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

### Result (7c7c913c) - 2026-07-06

Landed under the explicit-acceptance branch: Phase 4 has not landed yet, so
the duplicate-output-on-resume risk from the still-open `nextSequence`
tracking gap is knowingly accepted for now. A `paneVisible` state was added to
`TerminalPaneBody`, driven by extending the existing 100ms `focusWatchdog`
interval to unconditionally compute `container.offsetParent` on every tick
(reusing the established visibility idiom already used by the focus-restore
logic, rather than adding a second timer). The terminal socket-open effect now
depends on `paneVisible` and no-ops (setting `socketStatus="disconnected"`)
while hidden, letting React's own cleanup close the live socket; becoming
visible again re-runs the effect body unchanged, reopening via the existing
`terminalWebSocketCursor` resume mechanism — no new resume logic was needed.
A survey finding simplified the scope: Dockview already hides an
inactive-in-group tab via `display:none` on its content wrapper, and Phase 1
hides an inactive root's `.workbench-root-instance` the same way, so the
ticket's two visibility conditions ("root not active" and "group not
frontmost") both collapse to the same `offsetParent` check — no prop plumbing
through `buildWorkbenchEditorGroups`/`terminalWorkbenchPanesByGroup` was
needed. The xterm instance and `TerminalPaneState` (aside from
`socketStatus`) are untouched across the gate; no daemon-side change was made,
since the daemon's existing `after`-cursor backfill already tolerates a
closed/reopened client socket like any other reconnect.

Commits: `b0834727` (feat: gate terminal WebSocket lifecycle on pane
visibility), `7c7c913c` (fix: stop visibility-gated terminal sockets from
triggering HTTP poll fallback).

Review: partitioned correctness/fit/test. Fit and test both clean with zero
findings. Correctness found 1 Important issue: gating a socket closed sets
`socketStatus="disconnected"`, which flips `shouldPollTerminalOutput()` to
true — but the HTTP output-poll fleet is filtered to the active work root
only, so a background-root pane goes genuinely quiet (socket closed, not
polled, matching the phase's resource-reduction goal) while an active-root,
non-frontmost tab stayed in the poll fleet and began a continuous 120ms HTTP
poll while hidden — a net increase in traffic for exactly the in-group
tab-switch case the phase's own verification plan claimed to handle. Fixed in
`7c7c913c` by adding a `visibilityGated` field to `TerminalPaneState`,
distinct from `socketStatus`, so `shouldPollTerminalOutput` can tell
"intentionally gated closed" apart from "genuinely disconnected" — the gate
flag is set only in the hidden branch (where no live socket exists, so it can
never coincide with a real disconnect) and cleared as the first statement of
the visible branch, before the new socket is even opened, so a subsequent
real error/exit still polls correctly. Re-verified clean. One correctness
minor was accepted as-is (reusing `"disconnected"` status also makes
`dockviewLayoutModel.ts` treat the pane's meta as churn-allowed while hidden,
no visible effect).

Verification: `npm run build` and `npm run test:terminals`/`test:workbench`
all pass, including new unit coverage for the `visibilityGated` gate/poll
interaction. Playwright e2e remains not runnable in this sandbox
(`libasound.so.2` missing, no Chromium binary), the same pre-existing gap as
Phases 1-2; this phase's actual close-on-hide/reopen-on-show timing was
verified structurally via code reading instead (effect cleanup/dependency
ordering, functional setState guards against redundant transitions).

Spec Impact: none, per this ticket's own Spec Impact classification
(Contract-first: no for Phase 3) — internal lifecycle change only, no new
browser-visible contract.

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

### Result (6c0d1b3d) - 2026-07-06

Frontend: added `markTerminalOutputCursor` (`terminals.ts`), reusing the
cursor-advance formula already proven correct in
`appendTerminalWebSocketMessage`'s `status`/`exit` branch
(`nextSequence: Math.max(pane.nextSequence, chunkSequence + 1)`), and wired it
into `applyTerminalSocketMessage`'s previously-no-op `output` branch — the
direct xterm-write/`writtenLengthRef` path stays untouched, so this only adds
the missing cursor field, not a second copy of output text. Backend: added
`truncated: bool` to both the `Status` and `Exit` variants of
`TerminalWebSocketServerMessage`, and a new `TerminalSession::is_range_truncated(after)`
predicate wired into `send_output_backfill` using the `after` cursor value
captured *before* the backfill loop advances it (an ordering the plan
explicitly flagged as the mechanism's correctness-critical detail). The
truncation check only fires when `after > 0` **and** the oldest retained
chunk's sequence exceeds `after + 1`, deliberately excluding a fresh
`after == 0` first-time attach (which always means "send me everything you
have") from ever being misreported as a gap, even against a terminal that has
already produced more than `MAX_OUTPUT_CHUNKS` output. Frontend renders a
truncated frame as a visible gap-marker string appended to `pane.output`,
reusing the existing xterm-write diffing effect rather than a new render
path. No buffer retention-policy change and no touch to the HTTP
`terminal_output`/`TerminalOutputView` route, per the ticket's explicit
non-goal.

Commits: `182d4e28` (fix: advance terminal cursor per output frame and signal
ring-buffer truncation on resume), `6c0d1b3d` (test: cover cursor-capture
ordering and off-by-one boundary).

Review: partitioned correctness/fit/test. Correctness clean with zero
findings. Fit clean (1 minor accepted as-is: the new backend unit tests
landed in an existing `#[cfg(test)]` module whose name — inherited from
unrelated shell/PTY-portability tests — no longer describes its contents,
an unavoidable consequence of `TerminalSession` being file-private with no
external constructor). Test initially found 2 Important issues: (1) the
`send_output_backfill` wiring itself — the capture-before-loop ordering that
is the actual correctness-critical detail — had zero test coverage, since
the two original unit tests called `is_range_truncated` directly rather than
through the real call site; (2) the frontend cursor-advance test suite was
missing the exact off-by-one boundary (`chunkSequence == nextSequence - 1`).
Both fixed in `6c0d1b3d`: extracted `send_output_backfill`'s body into a pure,
directly-unit-testable `plan_output_backfill(session, cursor)` function
(`TerminalSession`'s file-private constructors made an external
`tests/routes.rs` integration test structurally impossible, and driving a
real PTY past 1024 discrete chunks is not deterministically controllable due
to OS-level write coalescing — both independently ruled out the plan's
originally suggested integration-test approach), with a new test proving the
capture-before-loop ordering by construction; and added the missing boundary
assertion for `markTerminalOutputCursor`. Re-verified clean.

Verification: `cargo test -p ws-dashboard-daemon` (39 lib tests including the
two new truncation tests, 144 route tests, 15 server tests) and
`npm run test:terminals` all pass. Playwright e2e remains not runnable in
this sandbox (`libasound.so.2` missing, no Chromium binary), the same
pre-existing gap as Phases 1-3.

Spec Impact: none, per this ticket's own Spec Impact classification
(Contract-first: no for Phase 4) — an internal wire-protocol field addition
with no documented contract to revise; checked the spec's terminal WebSocket
anchors, which describe behavior at a high level and do not enumerate
per-message field shapes.

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

### Result (cfcd5a3d) - 2026-07-06

Added a new `workbench/layoutRestore.ts` module (modeled on
`workRootFiles.ts`'s existing read-only-file-pane restore snapshot shape,
which this phase reuses rather than dockview's native `toJSON()`/`fromJSON()`
— unusable here since this app's panel params carry live, non-serializable
`ReactNode`s/closures and the app already owns a competing declarative
reconciliation loop). Persists each open work root's dockview groups, tab
order, active pane per group, and (landed in full, not deferred)
best-effort split proportions to a versioned `localStorage` key, seeded once
per rootKey the first time it's visited each session (never clobbering an
in-session live layout) and revalidated/pruned against currently-live
terminal/file pane ids on every relevant change, reusing
`reconcileActiveWorkbenchPanes` for active-pane repair. Updated the
`WorkRoot IO Restore Model` spec anchor to narrow "exact browser workbench
arrangement remain outside the restore model" to: arrangement is persisted
keyed by `serverRoute`+`workRootId`, restored on reload, never authoritative
over live daemon/resource state, and unavailable references are silently
dropped — Auth/terminal-process/Activity-acknowledgement exclusions
unchanged. Flagged `workbench/layoutSerialization.ts` as pre-existing,
unrelated dead code (a recursive tree-shaped model incompatible with this
app's actual flat `groups[]`/`paneOrderByGroup` shape) rather than retrofitting
it.

Commits: `cd8c93ed` (feat: add the layout restore module + tests),
`6591cc04` (feat: wire seed/save/revalidate into App.tsx), `a39d7da0` (feat:
best-effort split-size restore + spec anchor update), `de56d11c` (fix: merge
unvisited-root layout saves, gate terminal prune on load), `cfcd5a3d` (fix:
clear `terminalsReadyRootKeys` on work-root close).

Review: partitioned correctness/fit/test. Fit clean throughout with zero
findings. Test initially found 2 Important issues — the module's own
drop-stale-group-id defensive parsing branches were unexercised, and the
highest-restore-risk App.tsx wiring (the prune+reconcile revalidation
transformation) repeated a gap pattern this ticket's earlier phases already
flagged and fixed (extractable pure glue logic left untested) — both fixed
by extracting `revalidateWorkbenchLayoutForRoot` into `layoutRestore.ts` with
new unit tests, and adding the missing boundary tests; re-verified clean.
Correctness found 2 Critical issues on the first pass: (1) the save effect
built its persisted entries only from roots visited *this* session and did a
full overwrite of the storage key, silently wiping any root not re-visited
before the next reload (including wiping the entire snapshot on the very
first render); (2) the revalidation/prune effect ran synchronously the
moment a restored layout was seeded, before the async `listTerminals` call
for that root had resolved, permanently stripping every restored terminal
reference before terminals ever had a chance to load. Both fixed in
`de56d11c` — the save now writes the union of live entries for open roots
and untouched entries (from the frozen mount-time snapshot) for every root
not open this session, and a new `terminalsReadyRootKeys` per-root grace
flag (set once `listTerminals` resolves, including on error via `.finally`)
gates the terminal-pane portion of the prune. A second re-review found one
residual Important gap in that fix: the grace flag was append-only and never
cleared on work-root close, so reopening a previously-visited root within
the same session immediately re-pruned its restored terminal order using the
stale "ready" flag from its earlier visit — fixed in `cfcd5a3d` by clearing
the flag in the same close-cleanup effect that already tears down the
root's other per-root state. Final re-review: correctness clean (1 minor
accepted as-is: `pruneWorkbenchLayoutOrder`'s return type loses a nominal
type via an `Object.fromEntries` cast, cosmetic only), fit clean, test
clean.

Verification: `npm run test:workbench`, `npm run test:terminals`, and
`npm run build` all pass. Playwright e2e remains not runnable in this
sandbox (`libasound.so.2` missing, no Chromium binary), the same
pre-existing gap as Phases 1-4; this phase's actual dockview-visible restore
behavior was verified structurally via code reading and unit tests of the
extracted pure logic instead.

Spec Impact: `WorkRoot IO Restore Model` anchor updated per this ticket's
Contract-first: yes classification for Phase 5 (see above).

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

### Result (07d14b4d) - 2026-07-06

Implemented on `implement/phase-6-terminal-visual-buffer-restore`
(`97d1f9b0..07d14b4d`), merged into `ws-dashboard-dev`.

- Added `@xterm/addon-serialize@^0.14.0` as a new frontend dependency
  (compatible with existing `@xterm/xterm@^5.5.0`/`@xterm/addon-fit@^0.10.0`).
- New module `workbench/terminalVisualRestore.ts`: `TerminalVisualRestoreEntry`
  (`logicalKey`, `serialized`, `nextSequence`, `viewportY`, `capturedAtMs`),
  `loadTerminalVisualRestoreSnapshot`/`saveTerminalVisualRestoreSnapshot`
  against `localStorage` key `ws-dashboard.terminalVisual.v1`, a per-entry size
  bound (`terminalVisualRestoreMaxSerializedLength`, ~200,000 chars) that drops
  (never truncates) oversized entries so no partial ANSI escape sequence can
  reach `terminal.write`, and the pure decision function
  `resolveTerminalMountWrite(pane, restoreEntry)` returning
  `{kind:"restore", serialized, viewportY} | {kind:"replay", text} | {kind:"none"}`.
- `TerminalPaneBody`'s mount effect now loads `SerializeAddon` alongside the
  existing `FitAddon`, calls `resolveTerminalMountWrite` to pick the branch,
  and for the `restore` kind writes the serialized snapshot with
  `terminal.write(serialized, () => terminal.scrollToLine(viewportY))` — the
  `scrollToLine` call runs inside xterm's write-completion callback so it acts
  on the fully parsed buffer instead of racing xterm's asynchronous write.
  `writtenLengthRef` is left at 0 for the `restore` kind (decoupled from the
  snapshot write) so Phase 4's delta-write effect only writes output produced
  after the snapshot, appended after the restored buffer with no duplication
  or drop.
- Reattach-only threading: `terminalPaneFromSession`, `mergeListedTerminalSessions`,
  and `reconcileListedTerminalSessions` gained an optional
  `visualRestoreByLogicalKey` lookup that seeds `pane.nextSequence` from the
  matching snapshot's captured cursor, feeding Phase 4's
  `terminalWebSocketCursor = Math.max(0, nextSequence - 1)` catch-up math. The
  two non-reattach call sites (`createTerminalPane`, `placeTerminalSessions`)
  intentionally do not receive the lookup and keep starting empty.
- A debounced (~900ms) capture effect persists a `TerminalVisualRestoreEntry`
  per pane on `pane.output` changes via `SerializeAddon.serialize({scrollback: 2000})`,
  reading latest state through a ref at fire time; the timer is cleared on
  every dependency change and on unmount, with a guard against firing after
  the addon/terminal refs are nulled during teardown.
- Spec anchor `#260523-ws-dashboard-terminal-tab-restore` updated to describe
  the richer visual-buffer restore for id-reattached terminals, alongside the
  unchanged new-session-on-daemon-restart behavior.

Review and fix cycle: Fit review was clean throughout. Correctness review
(opus-tier) came back "clean with 2 minor" — a scroll-offset restore
reliability bug (the original code called `scrollToLine` immediately after
`terminal.write` rather than inside its completion callback, so it raced
xterm's async write-buffer parse) and an out-of-scope stale-entry-accumulation
follow-up (accepted, no fix — degrades gracefully, per-entry size bound was
the documented guardrail). Test review found one Important finding (the
mount-effect's three-way write-branch decision was inlined and untested,
repeating the same extractable-pure-glue gap shape already caught and fixed
in Phases 1/2/4/5) and one Minor (missing exact-boundary size tests). A single
combined fix-relay cycle (`07d14b4d`) fixed the scroll-offset race, extracted
`resolveTerminalMountWrite` with unit tests for all three branches, and added
the boundary-size tests; re-review confirmed all three partitions clean with
no new issues introduced by the extraction.

Verification: `npm run test:terminals`, `npm run test:workbench`, and
`npm run build` all pass in `ws-dashboard/frontend`. Playwright e2e
(`dashboard-acceptance.spec.ts`) remains not runnable in this sandbox
(`libasound.so.2` missing, no Chromium binary) — consistent with the
disclosure in every prior phase's Result; the mount-effect's DOM/xterm-coupled
behavior (async write-completion ordering, debounce-timer lifecycle) was
verified via direct code reading plus the extracted pure function's unit
tests, not an end-to-end run.

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

### Result (bb2bbf0a) - 2026-07-06

Implemented on `implement/phase-7-close-reopen-restore-reuse`
(`8b18b36b..bb2bbf0a`), merged into `ws-dashboard-dev`.

- Survey found that this phase's actual gap was not "no capture-and-restore
  wiring exists" but that Phases 5-6's snapshot state
  (`initialWorkbenchLayoutRestore`/`initialTerminalVisualRestore`) was frozen
  at `App` mount via `useState` with no setter, so every same-session
  reopen-seed and reattach-lookup site read the page-load snapshot instead of
  the continuously-updated live one. The survey also found a real regression,
  not just staleness: the layout save effect re-fired immediately after every
  close and clobbered `localStorage` for the just-closed root with the stale
  mount-time entry — so even a plain page *reload* right after an in-session
  close/edit sequence lost that edit, independent of any new close/reopen
  affordance.
- Converted both snapshots to `useRef`s (`workbenchLayoutRestoreRef`,
  `terminalVisualRestoreRef`) kept live by the existing save/capture effects,
  and repointed every read/write site (selection-seed effect,
  `initialGroupSizeById`, `onVisualRestoreEntryFor`/`onVisualCapture`, the
  `listTerminals` reconcile call) to read `.current`. A grep-for-stale-identifier
  completeness check caught one site missed by the initial survey (the
  `activePaneByRoot` seed effect), fixed in the same pass.
- Fixed the save-effect clobber bug by writing the merged live+untouched
  result back into the ref on every run, keyed via the existing
  `workbenchLayoutRestoreRootKey` export, so the ref and `localStorage` never
  diverge.
- Review found this merge/clobber-fix computation was itself left as
  untested inline glue — the same extractable-pure-logic gap this ticket's
  Test partition has now caught and fixed in six of its seven phases
  (1/2/4/5/6/7) — plus a smaller duplication of `upsertTerminalVisualRestoreEntry`'s
  upsert-by-key semantics in the new ref-mirror write. A single fix-relay
  cycle (`bb2bbf0a`) extracted `mergeWorkbenchLayoutRestoreEntries` into
  `workbench/layoutRestore.ts` and `upsertTerminalVisualRestoreEntryInSnapshot`
  into `workbench/terminalVisualRestore.ts`, unit-tested both, and wired
  App.tsx to delegate to them with no behavior change; re-review confirmed
  clean with no new issues. Correctness and Fit partitions were clean on the
  first pass.
- No spec edit: classified `Contract-first: no` per this ticket's own Spec
  Impact section — internal lifecycle/session-persistence plumbing, not a new
  documented contract.

Verification: `npm run build`, `npm run test:workbench`, and
`npm run test:terminals` all pass in `ws-dashboard/frontend`. Playwright e2e
remains not runnable in this sandbox (`libasound.so.2` missing, no Chromium
binary), consistent with every prior phase's disclosure; the close→reopen
integration itself (no App-level render harness exists in this codebase) was
verified by structural code trace plus the newly extracted pure functions'
unit tests, not an end-to-end run.

#### Ticket-wide completeness check

All 7 phases of this ticket now carry `### Result` sections and are merged
into `ws-dashboard-dev`:

1. Per-work-root workbench instances (no destroy on root switch) — `1ba87971`
2. Explicit "close work root" action — `d05f5fc0`
3. Visibility-gated terminal WebSocket lifecycle — `7c7c913c`
4. Cursor accuracy and gap signaling for reconnect — `6c0d1b3d`
5. Persist and restore per-work-root dockview layout across reload — `cfcd5a3d`
6. Capture and restore terminal visual buffer state across reload — `07d14b4d`
7. Reuse layout/terminal-visual restore for in-session close/reopen — `bb2bbf0a`

The two lines of work this ticket set out to unify — reload-survival (Phases
3-6) and in-session close/reopen (Phases 1-2, wired to the same primitive in
Phase 7) — now share one restore mechanism end-to-end: a closed root's
layout and terminal visual state stay live in the same ref-backed snapshot a
page reload reads from, so reopening within a session and reloading the page
restore identically. Every phase's Test-partition review independently
converged on the same remediation shape (extract pure glue logic out of
App.tsx effects, unit-test it), which is now a consistent, self-reinforcing
pattern across `workbench/openRootLookup.ts`, `workbench/layoutRestore.ts`,
and `workbench/terminalVisualRestore.ts`. The one durable, explicitly-accepted
gap across the whole ticket is Playwright e2e coverage for the actual
browser-integration behavior (page reload, socket reconnect, visual restore,
close/reopen) — not runnable in this sandbox across any phase — mitigated by
extracting and unit-testing every non-trivial pure computation reachable
without a browser, plus direct code-reading verification for the remaining
DOM/xterm/React-effect-coupled wiring. No further phases or follow-up work
are outstanding for this ticket.

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
