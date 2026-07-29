# Plan: 260726-bug-dashboard-restored-tab-close-inert-until-activated — Phase 1: Make a never-activated, reload-restored terminal tab closable on its first `×` click

## Relevant Ticket Contract

- **D1 (always lands).** Route `onRequestClosePane` through a stable
  `callbacksRef` forwarder, the same pattern `onAcknowledgePane` already uses,
  regardless of which mechanism the discriminator confirms.
- **D2.** The fix must not depend on a pending attention badge and must not
  require the tab to be Dockview-active — both configurations must close on
  the first `×` click.
- **D3.** Frontend-only. No daemon/route/protocol change.
- **Decision rule** (ticket `## Candidate Fixes`, "Decision rule" subsection):
  1. Badged-only reproduction + geometry shift observed → land **F1** (badge
     `position: absolute`, tab `position: relative`); this is the ticket's
     stated expected outcome. F1' only if a design pass rejects F1's dot
     placement.
  2. Unbadged reproduction ALSO occurs with a geometry shift → the reflow is
     not badge-driven → land **F3** (native `pointerdown` `stopPropagation()`
     on the close button via a ref), and record the activation/acknowledgement
     behavior-change cost in the Result.
  3. No geometry shift, but the click still never reaches the button → land
     **F3**.
  4. **F2** only substitutes for F1 if a design pass rejects F1's placement.
     **F4 is forbidden** — an implementer may not select it from the rule; if
     evidence points only there, stop and escalate to the lead rather than
     writing it.
  5. D1 lands in every branch, including the no-reproduction branch.
- **No-repro branch.** If neither configuration reproduces the symptom, do not
  speculatively land a mechanism fix: land D1 plus the browser regression
  guards below, record the no-repro explicitly (configurations tried, browser,
  what was observed), and close the phase on that basis. The spec edits still
  land in this branch.
- **Verification boundary (binding).** Playwright browser acceptance against
  the daemon-served production frontend. The primary/binding assertion is the
  **badged** configuration (pending attention badge present, tab never
  clicked, reload-restored) — the only configuration the original defect was
  observed in. The **unbadged** configuration is the secondary assertion for
  D2's other half. Neither assertion may click the tab body first. Do not pin
  the badge's post-click `data-attention-state` value in the assertion — F1
  leaves today's ack-on-activate behavior unchanged, F3 would change it; assert
  the popover and the close, not the badge state.
- **Constraints.** Keep `data-command-id="workbench.tab.close"`,
  `data-workbench-close-popover="cursor-near"`,
  `data-workbench-tab-close-affordance`, `data-workbench-pane-id`,
  `data-attention-state` attributes unchanged. Do not touch
  `shouldUpdateDockviewWorkbenchPanelParams`'s churn policy. Do not "tidy" the
  deliberate `dockviewLayout.tsx:401-409` naming exception
  (`data-workbench-tab-close-affordance`, not `data-workbench-*`).
- **Out of scope for this phase (deferred):** other surface kinds (agent,
  agentChat, editor, workRootActivity); keyboard/command-palette/drag close
  paths; `shouldUpdateDockviewWorkbenchPanelParams` churn-policy changes; the
  stale `editorGroups`/`terminalPanes` capture in `performWorkbenchPaneClose`
  (`App.tsx:6548-6604`) unless the discriminator lands on the frozen-closure
  mechanism.
- **Spec Impact** (owned by this phase, both branches): add an
  activation/refresh-independence invariant to
  `260516-ws-web-dashboard-workroot-workbench-substrate`, and extend the
  workbench tab polish evidence sentence in
  `260516-ws-web-dashboard-browser-ui-acceptance-gate` to name the
  reload-restored never-activated close path. Exact wording branches on the
  discriminator outcome (see Implementation Plan step 6).

## Out of Scope

- Phases beyond Phase 1 (there is only one phase in this ticket).
- `260725-bug-dashboard-fitnow-short-viewport-shrink`'s pre-existing failure at
  `e2e/dashboard-acceptance.spec.ts:3779` — judge the browser gate by failure
  site, not exit code; that failure is not this ticket's to fix.
- Any Rust/daemon change — D3 is explicit and nothing in the verified close
  path (`App.tsx:6606-6624`, `terminal.rs` routes) needs to change; the daemon
  already terminates the session correctly once a confirmed close request
  reaches it.

## Codebase Findings

**Close path / D1 target:**
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx:146-170` —
  `callbacksRef` (refreshed every render) plus the existing stable-forwarder
  pattern: `const acknowledgePane = useCallback((paneId) => { callbacksRef.current.onAcknowledgePane?.(paneId); }, []);`. D1's fix is the same shape for
  `onRequestClosePane`.
- `dockviewLayout.tsx:192-229` (`syncPanels`) calls
  `syncDockviewWorkbench(apiRef.current, groups, activePaneByGroup, callbacksRef.current.onRequestClosePane, acknowledgePane)` at line ~202 — this is
  where the RAW callback is read fresh per render but then frozen into
  Dockview's stored panel params the next time (if ever)
  `shouldUpdateDockviewWorkbenchPanelParams` allows a push. Must become
  `closePane` (new stable wrapper), added to the `useCallback` dependency array
  at line 229 (currently `[acknowledgePane, activePaneByGroup, groups]`).
- `dockviewLayout.tsx:472-478` (`syncDockviewWorkbench` signature) and
  `dockviewLayout.tsx:597-619` (`toDockviewWorkbenchPanelParams`) — both take
  `onRequestClosePane` as a plain parameter and stash it verbatim into
  `params.onRequestClosePane`; no signature/type change needed, only the value
  passed in from `syncPanels` changes from the raw ref-read to the new stable
  wrapper.
- `dockviewLayout.tsx:454-464` — the `<button>`'s `onClick` reads
  `params.onRequestClosePane?.(...)` from whatever params blob Dockview is
  currently holding for that panel — this is the call site the frozen-closure
  risk affects; unchanged by D1 except that it now always calls a stable
  function that internally re-reads the latest callback.

**Lost-`click` mechanism / geometry (F1 target):**
- `dockviewLayout.tsx:440-446` — `.workbench-tab-attention` span is
  conditionally rendered (`params.attentionState ? <span .../> : null`),
  currently in normal flex flow as a sibling of the icon/title/button.
- `ws-dashboard/frontend/src/styles.css:1660-1670` — `.dockview-workbench-tab`
  is `display: flex; gap: 4px;` with **no fixed width** (content-width tab,
  `padding: 0 var(--ws-space-06)`), no `position` declared.
- `styles.css:2662-2668` — `.workbench-tab-attention` is `width: 7px; height:
  7px; flex: 0 0 auto;` (no `position`), so removing it collapses 7px width
  plus one 4px flex `gap` = 11px total, confirming the ticket's own ~11px
  estimate. `.workbench-tab-icon` (`styles.css:2625-2632`) is already
  `position: relative` (used for its own `::after`), which is a `flex: 0 0
  auto` sibling, not a parent, of the attention span — so F1's `position:
  absolute` badge needs `.dockview-workbench-tab` (the flex container) to
  become `position: relative`, not the icon.
- `styles.css:2079-2099` — `.workbench-tab-close` is `width: 14px; height:
  14px; flex: 0 0 auto; opacity: 0;` revealed via
  `.dockview-workbench-tab:hover`/`:focus-within`. No `pointer-events` or
  `visibility` suppression (confirms ticket correction 5's DOM-teardown
  falsification is irrelevant to CSS visibility too — the button is a normal,
  always-hoverable click target).
- `App.tsx:6741-6752` (`acknowledgePaneAttention`) and `App.tsx:6754-6767`
  (`selectPane`) — dockview's native `pointerdown`-driven activation (ticket
  correction 3,
  `node_modules/dockview-core/dist/esm/dockview/components/titlebar/tabs.js:487-520`)
  fires `onDidActivePanelChange` → `dockviewLayout.tsx:267-282`'s
  `onSelectPane` → `App.tsx`'s `selectPane` → `acknowledgePaneAttention` for
  `persistentTerminal` panes. This is what flips `attentionState` mid-gesture
  and is the mechanism correction 6 traces. F1 does not change this call
  chain — it only removes the geometry consequence.
- `dockviewLayoutModel.ts:56-74` — `shouldUpdateDockviewWorkbenchPanelParams`'s
  `persistentTerminal` branch: attention-state comparison runs before the
  connected-socket early return, so the badge-clear repaint reaches Dockview
  even for a connected tab. Not touched by F1 (ticket's rejected-alternative
  list forbids widening this predicate).

**Discriminator harness reuse (all in `ws-dashboard/frontend/e2e/agent-attention-indicator.spec.ts`):**
- `spawnTerminalInRoot` (:385-424) — creates a terminal via daemon route
  directly (`page.evaluate` + `fetch`, not the toolbar), so the tab is never
  clicked/activated in-browser. Confirmed by the file's own comment at
  `:493-497`/`:514-520`: a terminal created this way and then
  `page.reload()` + `selectWorkRootMinimal()`'d renders as a real Dockview pane
  that is **not** the active tab until explicitly clicked — this is the exact
  "never-activated, reload-restored" shape Phase 1 needs, already proven to
  work in this file for a different assertion.
- `readCallbackToken` (:293-311) — requires the `dummy-echo-hooked` profile
  (mints a token); `spawnTerminalInRoot(..., null, ...)` (plain shell, no
  token) is correct for the no-attention configuration.
- `postTurnState` (:317-337) — POSTs `working`/`ready`/`idle` from outside the
  browser via the per-terminal callback token; this is how the badge is set
  BEFORE the reload, matching ticket reproduction step 5.
- `closeTerminalById` (:339-355) — hover + click `[data-command-id="workbench.tab.close"]` + expect
  `[data-workbench-close-popover="cursor-near"]` visible + click
  `[data-command-id="workbench.tab.close.confirm"]` + expect popover gone.
  This is already exactly the ticket's binding-assertion shape and needs no
  modification — it only needs to be called against a tab that was never
  clicked first (every existing call site in this file clicks the tab before
  calling it, which is precisely the workaround the ticket's Background
  section flags as no longer proving anything).
- `terminalTab`, `terminalTabsLocator`, `resolveWorkRootId`,
  `selectWorkRootMinimal`, `openWorkRootMinimal`, `attachOwnerSession`,
  `forceCloseTerminals` (:198-291) — reusable as-is.
- `test.describe.configure({ mode: "serial" })` (:148) plus `ownerCookies`
  capture (:144-146, :198-214) — this file's established pattern for adding a
  further sibling `test(...)` that reuses the same daemon/workRoot/session
  without re-pairing, already used twice (`"nav row agent counter"` :651,
  `"browser-level title/favicon attention cue (Tier 1)"` :1037). Phase 1's new
  assertions should be a **fifth `test(...)` in this same file**, not a new
  spec file — this reuses the existing `workRoot` fixture (already opened by
  the file's first test) via `attachOwnerSession` + `openWorkRootMinimal`
  (idempotent re-open, same pattern the "nav row agent counter" test uses on
  the same `workRoot` at :653).

**Build-trap correction (per lead-supplied context, verified in source):**
- `ws-dashboard/frontend/playwright.config.ts:17` declares
  `globalSetup: "./e2e/globalSetup.ts"`.
- `ws-dashboard/frontend/e2e/globalSetup.ts:77-99` runs `npm run build`
  unconditionally before any test starts and hard-fails the run on a non-zero
  build exit — **except** two announced skip paths read through
  `parseDaemonHarnessConfig()`: `WS_DASHBOARD_STATIC_DIR` set, or external
  daemon mode (`WS_DASHBOARD_DAEMON_MODE=external` /
  `WS_DASHBOARD_DAEMON_BASE_URL` / `WS_DASHBOARD_DAEMON_PAIRING_URL`). Neither
  is in play for an ordinary local run.
- The ticket's own "Build trap" bullet (claiming no `webServer`/`globalSetup`
  exists and only `test:browser` rebuilds) is **stale** — it predates commit
  951b0f27. Mental-model note `ws-web-dashboard/index.md:247` documents this
  exact drift and confirms the frontend-build staleness hazard is closed on
  the default path.
- **Residual gap, not closed by `globalSetup.ts`:** `ws-dashboard/frontend/package.json:27`
  shows `cargo build -p ws-dashboard-daemon` still lives only inside the
  `npm run test:browser` script. A bare `npx playwright test` (which this
  phase's iteration loop will want to use, to target just
  `agent-attention-indicator.spec.ts`) rebuilds the frontend but **not** the
  daemon binary. Since this phase is frontend-only (D3), the daemon binary
  itself never needs rebuilding for this change's own sake — but if the
  implementer's `target/debug/ws-dashboard` binary predates this branch for
  unrelated reasons, a bare `playwright test` run would silently serve a stale
  daemon. Use `npm run test:browser` (or `cargo build -p ws-dashboard-daemon`
  once, then bare `playwright test` for fast iteration) rather than assuming
  `globalSetup` covers the daemon binary too — it does not.

## Implementation Plan

1. **Add a temporary discriminator harness** to
   `ws-dashboard/frontend/e2e/agent-attention-indicator.spec.ts` (module-local
   helpers below the existing ones, e.g. after `readCallbackToken`). This code
   is run once against **unfixed** source to gather the mechanism evidence the
   decision rule needs, then either kept (folded into the permanent test in
   step 5) or deleted once the permanent assertions replace it — implementer's
   call, but the measurement must actually run, not be reasoned about.

   ```ts
   // Spawns a terminal via the daemon route directly (never clicked in the
   // browser), optionally posts a turn-state before reload so the restored
   // tab carries a pending attention badge, then reloads + reselects the
   // root - the exact "never-activated, reload-restored" shape this ticket
   // is about. Asserts the restored tab is genuinely inactive as a
   // precondition, not an assumption.
   async function spawnRestoredNeverActivatedTerminal(
     page: Page,
     workRootId: string,
     rootPath: string,
     title: string,
     attention: "working" | "ready" | null,
   ): Promise<string> {
     const terminalId = await spawnTerminalInRoot(
       page,
       workRootId,
       attention ? "dummy-echo-hooked" : null,
       title,
     );
     if (attention) {
       const token = readCallbackToken(terminalId);
       await postTurnState(terminalId, token, attention);
     }
     await page.reload({ waitUntil: "domcontentloaded" });
     await selectWorkRootMinimal(page, rootPath);
     await expect(terminalTab(page, terminalId)).toHaveCount(1, {
       timeout: 20_000,
     });
     await expect(terminalTab(page, terminalId)).toHaveAttribute(
       "aria-selected",
       "false",
     );
     await expect(terminalTab(page, terminalId)).toHaveAttribute(
       "data-attention-state",
       attention ?? "none",
     );
     return terminalId;
   }

   // Orchestrates the close gesture by hand (mouse move/down/wait/up) instead
   // of Locator.click(), so the close button's geometry can be read BETWEEN
   // pointerdown and mouseup - the exact window ticket correction 6 says the
   // button shifts in. Returns enough evidence to fill in the decision rule:
   // whether the button moved, whether the FIRST click's mouseup (fired at
   // the ORIGINAL, pre-shift coordinates) reached the button, and whether a
   // clean SECOND click (freshly hovered/located) does.
   async function measureCloseGesture(page: Page, terminalId: string) {
     const tab = terminalTab(page, terminalId);
     await tab.hover();
     const closeButton = tab.locator('[data-command-id="workbench.tab.close"]');
     const beforeBox = await closeButton.boundingBox();
     if (!beforeBox) {
       throw new Error("close button not visible/hoverable before gesture");
     }
     const x = beforeBox.x + beforeBox.width / 2;
     const y = beforeBox.y + beforeBox.height / 2;

     await page.mouse.move(x, y);
     await page.mouse.down();
     // Give React's discrete-event flush (triggered by dockview's native
     // pointerdown activation) two animation frames to settle before the
     // stationary mouseup below.
     await page.evaluate(
       () =>
         new Promise<void>((resolve) =>
           requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
         ),
     );
     const duringBox = await closeButton.boundingBox();
     await page.mouse.up();

     const popover = page.locator(
       '[data-workbench-close-popover="cursor-near"]',
     );
     const firstClickOpenedPopover = (await popover.count()) > 0;

     let secondClickOpenedPopover = false;
     if (!firstClickOpenedPopover) {
       await tab.hover();
       await tab.locator('[data-command-id="workbench.tab.close"]').click();
       secondClickOpenedPopover = (await popover.count()) > 0;
     }

     const shiftPx = duringBox ? duringBox.x - beforeBox.x : null;
     console.log(
       `[Phase1 discriminator] terminal=${terminalId} beforeX=${beforeBox.x.toFixed(1)} duringX=${duringBox?.x?.toFixed(1) ?? "null"} shiftPx=${shiftPx?.toFixed(1) ?? "null"} firstClickOpenedPopover=${firstClickOpenedPopover} secondClickOpenedPopover=${secondClickOpenedPopover}`,
     );

     // Leave the popover open (or closed, if this run genuinely fixed it) for
     // the caller to decide how to finish - this helper only measures.
     return {
       beforeBox,
       duringBox,
       shiftPx,
       firstClickOpenedPopover,
       secondClickOpenedPopover,
       popoverVisible: firstClickOpenedPopover || secondClickOpenedPopover,
     };
   }
   ```

2. **Run the discriminator against current (unfixed) source**, twice — once
   badged, once unbadged — inside one new temporary `test(...)` block (fifth
   test in the file, serial mode, reusing `workRoot`):

   ```ts
   test("Phase 1 discriminator: badged vs unbadged never-activated close", async ({
     page,
   }) => {
     await attachOwnerSession(page);
     await openWorkRootMinimal(page, workRoot);
     const workRootId = await resolveWorkRootId(page, workRoot);
     let badgedId = "";
     let plainId = "";
     try {
       badgedId = await spawnRestoredNeverActivatedTerminal(
         page, workRootId, workRoot, "Discriminator Badged", "ready",
       );
       const badgedResult = await measureCloseGesture(page, badgedId);
       // If the popover opened, close it now so state doesn't leak into the
       // next measurement; if not, leave a record and move on.
       if (badgedResult.popoverVisible) {
         await page
           .locator('[data-workbench-close-popover="cursor-near"] [data-command-id="workbench.tab.close.confirm"]')
           .click();
       }

       plainId = await spawnRestoredNeverActivatedTerminal(
         page, workRootId, workRoot, "Discriminator Plain", null,
       );
       const plainResult = await measureCloseGesture(page, plainId);
       if (plainResult.popoverVisible) {
         await page
           .locator('[data-workbench-close-popover="cursor-near"] [data-command-id="workbench.tab.close.confirm"]')
           .click();
       }
     } finally {
       await forceCloseTerminals(page, [badgedId, plainId]);
     }
   });
   ```

   Run with:
   ```
   cd ws-dashboard/frontend && npx playwright test e2e/agent-attention-indicator.spec.ts -g "Phase 1 discriminator"
   ```
   (`globalSetup.ts` builds the frontend unconditionally first — see the
   build-trap correction above; make sure `target/debug/ws-dashboard` exists
   and is current, e.g. by having run `cargo build -p ws-dashboard-daemon`
   once beforehand, since this bare invocation does not rebuild it.)

   Read the two `console.log` lines this prints. Apply the decision rule:
   - Both `shiftPx` values ≈ 0 (no geometry shift) in a run where the
     popover still doesn't open on the first click → the mechanism is not
     geometry-shift-based (retained-alternative frozen-closure story,
     ticket correction 4) → still apply D1, then re-measure; if D1 alone
     fixes it, this is the no-repro-after-D1 shape, treat as case 5 (D1 only).
   - `badgedResult.shiftPx` ≈ -11 (button moved left ~11px, matching the 7px
     dot + 4px gap) and `plainResult.shiftPx` ≈ 0 → **rule case 1**: land F1.
   - Both results show a comparable shift → **rule case 2**: land F3, and
     record the tab-strip-scroll or other non-badge-driven reflow evidence.
   - No shift in either, but `firstClickOpenedPopover` is false and
     `secondClickOpenedPopover` is true in both → **rule case 3**: land F3.
   - If evidence supports none of the above and only a press/release dispatch
     (F4) would close the gap → **do not implement F4**; stop and escalate to
     the lead per the ticket's D-rule item 4 and the lead-supplied constraint
     that F4 requires an explicit owner decision not currently recorded.

3. **Land D1 unconditionally** in
   `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx`:
   - Add a stable wrapper next to `acknowledgePane` (after line 170):
     ```ts
     const closePane = useCallback((request: DockviewTabCloseRequest) => {
       callbacksRef.current.onRequestClosePane?.(request);
     }, []);
     ```
   - In `syncPanels` (line ~202), change
     `callbacksRef.current.onRequestClosePane` to `closePane` in the
     `syncDockviewWorkbench(...)` call, and add `closePane` to that
     `useCallback`'s dependency array (line 229).
   - No change needed to `syncDockviewWorkbench`'s or
     `toDockviewWorkbenchPanelParams`'s signatures — they already accept a
     function of the same shape.

4. **Land the rule-selected candidate fix.** Expected branch (rule case 1),
   **F1**:
   - `ws-dashboard/frontend/src/styles.css:1660-1670` — add `position:
     relative;` to `.dockview-workbench-tab`.
   - `styles.css:2662-2668` — change `.workbench-tab-attention` to `position:
     absolute` with a placement that overlaps the tab's existing icon/leading
     area rather than participating in flex flow (so it contributes zero
     width in either state). Exact offset is a visual-design call the ticket
     itself flags as a cost ("a visual reposition of the dot that the
     frontend-design pass must accept") — keep the badge visually near where
     it renders today (adjacent to `.workbench-tab-icon`, which is the tab's
     first flex child at `padding-left: var(--ws-space-06)`), and consult the
     `frontend-design` skill if the initial placement reads awkwardly before
     the required post-implementation design pass
     (`260516-ws-web-dashboard-browser-ui-acceptance-gate`'s "post-implementation
     frontend-design verification and autonomous tweak pass").
   - No JSX change needed in `dockviewLayout.tsx:440-446` — the span stays
     conditionally rendered (keeps the existing `toHaveCount(0)` assertion at
     `agent-attention-indicator.spec.ts:582-587` valid, per the ticket's F1
     framing).
   - Do NOT implement F1' (always-rendered badge with `data-workbench-tab-attention="none"`)
     unless the design pass specifically asks for it — F1 is the ticket's
     preferred default.

   If evidence instead selects **F3** (rule case 2 or 3): attach a native
   (non-React) `pointerdown` listener to the close `<button>` via a `ref` +
   `useEffect` inside `DockviewWorkbenchTab`
   (`dockviewLayout.tsx:597-619`/wherever the tab component is defined —
   confirm the exact component boundary before editing) that calls
   `event.stopPropagation()` on the **native** event, not React's
   `onPointerDown` (React's synthetic handler runs after dockview's own
   ancestor `pointerdown` listener has already fired — the same trap the
   stale `dockviewLayout.tsx:422-423` comment fell into for `click`). Record
   in the Result: clicking `×` on an inactive tab no longer activates or
   acknowledges it, for every surface kind — a deliberate, ticket-accepted
   behavior change (F3's stated cost (a)).

5. **Replace the temporary discriminator test with the permanent binding
   assertions.** Delete or repurpose the `console.log`-based measurement test
   from step 2 into the phase's actual regression coverage — a sixth
   (or edited fifth) `test(...)` in the same file:

   ```ts
   test("restored, never-activated terminal tab closes on its first x click", async ({
     page,
   }) => {
     await attachOwnerSession(page);
     await openWorkRootMinimal(page, workRoot);
     const workRootId = await resolveWorkRootId(page, workRoot);
     let badgedId = "";
     let plainId = "";
     try {
       await test.step(
         "PRIMARY/BINDING: badged, never-activated tab closes on first x click",
         async () => {
           badgedId = await spawnRestoredNeverActivatedTerminal(
             page, workRootId, workRoot, "Restored Badged Close", "ready",
           );
           await closeTerminalById(page, badgedId);
         },
       );
       await test.step(
         "SECONDARY (D2): no-attention, never-activated tab also closes on first x click",
         async () => {
           plainId = await spawnRestoredNeverActivatedTerminal(
             page, workRootId, workRoot, "Restored Plain Close", null,
           );
           await closeTerminalById(page, plainId);
         },
       );
     } finally {
       await forceCloseTerminals(page, [badgedId, plainId]);
     }
   });
   ```

   `closeTerminalById` is unchanged and reused verbatim — it already hovers,
   clicks `[data-command-id="workbench.tab.close"]`, asserts the cursor-near
   popover, confirms, and asserts the popover is gone; the only thing that
   changes is that the tab body is never clicked beforehand, which is exactly
   what makes this assertion non-vacuous where every existing call site in
   this file was not (per the ticket's Background section calling out
   `agent-attention-indicator.spec.ts:986-991`'s workaround).

   If step 4 landed F1: no additional geometry assertion is required, but
   optionally keep a lightweight regression guard (e.g. assert
   `beforeBox.x === duringBox.x` from `measureCloseGesture`, reused as a
   helper) if it does not meaningfully lengthen the test — this directly
   encodes the new spec invariant ("attention indicator must not change the
   tab's close-affordance geometry").

6. **Write the two spec additions** (owned by this phase, ticket `## Spec
   Impact`):
   - `ai-docs/spec/ws-web-dashboard/index.md`, in the
     `{#260516-ws-web-dashboard-workroot-workbench-substrate}` section
     (currently ends around line 1170 with "...do not show mock or default
     panes when no live or user-opened surface exists."): add a sentence
     stating that tab lifecycle affordances (close, acknowledge) stay live
     regardless of Dockview parameter refresh timing and regardless of
     whether the tab has ever been the active pane. If the fix landed F1 or
     F1', add that a tab's attention indicator must not change the tab's
     close-affordance geometry when it appears or clears.
   - `ai-docs/spec/ws-web-dashboard/index.md`, in the
     `{#260516-ws-web-dashboard-browser-ui-acceptance-gate}` section, extend
     the "Workbench tab polish evidence" paragraph (currently
     lines 1730-1737) so the covered close paths explicitly include a
     reload-restored, never-activated tab (not just the already-clicked path
     the sentence currently implies coverage of).
   - Write both from the actually-implemented/asserted behavior (contract-first:
     no), after the browser assertions in step 5 are green.

7. **No `dockviewLayoutModel.ts` change expected** (F1/F3 do not touch
   `shouldUpdateDockviewWorkbenchPanelParams`), so `npm run test:workbench` is
   not required by this phase's own changes — only run it if step 4 ends up
   touching that predicate (would only happen under the frozen-closure
   branch, which is out of the expected path).

## Verification Plan

- Iterate fast against just this spec file:
  `cd ws-dashboard/frontend && npx playwright test e2e/agent-attention-indicator.spec.ts`
  (relies on `playwright.config.ts`'s `globalSetup.ts` to rebuild the frontend
  unconditionally; ensure the daemon binary itself is already built at least
  once — see the build-trap correction in Codebase Findings).
- Full closing evidence: `cd ws-dashboard/frontend && npm run test:browser`
  (rebuilds both frontend and daemon binary, then runs the whole Playwright
  suite). Judge by **failure site**, not exit code: a non-zero exit is
  acceptable evidence only if `e2e/dashboard-acceptance.spec.ts:3779`
  (`expect(shortRows).toBe(terminalClearFixTallRows)`, owned by
  `260725-bug-dashboard-fitnow-short-viewport-shrink`) is the sole failing
  site. Any failure in `agent-attention-indicator.spec.ts` or any other file
  is this ticket's to fix.
- No Rust/cargo test run is required (D3, frontend-only); `cargo build -p
  ws-dashboard-daemon` only needs to run because `npm run test:browser` or the
  manual repro steps need a current daemon binary, not because daemon source
  changed.
- Manual repro (optional, only if the automated harness needs cross-checking):
  ticket's own `## Reproduction` section, steps 1-7, including Control A and
  Control B.

## Escalations

- None for the survey itself — the ticket already supplies the decision rule,
  the rejected-alternative list, and a concrete F1 code direction; nothing
  here requires a deeper strategy/contract read before implementation starts.
- **Conditional escalation embedded in the Implementation Plan** (not a
  block on starting execution): if the discriminator evidence in step 2
  matches none of decision-rule cases 1-3 and only a press/release dispatch
  (F4) would close the gap, the implementer must stop before writing F4 and
  escalate to the lead — the ticket's D-rule explicitly withholds F4
  selection authority from an implementer, and the owner is away with no
  recorded decision for it.
- Watch for one open question the survey could not resolve without running
  the harness: whether the badged configuration's measured shift is closer to
  the ticket's ~11px estimate or attributable to something else (e.g. font
  metrics changing the icon width) — if the measured shift in step 2 is
  present but does not match the badge-width-plus-gap explanation, note that
  in the Result even if F1 is still selected, since it would mean the geometry
  story is incomplete rather than fully explained.
