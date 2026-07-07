# Plan: 260707-bug-dashboard-terminal-clears-on-tab-switch — Phase 1: Root-cause and fix terminal clear/1-row-collapse on tab or session switch

> Research status (2026-07-07, second pass): the reported symptom
> (`<cols> x 1` collapse + "cleared" content) has now been **live-reproduced**
> against a real Chromium via Playwright. The trigger is NOT the hidden/detached
> state the first plan and the first escalation both chased (that path is
> NaN-guarded and safe). The trigger is a **visible-but-too-short pane height**
> reaching `fitNow()`. See Codebase Findings for the captured evidence.

## Relevant Ticket Contract
- Reproduce reliably; identify which layer owns the regression (dockview
  visibility lifecycle, xterm `fit()`/`resize()` call site, or the Phase 6/7
  restore/reattach write path) — do not assume.
- Fix without regressing the just-landed visual-buffer-restore behavior
  (Phase 6/7).
- Add/extend a Playwright e2e regression test in
  `e2e/dashboard-acceptance.spec.ts`.
- Verification order: reproduce first (screenshot/trace before fix), then
  confirm fixed via new/extended e2e coverage, run at least twice to rule out
  flake.
- Ticket Background hypothesis (now partially corrected by evidence): the
  "cleared" look is xterm re-rendering into a 1-row viewport, not data loss in
  the serialized snapshot. **Confirmed true** — the serialized buffer and PTY
  content survive; the collapse is a resize/fit artifact.
- Ticket Background names two repro modes ("dockview tab switch" and
  "navigating away to another session/workroot and coming back"). **See
  Escalations**: at a normal (tall) viewport neither of those alone reproduces
  the collapse; the actual trigger is a short *visible* pane height that can
  occur transiently during those switches (or with a short window / short
  split). The regression test must reproduce via that confirmed trigger.

## Out of Scope
- Any change to the Phase 6/7 serialize/persist contract
  (`workbench/terminalVisualRestore.ts`) — live repro confirms it is not the
  culprit; the restored buffer reconstructs intact even across the wrap-at-80 →
  reflow-to-153 remount path (see findings).
- General dockview layout/pane-order persistence (`workbench/layoutRestore.ts`).
- The minor cosmetic restore-seam garble observed at the very top of the
  restored scrollback on reload (overlapping command-echo lines where the
  persisted snapshot tail meets the socket replay) — it is not the reported
  clear/collapse symptom, it does not lose the bulk buffer or collapse rows, and
  chasing it here would exceed this phase. Capture as a separate `idea/` ticket
  if desired.
- Any future phase content in the ticket (none exists beyond Phase 1).

## Codebase Findings

### Live-repro evidence (2026-07-07, real Chromium via Playwright, daemon-served `dist`)
- **Mechanism #3 (multi-workRoot switch) — NEGATIVE.** Opened workRoot A, ran a
  terminal command, opened workRoot B, switched back to A (and repeated 3x).
  Terminal stayed `153x54`, emulator row count stayed `54`, all content intact.
  Reason: a non-active workRoot's workbench root is hidden via
  `App.tsx#L5146-5147` `style={{ display: "none" }}` — the terminal pane stays
  **mounted** (not remounted) and its canvas content is preserved; on return
  the socket reconnects from the sequence cursor and nothing collapses.
- **Mechanism #2 (reload → Phase 6/7 restore mount-write-before-fit) —
  NEGATIVE for the reported symptom.** Reloaded 3x with a line deliberately
  wider than the 80-col default mount width (to force wrap-at-80 →
  reflow-to-153 on the restore write). Stayed `153x54`, emulator `54`, and the
  wide line reconstructed **intact** every time. Only a minor cosmetic seam
  garble at the top of scrollback (out of scope, above). Note: after reload the
  layout-restore reactivates the E2E-agent tab, not the Terminal tab — the test
  must click the Terminal tab before asserting.
- **Mechanism #1 (`fitNow` collapse on short visible pane) — REPRODUCED, exact
  symptom.** At full viewport: emulator `54` rows. Shrinking the browser
  viewport height to `160px` (pane body becomes a few px tall while the pane is
  still visible, `offsetParent != null`) → footer reads **`RUNNING · 153X1`**,
  emulator collapses to **1 row**, visible content cleared to just the prompt.
  At `90px` still `153x1`. Growing back to `900px` → recovers to `153x54` with
  content restored. Screenshot evidence captured `153X1` with a single prompt
  row. **This is the ticket's `<cols> x 1` symptom exactly, and it is
  transient/height-dependent — which is why the first escalation's tests
  (visibility toggles at a fixed tall 1440x900 viewport) never saw it.**

### Root cause
- `ws-dashboard/frontend/src/App.tsx#L6763-6779` — `fitNow()`. On a **visible**
  container whose usable height is very small, `fitAddon.fit()` proposes
  `rows = Math.max(MINIMUM_ROWS=1, floor(availableHeight/cellHeight))` = `1`
  (vendor `@xterm/addon-fit/src/FitAddon.ts#L82-87`), and/or the local shrink
  loop `while (terminal.rows > 1 && !terminalScreenFitsVisibleBox(container))
  terminal.resize(terminal.cols, terminal.rows - 1)` (`L6776-6778`) drives rows
  down. Either way `terminal.rows` becomes `1`. `fit()` also calls
  `core._renderService.clear()` on the size change, producing the "cleared"
  look. This is the **only** shrink path and it has **no lower-bound / short-
  container guard**. Primary driver in the repro was `fit()` proposing `1`
  (the shrink loop's `rows > 1` guard makes it secondary; a guard must still
  cover it because it can drive `3 → 1` on a short container).
- The degenerate size is then **forwarded to the daemon PTY**:
  `forwardSize()` (`App.tsx#L6781-6834`) reads the now-collapsed
  `terminal.rows` and sends a `resize` frame; `terminals.ts#L87-91`
  `terminalSizeBounds.minRows = 1` **accepts `rows = 1`**, so the collapse
  becomes a real PTY-side resize, and the footer (`displaySession`, set via
  `onSocketResize`) shows `153x1`. Confirmed: the repro footer read `153X1`.
- The **first plan's proposed guard (`container.offsetParent === null`) would be
  a no-op** here: during the collapse the pane is *visible* (`offsetParent !=
  null`) but *short*. The correct signal is the **measured/proposed height**,
  not the hidden/detached state. This is the key correction over the first plan
  and the first escalation.
- Why it "sticks" on a real tab/session switch (not just a momentary shrink):
  during dockview relayout the pane can be measured short **while visible** for
  a frame; the `ResizeObserver` (`App.tsx#L6852-6853`) fires `fitNow()`
  synchronously, collapsing + forwarding `153x1`. If the settle-to-full-height
  frame is observed while the pane is momentarily hidden/detached (NaN → vendor
  early-return, no correction) or does not re-fire the observer, the collapsed
  `153x1` persists until the next incidental resize. The fix therefore needs
  both a guard against collapsing on a short measurement **and** a deterministic
  corrective refit when the pane returns to a real size/visibility.

### Test infrastructure
- `e2e/dashboard-acceptance.spec.ts` — reuse module-scope helpers:
  `openWorkRootInBrowser` (`L475`), `terminalTabs` (`L313`, the `Terminal` tab
  locator), `runInTerminal` (`L307`), `terminalSurface` (`L258`),
  `commandPlan.echo` / `commandPlan.scrollLines`. Terminal creation:
  `page.locator('[data-command-id="terminal.create"]').click()` (as in the
  existing "create terminal" step, `L2314`). Terminal footer size:
  `.terminal-status-line` renders `<status> · <cols>x<rows>` (`App.tsx#L7054-
  7057`); the existing `terminalColumns` helper (`L619-624`) parses it but
  returns only `match[1]` — add a `terminalRows` sibling returning `match[2]`.
  Emulator row count proxy for asserting a true emulator collapse (independent
  of the daemon-confirmed footer): `.xterm-rows > div` child count equals
  `terminal.rows`.
- The confirmed repro drives `page.setViewportSize({ width: 1440, height: N })`
  (N small, e.g. `160`) to force the short-pane collapse, then restores a tall
  viewport to assert recovery. This reproduces deterministically without any
  daemon-timing luck. (A pure dockview tab switch at a tall viewport does NOT
  collapse — do not assert that it does.)
- Playwright config viewport default is `1440x900`
  (`playwright.config.ts#L21`). Daemon harness auto-spawns the daemon and
  scrapes the pairing URL (`e2e/daemonHarness.ts`); `beforeAll` sets up
  `workRoot`/`secondWorkRoot` and navigates via `daemon.pairingUrl`.

## Implementation Plan

1. **Reproduce first (ticket verification order).** Add a temporary
   Playwright measurement (or reuse the researcher's approach): create a
   terminal, capture emulator rows + footer size at a tall viewport, then
   `page.setViewportSize` to a short height (e.g. `160px`), assert the collapse
   to `<cols>x1` (screenshot/trace), then restore a tall viewport. This is the
   pre-fix evidence the ticket requires. (Confirmed to reproduce; the guarded
   fix must make the short-viewport assertion flip from "collapses" to "clamped
   to a minimum / preserved".)

2. **Guard `fitNow()` against a degenerate short-container collapse**
   (`App.tsx#L6763-6779`). Contract: `fitNow()` must never apply a fit result
   that would set `terminal.rows` to the degenerate floor (`1` /
   `FitAddon.MINIMUM_ROWS`) because the *visible* container is too short to host
   a real grid; instead it preserves the current (last-good) emulator size and
   returns without resizing. Use the fit-relevant measured signal, not the
   hidden/detached signal:
   - Read the proposed dimensions before applying — `fitAddon.proposeDimensions()`
     is a public method (`@xterm/addon-fit/src/FitAddon.ts#L50`) returning
     `{ cols, rows }` or `undefined`. If it is `undefined` or `rows <= 1` (i.e.
     the container is unmeasurable or too short), skip the resize/shrink-loop
     entirely and return, leaving `terminal.rows`/`cols` at their last good
     values. This is the cheapest way to distinguish "genuine 1-row fit for a
     short pane" from a normal fit, without hardcoding pixel thresholds.
   - The same guard must prevent the shrink loop (`L6776-6778`) from being
     entered in the degenerate case (skip it when the guard tripped), so it
     cannot drive `rows` down to `1` on a short container either.
   - Do not weaken the existing `try/catch` around `fitAddon.fit()` or the
     `clampTerminalSize` PTY-bound clamp — both stay.

3. **Deterministic corrective refit on return to a real size/visibility**
   (`App.tsx#L6905-6919` `paneVisible`-gated effect and/or the `focusWatchdog`
   visibility-transition branch at `L6855-6857`). Contract: when `paneVisible`
   transitions `false → true` (pane shown again after a tab/session/workRoot
   switch), explicitly run `fitNow()` and, if it changed the size,
   `forwardSize()`, rather than relying solely on the next incidental
   `ResizeObserver` callback. Combined with step 2, this guarantees a pane that
   was collapsed (or skipped-while-short) is re-fitted to its real size the
   moment it is genuinely visible and measurable. Reuse the existing `fitNow`/
   `forwardSize` closures (do not duplicate fit logic).

4. **Confirm no degenerate size can be forwarded to the daemon.** With step 2
   in place, `forwardSize()` (`App.tsx#L6781-6834`) reads a preserved (non-
   collapsed) `terminal.rows`, so a `rows = 1` frame is no longer produced from
   a transient short measurement. Verify there is no other call site that
   reaches `terminal.resize`/`forwardSize` while the container is short. Do not
   change `terminalSizeBounds.minRows` (`terminals.ts#L90`) — the PTY contract
   floor is a separate concern; the fix is to stop *generating* the degenerate
   size, not to reject it at the bound.

5. **Add the Playwright e2e regression test** in
   `e2e/dashboard-acceptance.spec.ts`. Add a `terminalRows` helper (sibling of
   `terminalColumns`, `L619-624`, returning `match[2]`). The test, reusing
   `openWorkRootInBrowser` / `terminal.create` / `runInTerminal` /
   `commandPlan`:
   - open a workRoot, create a terminal, run a command producing multi-row
     output, record footer `<cols>x<rows>` and the emulator `.xterm-rows > div`
     count and a visible output marker at a tall viewport;
   - shrink the viewport to a short height (e.g. `160px`), then assert the
     terminal does **not** collapse to `<cols>x1` and content is not cleared
     (post-fix expectation: rows stay `> 1` / preserved, marker still present);
   - restore a tall viewport and assert rows/cols return to the expected value
     and the marker is present;
   - additionally exercise an in-session dockview tab switch and a
     session/workRoot round-trip (per the ticket's literal repro-mode list) as
     regression coverage that they remain stable (they already pass pre-fix —
     include them so the mandated modes are asserted, but the load-bearing new
     assertion is the short-viewport one).
   Run the new test at least twice locally to rule out flake.

## Verification Plan
- Manual/automated repro capture (screenshot) **before** the fix showing
  `<cols>x1` in the footer after a short-viewport transition (already captured
  in research; the executor should re-capture on their branch per the ticket's
  "reproduce first" clause).
- `cd ws-dashboard/frontend && npm run build && (cd .. && cargo build -p ws-dashboard-daemon)`
  then `npx playwright test e2e/dashboard-acceptance.spec.ts -g <new test name>`
  — run at least twice.
- Full `npx playwright test e2e/dashboard-acceptance.spec.ts` pass (no
  regression to existing Phase 5/6/7 layout/visual-restore coverage).
- Node unit tests for touched pure modules stay green
  (`workbench/terminalVisualRestore.test.ts`, `terminals.test.ts`) — confirms
  the restore write path and PTY bounds were left untouched.

## Escalations

### Resolved (2026-07-07): reframe of the ticket's two mandated repro modes, approved

Approved as proposed: keep the ticket's two named repro modes (in-session tab
switch, session/workRoot round-trip) as stability regression assertions, but
make the short-viewport transition the load-bearing new assertion, since that
is the confirmed actual trigger. Do not invest in constructing an artificial
dockview-relayout sequence to force a transient short-visible frame on a bare
tab switch — that would be higher effort, flakier, and no closer to the real
defect than directly asserting the confirmed trigger. This does not
contradict the ticket's verification boundary since both named modes remain
covered as non-regressions; it only redirects which assertion carries the
proof of the fix.

### Reframe of the ticket's two mandated repro modes (needs lead awareness, not a blocker)
- **Confidence: high (live-reproduced both ways).**
- **Evidence:** At the default tall viewport (1440x900), neither a pure in-
  session dockview tab switch nor a full session/workRoot round-trip (open
  second root, switch back; and full page reload with Phase 6/7 restore)
  reproduces the collapse — terminal stays `153x54` with content intact.
  Shrinking the *visible* pane's height to a few pixels (`setViewportSize`
  height `160`/`90`) reproduces `153x1` + cleared content immediately, and it
  recovers when height grows. The ticket Background attributes the symptom to
  the tab/session switch itself; the true trigger is a **short visible pane
  height**, which a tab/session switch can hit only transiently during relayout
  (or when the window/split is short), not deterministically at a tall viewport.
- **Required lead decision:** The plan keeps the ticket's two mandated repro
  modes as regression assertions (they must stay non-collapsing) but makes the
  **short-viewport transition** the load-bearing new regression assertion, since
  that is what actually reproduces the defect. This slightly reframes — without
  contradicting — the ticket's stated verification boundary ("both reproduction
  modes"). Confirm this reframing is acceptable, or direct whether the executor
  should instead invest in constructing a dockview relayout sequence that forces
  a transient short-visible frame on a bare tab switch (higher effort, flakier,
  and not clearly closer to the real user-visible defect).

Otherwise: **None** — root cause is identified and reproduced, the fix location
and contract are settled, and a clean guard using the existing
`fitAddon.proposeDimensions()` / `fitNow` / `forwardSize` mechanisms is
specified. No shortcut or bypass path is required.
