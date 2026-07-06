# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 3: Visibility-gated terminal WebSocket lifecycle

## Relevant Ticket Contract

- "Extend the terminal socket effect (`frontend/src/App.tsx:6235-6306`) so the
  WebSocket closes when its pane becomes invisible — its work root is not the
  active one, or (within the active root) its dockview group is not frontmost —
  and reopens using the existing cursor-resume mechanism when the pane becomes
  visible again. The xterm instance and `TerminalPaneState` must remain
  mounted and untouched across this close/reopen; only the socket lifecycle
  changes."
- "Depends on Phase 1 (multiple simultaneously-mounted, non-visible panes) to
  have a meaningful population of sockets to gate." Phase 1 has landed on
  this branch (per-work-root instances kept mounted via `display:none`).
- "Should land after Phase 4's cursor-accuracy fix, or explicitly accept minor
  duplicate-output-on-resume until Phase 4 lands." — **This phase proceeds
  under the explicit-acceptance branch**; Phase 4 has not landed on this
  branch. See Escalations for the concrete mechanism this risk comes from.
- Spec Impact: "Contract-first: no" for Phase 3 — internal lifecycle change
  only, no new browser-visible persisted contract, no spec update in this
  phase.

## Out of Scope

- Phase 4 (frontend `nextSequence` update on `output` frames; backend
  truncation/gap signal in `crates/daemon/src/terminal.rs`). Not implemented
  here; its absence is the source of the accepted duplicate-output risk
  below.
- Phases 5-7 (layout/terminal-visual persistence and restore-on-reopen). Not
  touched.
- Daemon-side subscriber-count awareness
  (`crates/daemon/src/terminal.rs:686-755` per ticket background) — Phase 3 is
  frontend-only; the daemon already tolerates a closed/reopened client
  socket via the existing `after`-cursor backfill path, no backend change
  needed.
- Any change to `closeTerminalPane`/`closeTerminal()` (daemon PTY
  termination) — this phase only gates the *browser* WebSocket, never the
  daemon terminal session.
- Any change to xterm instance lifecycle (creation/dispose effect, deps
  `[]`) or to `TerminalPaneState` shape/fields beyond `socketStatus` value
  usage — only the socket-open effect changes.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:6462-6533` — the current terminal socket
  effect (ticket's cited `6235-6306` has drifted; this is the current
  location post-Phase-1/2). Effect deps are `[terminalId]` only; it opens a
  `WebSocket` unconditionally on mount/terminalId-change and closes it only
  on cleanup (unmount or `terminalId` change). This is the effect to extend.
- `ws-dashboard/frontend/src/App.tsx:6462-6471` — socket creation already
  calls `terminalWebSocketUrl(terminalId, terminalWebSocketCursor(liveRef.current.pane), window.location, liveRef.current.pane.session.serverRoute)`
  fresh on every effect run, i.e. **the resume-cursor mechanism this phase
  needs already works exactly as required** — re-running this effect after a
  gated close automatically resumes from the pane's current
  `nextSequence - 1`. No new resume logic needed; the phase’s only job is
  controlling *when* the effect body runs.
- `ws-dashboard/frontend/src/terminals.ts:162-163` — `terminalWebSocketCursor(pane) => Math.max(0, pane.nextSequence - 1)`, pure and already unit-tested
  (`terminals.test.ts:264`). Reused unchanged.
- **Key finding — existing visibility signal already matches the ticket's
  exact definition, no prop plumbing needed:**
  `ws-dashboard/frontend/node_modules/dockview-core/dist/cjs/dockview/components/panel/content.js:81`
  — Dockview hides an inactive-in-group panel by setting
  `this.element.style.display = 'none'` on its content wrapper. Combined
  with Phase 1's `ws-dashboard/frontend/src/App.tsx:4781-4784`
  (`.workbench-root-instance` gets inline `style={{ display: "none" }}` for
  every non-selected root), **both halves of the ticket's visibility
  definition — "work root is not the active one" and "dockview group is not
  frontmost" — collapse to the same DOM condition**: the terminal pane's own
  container element has no `offsetParent`. Dockview's default panel renderer
  keeps the React component mounted (matching the ticket background's "tab
  switching already avoids destroy... hidden via CSS" claim) while
  `display:none`-ing its wrapper, and Phase 1 does the same at the root
  level. There is no "maximize a dockview group" feature enabled anywhere in
  this codebase (checked `dockviewBridgeOptions`,
  `ws-dashboard/frontend/src/workbench/dockviewBridge.ts`; grep for
  `maximiz` only matches a test-double type-guard in
  `workbenchModel.test.ts:549,642-643`), so groups placed side-by-side via
  splits are always simultaneously visible — "not frontmost" in practice only
  means "not the active tab within its own group," which is exactly what
  Dockview's `display:none` already encodes. This means Phase 3 does **not**
  need to thread `activePaneByGroup`/`isSelectedRoot` down through
  `buildWorkbenchEditorGroups` → `terminalWorkbenchPanesByGroup` →
  `terminalWorkbenchPane` at all.
- `ws-dashboard/frontend/src/App.tsx:6271-6274` and `6426-6429` — the
  `container.offsetParent` visibility check is **already an established idiom
  in this exact component** (used by `keydownFallback` and the
  `focusWatchdog` interval), confirming this is the intended/existing way
  this codebase detects "is this terminal pane's DOM actually on screen"
  rather than a new pattern being introduced.
- `ws-dashboard/frontend/src/App.tsx:6422-6436` (`focusWatchdog`, a
  `window.setInterval(..., 100)` inside the xterm-mount effect, deps `[]`) —
  already polls `container.offsetParent` every 100ms, but only acts on it
  when `keepTerminalFocusRef.current` is true (early-returns otherwise).
  Reuse/extend this single interval to also drive the new visibility
  `useState`, rather than adding a second per-pane timer.
- `ws-dashboard/frontend/src/terminals.ts:59` —
  `socketStatus: "disconnected" | "connecting" | "connected" | "fallback"`.
  `"disconnected"` is already the pane's initial default
  (`terminals.ts:318`) and is otherwise unused as a *reason* label, so it is
  safe to reuse for "socket intentionally closed because pane is not
  visible," distinct from `"fallback"` (reserved for the socket-error /
  exit-triggered HTTP-fallback path, `terminals.ts:542`,
  `App.tsx:6510-6526`).
- `ws-dashboard/frontend/src/App.tsx:6224-6232` (`sendInputBytes`) — already
  falls back to `liveRef.current.actions.onSendData` (HTTP POST path,
  `sendTerminalInput` via `sendTerminalData` at `App.tsx:4447-4470`) whenever
  `socketRef.current?.readyState !== WebSocket.OPEN`. No new fallback wiring
  needed for the (rare/unexpected) case of input arriving while a pane's
  socket is gated closed.
- `ws-dashboard/frontend/src/App.tsx:4409-4427` (`applyTerminalSocketMessage`)
  — early-returns for `message.type === "output"` (`if (message.type === "output") { return; }`), so `pane.nextSequence` is only advanced by trailing
  `status`/`exit` frames today. This is the current (not-yet-fixed) state
  matching Phase 4's still-open scope — confirms the ticket's claimed line
  numbers (`frontend/src/App.tsx:4194-4212`) have drifted but the underlying
  gap it describes is still present and unresolved on this branch.
  **This is the exact mechanism behind the accepted duplicate-output risk**:
  when a socket is gated closed mid-batch (i.e., between two `status`
  frames), `pane.nextSequence` may be stale, so reopening with
  `terminalWebSocketCursor(pane)` can re-request and re-render already-seen
  output chunks. Ticket explicitly permits accepting this until Phase 4
  lands.
- `ws-dashboard/frontend/src/terminals.test.ts` and
  `terminalCommandPlan.test.ts` — the only automated coverage for terminal
  logic; both are pure-function unit tests run via `npm run test:terminals`,
  none render `App.tsx` JSX or exercise `useEffect`/DOM timing. Per Phase 1/2
  Results, no test harness in this repo renders `App.tsx` — this phase's
  actual effect-gating behavior is only verifiable by Playwright e2e
  (`dashboard-acceptance.spec.ts`), which cannot run in this sandbox
  (`libasound.so.2` missing, no Chromium binary — same pre-existing gap
  Phase 1/2 already documented).

## Implementation Plan

1. **`ws-dashboard/frontend/src/App.tsx` — add a `paneVisible` state to
   `TerminalPaneBody`** (near the existing `useState`s at ~6170): `const
   [paneVisible, setPaneVisible] = useState(true);` (optimistic default —
   matches current always-connect behavior for the common case of a newly
   mounted, actually-visible pane; a pane mounted while already hidden, e.g.
   a background root's pre-existing panes, briefly opens then closes on the
   first watchdog tick, an accepted minor inefficiency, not a correctness
   issue).

2. **Extend the existing `focusWatchdog` interval (`App.tsx:6422-6436`)**
   inside the xterm-mount effect (deps `[]`) to unconditionally compute
   `const nowVisible = Boolean(container.offsetParent);` on every tick and
   call `setPaneVisible((current) => current === nowVisible ? current : nowVisible);`
   before the existing focus-only early-return logic. Do not add a second
   interval — one poll driving both concerns keeps per-pane timer count
   unchanged.

3. **Gate the socket effect (`App.tsx:6462-6533`)**:
   - Add `paneVisible` to the dependency array: `}, [terminalId, paneVisible]);`.
   - At the top of the effect body, add:
     ```ts
     if (!paneVisible) {
       liveRef.current.actions.onSocketStatus(liveRef.current.pane, "disconnected", null);
       return;
     }
     ```
     This makes the effect a no-op while hidden. When `paneVisible` flips
     `true → false`, React runs the *previous* run's cleanup (closing the
     live socket via the existing `socket.close()` cleanup, unchanged) before
     this no-op body runs. When it flips `false → true`, the effect body runs
     normally and opens a fresh socket using
     `terminalWebSocketCursor(liveRef.current.pane)` exactly as today —
     already-correct resume behavior, no changes needed there.
   - Do not change any other part of the effect body (socket message
     handling, error/close listeners, cleanup) — only the guard and the dep
     array change.

4. **No changes needed to** `buildWorkbenchEditorGroups`,
   `terminalWorkbenchPanesByGroup`, `terminalWorkbenchPane`,
   `buildEditorGroupsForRoot`, `TerminalPaneActions`, or any dockview
   layout/model file — the visibility signal is self-contained inside
   `TerminalPaneBody` via its own `containerRef`, per the Codebase Findings
   above.

5. **No changes needed to** `crates/daemon/src/terminal.rs` — the daemon's
   existing `after`-cursor backfill path already supports being asked for
   output starting from an arbitrary cursor on each new connection; a
   closed/reopened client socket is indistinguishable from any other
   reconnect from the daemon's point of view.

## Verification Plan

- `npm run build` (tsc -b + vite build) inside `ws-dashboard/frontend` — must
  pass; primary signal for the effect/dependency-array change's type
  correctness.
- `npm run test:terminals` — must continue passing unchanged (no pure-helper
  signatures touched; `terminalWebSocketCursor`/`markTerminalSocketStatus`
  reused as-is).
- `npm run test:workbench` — regression guard, unaffected by this phase.
- Playwright e2e (`dashboard-acceptance.spec.ts`) is the only test surface
  that could exercise the actual close-on-hide/reopen-on-show behavior
  end-to-end; per Phase 1/2 Results this cannot run in this sandbox
  (`libasound.so.2` missing, no Chromium binary) — note this gap explicitly
  in the phase report rather than silently skipping it.
- Manual/structural verification: confirm via code reading (or a local dev
  run outside this sandbox) that (a) switching the active work root closes
  the WebSocket for the now-backgrounded root's terminal panes within one
  watchdog tick (~100ms) while their xterm DOM/scrollback stays intact, (b)
  switching back reopens the socket and resumes without a full output
  replay, (c) switching tabs within a single root's dockview group (not
  root-switching) also gates the backgrounded tab's socket the same way,
  since both cases resolve to the same `offsetParent` check.

## Escalations

- None.
