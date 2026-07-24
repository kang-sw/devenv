# Plan: 260724-bug-dashboard-terminal-dead-shell-undetected-steady-state — Phase 2: Frontend retirement of dead panes

## Relevant Ticket Contract

- Panes whose `session.status` is `exited`/`terminated`/`error` visually
  retire via **gray-out + an explicit clear affordance** — retain-with-clear,
  not auto-remove, so the exited shell's final scrollback stays readable.
- Detection already arrives live over the WS status/exit frame; no new
  detection mechanism is needed for the socket-connected case.
- Add only a **coarse** (seconds-scale) `listTerminals` reconciliation
  backstop for the daemon-side `admits_attach()` drop-off, scoped to when the
  WS is in fallback — far coarser than the existing 120ms output poll.
- Make close idempotent: treat `404` from `close_terminal` as success in
  `closeTerminalPane` so the auto-reap/manual-close race does not surface a
  spurious "terminal close failed".
- Phase-local verification: a terminal whose helper reports
  `exited`/`error`/`terminated` visually retires without a work-root switch;
  the clear affordance removes the pane; a manual close on an
  already-retired/already-gone pane does not surface "terminal close
  failed"; the reconciliation poll only needs asserting when the WS is
  forced into fallback.

## Out of Scope

- Phase 1 (Windows helper-side reaper) — already shipped, not touched here.
- Phase 3 (Unix regression test extension, native-Windows acceptance walk).
- Server-side changes of any kind (`crates/daemon/src/terminal.rs` already
  returns 404 correctly on close of a missing/already-removed terminal at
  both the `get` guard (`terminal.rs:788-790`) and the `remove` guard
  (`terminal.rs:794-796`); no server edit is needed for the idempotent-close
  requirement).
- The Dockview tab-X close-confirmation flow (`WorkbenchClosePopover`,
  `App.tsx:6192-6233`, `workbench/surfaceRegistry.ts` `closeConfirmationPolicy`)
  — orthogonal to the in-pane "Terminate"/clear button this phase touches;
  ticket does not ask for its behavior to change.
- Daemon-side steady-state heartbeat — explicitly rejected in the ticket's
  "Not doing" section.

## Codebase Findings

- `ws-dashboard/frontend/src/terminalPaneBody.tsx:749-773` — the pane's
  "Terminate" button (`data-command-id="terminal.close"`) already calls
  `actions.onClose(pane)` directly (wired to `closeTerminalPane` at
  `App.tsx:4305`), and the status line already renders
  `displaySession.status`. This is the natural attachment point for both the
  gray-out class and the relabeled "Clear" affordance — no new command or
  dispatch path is required, the existing `data-command-id` stays.
- **Line-anchor drift vs. ticket text (evidence-based correction):** the
  ticket cites `terminalPaneBody.tsx:589-594` for "status frame arrives
  live" — that range is actually inside the socket `"open"` handler's resize
  catch-up comment. The real status/exit handling is the `else` branch of
  the `"message"` listener at `terminalPaneBody.tsx:614-619`
  (`setDisplaySession((current) => ({ ...current, status: message.status }))`)
  plus the parent-level merge in `terminals.ts:792-804`
  (`appendTerminalWebSocketMessage`, sets `pane.session.status`). Confirm
  against current source, not the ticket's line numbers, before editing.
- **`displaySession` vs. `pane.session.status` — real gating risk.**
  `displaySession` (`terminalPaneBody.tsx:117`, updated only inside the
  `"message"` WS listener at line ~614-619) does **not** update during HTTP
  fallback polling — only `pane.session.status` does, via
  `appendTerminalOutput` (`terminals.ts:597-622`, sets
  `session: { ...pane.session, status: output.status }` from the poll
  response). Since this phase explicitly cares about the fallback-poll path
  (that's the whole reason for the reconciliation backstop), gate the
  gray-out/retirement condition off the `pane` prop's
  `pane.session.status`, not the locally-mirrored `displaySession.status`,
  or a shell that dies while the WS is in fallback will never visually
  retire until the next WS message happens to arrive (which may never
  happen for a genuinely dead connection).
- `ws-dashboard/frontend/src/terminals.ts:9-19` (`TerminalSessionView`) and
  `terminals.ts:53-80` (`TerminalPaneState`) — `status` is
  `"running" | "exited" | "terminated" | "error" | string`; an existing
  precedent for status-based filtering is
  `terminalRestoreIntentsFromPanes`'s `pane.session.status === "running"`
  filter (`terminals.ts:409`). Reuse the same three-way literal set
  (`exited`/`terminated`/`error`) for the retirement predicate.
- `ws-dashboard/frontend/src/terminals.ts:334-344` (`closeTerminal`) — throws
  a bare `new Error(await terminalErrorMessage(response))` on any
  `!response.ok`, with no status code preserved on the thrown error, and is
  called from exactly one production site
  (`App.tsx:5756`, `closeTerminalPane`; the only other caller is
  `terminals.test.ts:1080`, a unit test). The minimal, single-call-site-safe
  fix is inside `closeTerminal` itself: treat `response.status === 404` as a
  normal (non-throwing) return, so the idempotency guarantee lives at the
  helper level and every current/future caller inherits it for free, instead
  of threading a status check through `closeTerminalPane`'s `.catch`.
- `ws-dashboard/crates/daemon/src/terminal.rs:784-799` (`close_terminal`) —
  confirmed both 404 branches already exist server-side (`get` miss at
  788-790, `remove` miss at 794-796, i.e. a TOCTOU between the access-check
  read and the remove already degrades to 404, not a panic/500). **Ticket's
  cited range `terminal.rs:737-745` is drift** — that range is actually
  `terminal_resize`, not `close_terminal`. No server-side change needed;
  this finding is verification-only, confirming the frontend-only 404 fix is
  safe to make unilaterally.
- `ws-dashboard/crates/daemon/src/terminal.rs:272-280` (`list_for_work_root`)
  — confirmed: filters live sessions with `session.admits_attach()`
  (`terminal.rs:931-937`, true while `Running` or inside the daemon grace
  window). This is the exact drop-off the reconciliation backstop must
  catch: once grace elapses, a session silently stops appearing in
  `list_for_work_root`'s output with no accompanying WS frame for a pane
  stuck in fallback.
- `ws-dashboard/frontend/src/terminals.ts:535-567`
  (`reconcileListedTerminalSessions`) — **directly reusable, no new
  reconciliation logic needed.** Already implements exactly "prune panes for
  this work root/serverRoute that are no longer in the daemon's live list,
  unless locally created after `pruneStartedAtMs`" plus merge-in of any
  listed session via `mergeListedTerminalSessions`. This is the same
  function the existing one-shot `listTerminals` mount effect already calls.
- `ws-dashboard/frontend/src/App.tsx:4453-4515` — the existing one-shot
  `listTerminals` effect (fires only on `workbenchModel?.root.id` /
  `...serverId` change, i.e. work-root switch/mount) is the structural
  precedent for the new coarse backstop: `listTerminals(rootId, serverRoute)`
  → `setTerminalPanes(current => persistTerminalPanesForWorkRoot(rootId,
  reconcileListedTerminalSessions(current, rootId, sessions,
  listStartedAtMs, serverRoute, terminalVisualRestoreRef.current),
  serverRoute))`. The coarse backstop should reuse this same apply-path
  (ideally by extracting it into a small shared callback so the two call
  sites cannot drift) rather than reimplementing list-diffing.
  `placeTerminalSessions`/`setTerminalPaneOrderByGroup` in that same effect
  is initial-placement policy for freshly-discovered panes and is not
  required on every coarse reconciliation tick — only
  `reconcileListedTerminalSessions` matters for retiring gone-from-daemon
  entries.
- `ws-dashboard/frontend/src/App.tsx:4946-5026` — the existing 120ms
  fallback output-poll effect is the interval-management structural
  precedent (per-work-root `useEffect` keyed on
  `[workbenchModel?.root.id, workbenchModel?.root.resourcePath.serverId]`,
  `window.setInterval`, in-flight guard, `cancelled` flag on cleanup). The
  new coarse backstop effect should mirror this shape at a much larger fixed
  interval (e.g. a new constant sibling to
  `terminalOutputPollIntervalMs` at `App.tsx:443`, seconds-scale — the
  `workRootActivityRefreshIntervalMs = 3_000` constant at `App.tsx:444` is
  an existing "coarse seconds-scale poll" precedent for the same order of
  magnitude), gated to only fire the `listTerminals` call when at least one
  pane in that root currently has `socketStatus === "fallback"`
  (`terminals.ts:53-80`'s `socketStatus` field) — i.e. skip the network call
  entirely when every pane is socket-connected, since the WS status/exit
  frame already covers that case per the ticket's own contract.
- `ws-dashboard/frontend/src/terminals.ts:806-813`
  (`shouldPollTerminalOutput`) — existing precedent for "is this pane in a
  degraded-transport state" (`socketStatus !== "connecting" && !==
  "connected"`), usable as a reference for the backstop's own gating
  predicate, though the backstop should key specifically off `"fallback"`
  (a real degraded state) rather than also matching the transient initial
  `"disconnected"` pane-construction default, to avoid firing on every
  freshly-created pane before its first connect attempt resolves.
- `ws-dashboard/frontend/src/styles.css:2003-2052` — `.terminal-pane`,
  `.terminal-controls`, `.terminal-status-line`, `.terminal-error` are the
  existing terminal-pane style hooks. Reusable muted/retired-state token
  vocabulary already exists elsewhere in this file and should be reused
  rather than introducing new raw colors: `--ws-color-text-disabled`
  (`styles.css:36`), and the `.resource-row-muted` /
  `.resource-row-error` pattern (`styles.css:2737-2745`, muted =
  `color: var(--ws-color-text-secondary)` + a state-colored left border,
  error = `background: var(--ws-color-notice-error)`) is the closest
  existing precedent for a status-driven pane treatment. `.action-button`
  variants already include `.action-button-danger`
  (`styles.css:3548-3554`) if the clear affordance's button should read as
  a distinct/destructive action from "Terminate" on a live pane; a plain
  relabeled `.action-button` is also consistent with the existing single
  "Terminate" button if the executor prefers not to introduce a second
  visual button style.
- `ws-dashboard/frontend/src/App.tsx:4305` — confirms `onClose:
  closeTerminalPane` is the actions wiring passed into `TerminalPaneBody`;
  `closeTerminalPane` itself is at `App.tsx:5749-5775` (ticket cites
  `5608-5628` — drift; same function, real location differs).

## Implementation Plan

1. **Idempotent close** — `ws-dashboard/frontend/src/terminals.ts:334-344`
   (`closeTerminal`): change the `!response.ok` guard so a `404` response
   returns normally instead of throwing (e.g.
   `if (!response.ok && response.status !== 404) { throw ... }`). No
   change needed in `closeTerminalPane` (`App.tsx:5749-5775`) or in
   `crates/daemon/src/terminal.rs` — both already degrade correctly to 404.
2. **Gray-out + clear affordance** —
   `ws-dashboard/frontend/src/terminalPaneBody.tsx`: derive
   `const isRetired = ["exited", "terminated", "error"].includes(pane.session.status)`
   from the `pane` prop (not `displaySession`, per the finding above) near
   the component body (~line 749), apply a new modifier class (e.g.
   `terminal-pane-retired`) on the outer `.terminal-pane` div when true, and
   swap the "Terminate" button's label to something like "Clear" when
   `isRetired` — the button keeps calling `actions.onClose(pane)` and keeps
   `data-command-id="terminal.close"` unchanged (already idempotent via
   step 1, and `terminate()` server-side is already safe to call on an
   already-dead session per the terminal.md module contract). Add the
   `.terminal-pane-retired` (and optionally `.terminal-controls` /
   `.terminal-status-line` companion) rule(s) to
   `ws-dashboard/frontend/src/styles.css` near the existing
   `.terminal-pane`/`.terminal-controls` block (`styles.css:2003-2052`),
   reusing `--ws-color-text-disabled` / the `.resource-row-muted` pattern
   for the gray-out treatment rather than a new raw color.
3. **Coarse reconciliation backstop** —
   `ws-dashboard/frontend/src/App.tsx`: add a new per-work-root
   `useEffect` (keyed the same way as the existing 120ms poll effect at
   `App.tsx:4946-5026`) that, on a new coarse interval constant (new sibling
   constant to `terminalOutputPollIntervalMs` at `App.tsx:443`, seconds-
   scale, e.g. named `terminalListReconciliationPollIntervalMs`), checks
   whether any pane for the current root has `socketStatus === "fallback"`
   and if so calls `listTerminals(rootId, serverRoute)` then applies the
   result through `reconcileListedTerminalSessions` +
   `persistTerminalPanesForWorkRoot` — the same apply-path already used by
   the mount effect at `App.tsx:4453-4515`/`4485-4498`. Prefer extracting
   that apply-path into one small shared function called from both effects
   over duplicating the `setTerminalPanes(...)` block, so the two paths
   cannot drift. Do not reuse/duplicate `placeTerminalSessions` — it is
   initial-placement policy, not needed for a reconciliation-only tick.
4. Confirm no other call site needs updating: `closeTerminal` has exactly
   one production caller (`App.tsx:5756`); the retirement predicate and
   gray-out styling are local to `terminalPaneBody.tsx` +
   `styles.css` and do not touch the Dockview tab-close-confirmation flow.

## Verification Plan

- `npm run build` (in `ws-dashboard/frontend/`) — must stay clean (`tsc -b
  && vite build`); required by the render prompt regardless of touched
  modules.
- `npm run test:terminals` — the correct focused target for every file this
  phase touches (`terminals.test.ts` covers `closeTerminal`'s 404 handling;
  the target also runs `terminalCommandPlan.test.js`,
  `workbench/terminalVisualRestore.test.js`, and the e2e-tests
  `daemonHarness.test.js` — all still relevant since they exercise the same
  `terminals.ts`/pane-lifecycle surface).
- Add/extend a `terminals.test.ts` case asserting `closeTerminal` resolves
  (does not throw) on a mocked `404` response, and a
  `reconcileListedTerminalSessions`-level case (or a new small pure-function
  test if the backstop's gating predicate is factored out) asserting a pane
  whose logical key drops out of a `listTerminals` response is pruned only
  when it predates `pruneStartedAtMs` — mirroring existing coverage
  patterns already in `terminals.test.ts`.
- Manual/phase-local checks per the ticket's own verification boundary
  (status-driven gray-out without a work-root switch; clear affordance
  removes the pane; already-gone-pane close does not surface "terminal
  close failed"; reconciliation poll only asserted when WS is forced into
  fallback) are dogfood-level, not currently automatable without a browser
  harness.
- Do NOT gate on `npm run test:browser` (Playwright) — independently RED on
  unrelated bug 260713 per the render prompt's instruction.

## Escalations

- None.
