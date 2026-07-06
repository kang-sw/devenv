# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 6: Capture and restore terminal visual buffer state across reload

## Relevant Ticket Contract

- Capture each terminal pane's visible scrollback buffer, cursor position,
  text styles, and scroll viewport offset (e.g. via `@xterm/addon-serialize`
  or an equivalent buffer walk), persisted debounced alongside Phase 5's
  layout state and bounded in size.
- When the frontend reattaches to a still-alive daemon terminal by id, replace
  the current plain-text `pane.output` replay with: write the serialized
  buffer snapshot into the freshly created xterm instance, restore the scroll
  viewport offset, then apply Phase 4's delta-cursor mechanism to catch up on
  any output the daemon produced after the snapshot was taken.
- New sessions spawned via the restore-intent fallback (no daemon-alive
  terminal to reattach to) still start from an empty buffer — nothing to
  restore.
- Constraints (ticket-level, apply to this phase): persisted keys must include
  `serverRoute` + `workRootId` + terminal id to avoid collisions; snapshots
  must be size-bounded and writes debounced (a bounded visual cache tied to
  one pane's lifecycle, not a general raw-output store); restore must never
  be treated as authoritative over live daemon/resource state.
- Spec Impact: Contract-first: yes. Update the `#260523-ws-dashboard-terminal-tab-restore`
  sub-anchor (`ai-docs/spec/ws-web-dashboard/index.md:1556-1559`) to describe
  visual-buffer restore for id-reattached terminals, alongside the unchanged
  new-session-on-daemon-restart behavior.
- Verification should cover buffer-serialize/restore round-trip, scroll
  offset restore, size-bound enforcement, and a check that delta-cursor
  catch-up after a restored snapshot does not duplicate or drop output.

## Out of Scope

- Phase 7 (reusing this same primitive for in-session close/reopen via Phase
  2's close action) — this phase only covers full page reload.
- Any buffer retention-policy change on the daemon side (chunk-count vs.
  byte-size vs. time-based ring buffer) — explicitly out of scope per Phase 4.
- New-session-via-restore-intent path — unchanged, still starts empty.
- Any change to `crates/daemon/src/terminal.rs` — this phase's mechanism is
  entirely frontend (browser-local persistence + xterm buffer replay); the
  daemon-side delta/truncation primitives it reuses already exist from Phase 4.

## Codebase Findings

- `ws-dashboard/frontend/package.json:24-56` — confirmed `@xterm/xterm@^5.5.0`
  and `@xterm/addon-fit@^0.10.0` are present; **no `@xterm/addon-serialize` or
  any other `@xterm/*` addon exists yet**. `npm view @xterm/addon-serialize
  version` succeeded in this sandbox (resolved `0.14.0`, compatible with
  xterm.js 5.x, same major-version-alignment pattern as the existing
  `addon-fit@0.10.0`/`xterm@5.5.0` pairing) — network access to the npm
  registry is available from this sandbox, so `npm install` is expected to
  succeed, but the executor must still run it and treat a failure as a hard
  blocker requiring escalation, not a silent workaround.
- `ws-dashboard/frontend/src/App.tsx:6465-6494` (`TerminalPaneBody` mount
  effect, empty deps `[]`, runs exactly once per component mount) —
  currently always does `terminal.write(liveRef.current.pane.output)` as the
  initial content and sets `writtenLengthRef.current = initialOutput.length`.
  This is the exact "plain-text replay" the phase text calls out and the
  precise site to branch: if a persisted visual snapshot matches this pane,
  write the snapshot instead and do **not** touch `writtenLengthRef` (it must
  stay scoped to `pane.output` tracking, independent of the snapshot's own
  escape-sequence text, to avoid corrupting the existing delta effect below).
- `ws-dashboard/frontend/src/App.tsx:6827-6840` — the existing delta-write
  effect diffs `pane.output.length` against `writtenLengthRef.current` and
  writes only the new suffix into the terminal. Because `pane.output` always
  starts at `""` for a freshly reattached pane (see next finding) and
  `writtenLengthRef` starts at `0` in both the snapshot and non-snapshot
  branches, this effect needs **no changes** — new output arriving after a
  snapshot-based initial write is appended correctly, with no duplication,
  as long as the snapshot write never touches `writtenLengthRef`.
- `ws-dashboard/frontend/src/terminals.ts:311-329` (`terminalPaneFromSession`)
  — always sets `output: ""` and `nextSequence: 0` for every freshly built
  pane, regardless of whether the session is a genuine reattach or a brand
  new spawn. This is the seam for wiring in `nextSequence` seeding: pass an
  optional lookup (by `logicalKey`, see next finding) so a reattach with a
  matching persisted visual snapshot seeds `nextSequence` from the snapshot's
  captured sequence instead of `0`.
- `ws-dashboard/frontend/src/terminals.ts:170-172` (`terminalWebSocketCursor`)
  — `Math.max(0, pane.nextSequence - 1)`. Seeding `nextSequence` from a
  snapshot's captured sequence (see above) means the very first WebSocket
  connect after a reload requests `after = capturedSequence`, not `after = 0`
  — this is the mechanism that makes Phase 4's delta-cursor catch-up "just
  work" against a restored snapshot with zero new code in the socket-open
  path: chunks already baked into the snapshot are never re-requested, and if
  the daemon's ring buffer already evicted everything back to that cursor,
  Phase 4's existing `truncated: true` signal fires exactly as it does for
  any other stale-cursor reconnect (`crates/daemon/src/terminal.rs`,
  `TerminalSession::is_range_truncated`).
- `ws-dashboard/frontend/src/terminals.ts:52-68` (`TerminalPaneState`) —
  already carries a `logicalKey` field, built by
  `terminalPaneLogicalKey(workRootId, terminalId, serverRoute)`
  (`terminals.ts:293-302`, format `persistentTerminal/<serverScopedIdentity>/<terminalId>`).
  This is the exact collision-safe key the ticket's Constraints section
  requires (`serverRoute` + `workRootId` + terminal id) — reuse it verbatim
  as the persisted-snapshot key; do not invent a new key shape.
- `ws-dashboard/frontend/src/App.tsx:3946-3982` — the **only** call site that
  reattaches to daemon-alive terminals on load: the `listTerminals(...)`
  effect calls `reconcileListedTerminalSessions` → `mergeListedTerminalSessions`
  (`terminals.ts:462-507`) → `terminalPaneFromSession` (`terminals.ts:311`).
  Threading the optional visual-restore lookup through these three functions
  (all currently pure, all already exported and tested per `test:terminals`)
  is the single integration point needed; the other two `terminalPaneFromSession`
  call sites (`App.tsx:4592` inside `createTerminalPane`, and `App.tsx:6280`,
  both genuine new-terminal-creation paths, not reattach) must **not** receive
  the lookup, so they keep starting from an empty buffer as the phase text
  requires.
- `ws-dashboard/frontend/src/workbench/layoutRestore.ts:1-80` and its App.tsx
  wiring (`App.tsx:3616-3649`, `App.tsx` `initialWorkbenchLayoutRestore`
  state) — the established sibling pattern to mirror: a versioned
  `localStorage` blob (`ws-dashboard.workbenchLayout.v1`), a pure
  load/save/parse module independent of React, loaded once at App mount into
  a snapshot passed down as a prop/lookup. Phase 5's own save effect is a
  **plain effect with no literal debounce timer** (explicitly noted in its
  own comment, `App.tsx:3600-3603`, because layout changes are infrequent
  discrete user actions) — this precedent does **not** transfer directly to
  Phase 6: terminal output changes fire far more often (every WebSocket
  output frame), and the ticket's Constraints section explicitly requires
  "writes debounced" for terminal snapshots specifically, so this phase
  should add a real timer-based debounce (e.g. reset a `setTimeout` on every
  qualifying change, fire after ~800ms–1s of quiet), unlike Phase 5.
- `ws-dashboard/frontend/src/App.tsx:6738-6823` (visibility-gated socket
  effect, Phase 3/4) — confirms sockets close on pane-hide and reopen on
  pane-show using the same `terminalWebSocketCursor(pane)` cursor mechanism
  this phase's `nextSequence` seeding feeds into; no changes needed here, but
  worth noting: the debounced capture timer set up in the mount effect must
  be cleared in that effect's own cleanup (mount effect cleanup,
  `App.tsx:6714-6736`) so a disposed terminal never has a pending serialize
  callback fire against `terminalRef.current` after dispose.
- No existing `@xterm/addon-serialize` import or `SerializeAddon` reference
  anywhere in the frontend tree (confirmed via grep); this is a net-new
  dependency and net-new addon load alongside the existing `FitAddon` load
  at `App.tsx:6482-6483`.
- Spec anchor to update: `ai-docs/spec/ws-web-dashboard/index.md:1556-1559`
  (`#260523-ws-dashboard-terminal-tab-restore`) — currently only describes
  daemon-restart new-session behavior; needs an added paragraph describing
  visual-buffer capture/restore for id-reattached terminals, matching Phase
  5's precedent of editing the same `WorkRoot IO Restore Model` anchor
  in-place (`index.md:1544-1554`).

## Implementation Plan

1. Add `@xterm/addon-serialize` (`^0.14.0`) to
   `ws-dashboard/frontend/package.json` dependencies, alongside the existing
   `@xterm/*` entries; run `npm install` in `ws-dashboard/frontend` and
   confirm the lockfile updates cleanly. If install fails (no registry access
   in the actual execution sandbox), stop and escalate rather than vendoring
   or stubbing the dependency.
2. Add a new pure module `ws-dashboard/frontend/src/workbench/terminalVisualRestore.ts`,
   modeled on `workbench/layoutRestore.ts`'s shape/defensiveness:
   - `TerminalVisualRestoreEntry = { logicalKey: string; serialized: string; nextSequence: number; viewportY: number; capturedAtMs: number }`.
   - `loadTerminalVisualRestoreSnapshot()` / `saveTerminalVisualRestoreSnapshot(entries)`
     against a new versioned `localStorage` key (e.g.
     `ws-dashboard.terminalVisual.v1`), reusing `browserStorage()` from
     `workRootFiles.ts` like `layoutRestore.ts` does.
   - Enforce the size bound at this layer: cap `serialized.length` at a fixed
     constant (e.g. 200_000 chars) and drop/skip persisting any entry that
     exceeds it, plus pass a bounded `scrollback` option (see step 4) so the
     addon itself does not serialize unlimited history.
3. In `terminals.ts`:
   - Add an optional `visualRestoreByLogicalKey?: Record<string, Pick<TerminalVisualRestoreEntry, "nextSequence">>`
     parameter to `terminalPaneFromSession`, seeding `nextSequence` from a
     matching entry (by `logicalKey`, computed the same way the function
     already computes it) instead of `0` when present.
   - Thread the same optional parameter through `mergeListedTerminalSessions`
     and `reconcileListedTerminalSessions` (both currently pure, both already
     exported), passing it straight to `terminalPaneFromSession`.
   - Do not change the two non-reattach call sites (`createTerminalPane`,
     `App.tsx:6280`) — they must not pass this parameter.
4. In `App.tsx`:
   - Load the visual-restore snapshot once at App mount into a new
     `initialTerminalVisualRestore` state (mirroring
     `initialWorkbenchLayoutRestore`'s load-once pattern), and pass the
     relevant slice into the `listTerminals` reattach call site
     (`App.tsx:3973`) as the new `reconcileListedTerminalSessions` argument.
   - Pass a per-pane lookup (e.g. via `actions`, matching how other
     pane-scoped callbacks are threaded into `TerminalPaneBody`) so the
     component can read `initialTerminalVisualRestore[pane.logicalKey]` at
     mount time.
   - In `TerminalPaneBody`'s mount effect (`App.tsx:6465-6494`): after
     `terminal.open(container)`, load `new SerializeAddon()` via
     `terminal.loadAddon(...)` (alongside the existing `FitAddon`). If a
     matching visual-restore entry exists for `pane.logicalKey`: `terminal.write(entry.serialized)`
     then restore scroll via `terminal.scrollToLine(entry.viewportY)`, and
     leave `writtenLengthRef.current` at `0` (do not set it from the
     snapshot). Otherwise, keep the existing `pane.output` replay path
     unchanged.
   - Add a debounced capture mechanism: track a timer ref, reset it (~800ms–1s)
     on every `pane.output` change (the same trigger as the existing delta
     effect at `App.tsx:6827-6840`, or a shared effect), and on fire call
     `serializeAddon.serialize({ scrollback: <bound> })` plus read
     `terminal.buffer.active.viewportY`, then persist a
     `TerminalVisualRestoreEntry` keyed by `pane.logicalKey` with
     `nextSequence` read from `liveRef.current.pane.nextSequence` at capture
     time (this is what makes the Phase-4 delta-cursor catch-up line up
     exactly on the next reload). Clear the timer in the mount effect's
     cleanup (`App.tsx:6714-6736`) so no callback fires after `terminal.dispose()`.
5. Update `ai-docs/spec/ws-web-dashboard/index.md:1556-1559`
   (`#260523-ws-dashboard-terminal-tab-restore`) to add a paragraph
   describing: id-reattached terminals restore their serialized visual
   buffer (scrollback, styles, cursor) and scroll viewport offset from a
   bounded, debounced browser-local snapshot keyed by `serverRoute` +
   `workRootId` + terminal id, with the existing delta-cursor mechanism
   catching up any output produced after the snapshot, and a truncation gap
   marker surfacing if the daemon's retained output no longer covers the gap
   — new-session-via-restore-intent behavior is unchanged (still starts
   empty).

## Verification Plan

- `npm install` in `ws-dashboard/frontend` succeeds and resolves
  `@xterm/addon-serialize`.
- `npm run build` (tsc -b + vite build) passes.
- `npm run test:terminals` passes, extended with new unit coverage for: (a)
  `terminalPaneFromSession`/`mergeListedTerminalSessions` seeding
  `nextSequence` from a matching visual-restore entry vs. leaving it `0` when
  no entry matches or the session is a fresh spawn; (b) the new
  `terminalVisualRestore.ts` module's load/save/size-bound-drop behavior
  (round-trip a small entry, and confirm an oversized `serialized` string is
  dropped/skipped rather than silently truncated mid-escape-sequence).
- Manual/structural check (Playwright e2e remains not runnable in this
  sandbox per Phases 1-5's documented `libasound.so.2`/no-Chromium gap):
  trace through the mount-effect branch and confirm `writtenLengthRef` stays
  decoupled from the snapshot write, and that the debounce timer is cleared
  on dispose (no post-dispose `terminalRef.current` access).

## Escalations

- None.
