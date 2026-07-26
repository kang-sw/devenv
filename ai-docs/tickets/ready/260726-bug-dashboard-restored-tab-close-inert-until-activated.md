---
title: Dashboard terminal tab close button is inert on a reload-restored tab until the tab is clicked once
related:
  260725-feat-dashboard-pty-agent-attention-notification: Phase 6 introduced the stable-forwarder pattern this fix reuses; its Phase 7 e2e step is where the symptom was captured
  260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky: earlier close-confirmation flakiness on the same tab close path
  260725-bug-dashboard-fitnow-short-viewport-shrink: owns the known pre-existing browser-gate failure this ticket's run must not be blamed for
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
  - 260516-ws-web-dashboard-terminal-close-termination
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
sage-review-design: completed
sage-review-completeness: completed
---

# Dashboard terminal tab close button is inert on a reload-restored tab until the tab is clicked once

## Background

A terminal tab that was restored by a page reload and never clicked cannot be
closed: the `×` click produces no confirmation popover and no close, and the
pane stays running. Clicking the tab body once first makes the identical close
work.

The observation is recorded in-tree, at the site that had to work around it:
`ws-dashboard/frontend/e2e/agent-attention-indicator.spec.ts:986-991` ("Against
a tab that was restored by the reload and never clicked, the `×` click produced
no confirmation popover and no close at all - the pane stayed running"). That
test now closes from an already-clicked tab, so the defect is currently
unasserted anywhere.

This is a user-visible regression against already-specified behavior:
`260516-ws-web-dashboard-workroot-workbench-substrate` states that tabs expose
hover-only close buttons and that live terminal closes use a cursor-near
`Yes`/`No` confirmation popover, and
`260516-ws-web-dashboard-terminal-close-termination` states that closing a
terminal panel terminates its session after that inline confirmation. Neither
spec conditions any of it on the tab having been activated first.

## Verified Close Path

Read on 2026-07-26; every line below was confirmed in source, not inferred.

- The `×` is a React `<button data-command-id="workbench.tab.close">` inside the
  Dockview tab renderer. Its `onClick` calls `event.preventDefault()`,
  `event.stopPropagation()`, then `params.onRequestClosePane?.({...})` —
  `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx:448-467`.
- `onRequestClosePane` reaches the tab **through Dockview panel params**, not
  through a stable forwarder: `toDockviewWorkbenchPanelParams` embeds the raw
  callback (`dockviewLayout.tsx:597-619`), and `syncPanels` reads
  `callbacksRef.current.onRequestClosePane` only at sync time
  (`dockviewLayout.tsx:202`). Contrast `onAcknowledgePane`, which is wrapped in a
  stable `useCallback` that dereferences `callbacksRef.current` at call time
  (`dockviewLayout.tsx:160-170`) precisely so a frozen params snapshot cannot
  freeze a stale closure.
- Panel params are refreshed only when
  `shouldUpdateDockviewWorkbenchPanelParams` returns true
  (`dockviewLayout.tsx:551-556`). For `surfaceKind === "persistentTerminal"` that
  predicate returns true only on a change to
  `groupId`/`groupLabel`/`paneId`/`category`/`surfaceKind`/`title`/`detail` or to
  `attentionState`, and otherwise returns
  `socketStatus !== "connecting" && socketStatus !== "connected"` —
  `ws-dashboard/frontend/src/workbench/dockviewLayoutModel.ts:42-74`.
- `meta[1]` is the pane's live socket status and `detail` is the immutable
  terminal id — `ws-dashboard/frontend/src/workbench/terminalWorkbenchPane.tsx:98-113`.
  So **a connected terminal tab's params, and with them its close closure, are
  frozen** until its attention state changes.
- App-side handler: `requestWorkbenchPaneClose`
  (`ws-dashboard/frontend/src/App.tsx:6606-6624`). It has exactly one silent
  early return —
  `const requestWorkRootId = workbenchModel?.root.id ?? selectedWorkRootId;`
  followed by `if (!requestWorkRootId) { return; }` (`App.tsx:6612-6615`). Both
  operands derive from `selection` (`App.tsx:4273`, `App.tsx:4990`), so that
  return is reachable only when `selection` is null. Every other path calls
  `setPendingCloseRequest(...)`, and the popover has no auto-dismiss
  (`App.tsx:6968-6981`, `App.tsx:6985-7027`) — so "no popover at all" means the
  handler either early-returned or never ran.
- The `×` is not CSS-blocked on an inactive tab: `.workbench-tab-close` is
  `opacity: 0` with a `:hover`/`:focus-within` reveal and no `pointer-events`
  or `visibility` suppression — `ws-dashboard/frontend/src/styles.css:2079-2099`.
  Hover-then-click (what the e2e helper does at
  `e2e/agent-attention-indicator.spec.ts:339-352`) is a legitimate click.

## Corrections to the Captured Hypothesis

The `idea/`-stage hypothesis ("a stale closure or a params snapshot that is only
refreshed on activation") is half right and half wrong. Corrected:

1. **Falsified: activation is not a params-refresh trigger.** Nothing in the
   sync path keys a params refresh off active state. `onDidActivePanelChange`
   only calls `onSelectPane` (`dockviewLayout.tsx:267-282`); the active-pane
   branch of the sync only calls `existingPanel.api.setActive()`
   (`dockviewLayout.tsx:581-586`). The refresh gate is
   `shouldUpdateDockviewWorkbenchPanelParams` alone.
2. **Corrected explanation for the "click the tab body first" control.** In the
   run where this was observed, the tab carried a pending attention badge, and
   the tab-body click acknowledged it (`dockviewLayout.tsx:424-426` →
   `acknowledgePaneAttention`, `App.tsx:6736-6747`), flipping `attentionState`
   from `"ready"` to `undefined` — which **is** a refresh trigger
   (`dockviewLayoutModel.ts:69-71`). So the control may prove "the params got
   refreshed", not "the tab got activated". Phase 1 must re-run the control on a
   tab with **no** pending attention before treating tab-activation as the
   relevant variable.
3. **New verified fact, contradicting an in-source comment.** The comment at
   `dockviewLayout.tsx:422-423` says "The close button stops propagation, so
   closing a tab never routes through here." That is true only for the React
   `click`. Dockview binds a **native `pointerdown`** listener on `.dv-tab`
   (`node_modules/dockview-core/dist/esm/dockview/components/tab/tab.js:145-147`)
   and, for a non-active tab in a non-edge group, synchronously calls
   `this.group.model.openPanel(panel)` from it
   (`node_modules/dockview-core/dist/esm/dockview/components/titlebar/tabs.js:487-520`).
   React's `stopPropagation()` on the later `click` cannot prevent that. So
   **clicking the `×` on a never-activated tab activates the panel anyway**,
   which is exactly the window in which a lost `click` could occur.
4. **Reachability of the silent early return is doubtful.** `openWorkRootKeys`
   starts empty on every load (`App.tsx:646`) and is not restored from storage,
   and the render-time union resolves the selected entry from `selection`
   directly (`App.tsx:5028-5040`), so the first render that mounts any
   `DockviewWorkbenchLayout` already has a non-null `selection`. A frozen
   closure captured on such a render would still produce a popover (possibly
   with a wrong `workRootId`), not silence. This weakens — but does not by
   itself kill — the stale-closure story.
5. **Falsified: no DOM teardown happens on activation.** An earlier draft of this
   ticket blamed `Tab.setContent`'s `removeChild`/`appendChild`
   (`.../dockview/components/tab/tab.js:159-163`). That pair runs only when a
   `Tab` is constructed: `TabsContainer.openPanel` returns early when
   `this._tabMap.has(panel.id)` and only past that guard does it `new Tab(...)`
   and `tab.setContent(panel.view.tab)`
   (`.../dockview/components/titlebar/tabs.js:410-416`). Activating an
   already-mounted tab never reaches it. Dockview-react's `ReactPart.update()`
   likewise re-renders through the same portal rather than replacing the host
   element (`node_modules/dockview/dist/esm/react.js:55-66`). Any "the tab
   element was torn down mid-gesture" story is out; a lost `click` here must come
   from the button *moving*, not from the button being replaced.
6. **New verified fact: pressing `×` on a badged, never-activated tab clears the
   badge and reflows the tab.** Dockview's native `pointerdown` activation
   (correction 3) fires `onDidActivePanelChange` → `onSelectPane`
   (`dockviewLayout.tsx:267-282`) → `selectPane`, which calls
   `acknowledgePaneAttention` for `persistentTerminal` panes
   (`App.tsx:6749-6758`, `App.tsx:6736-6747`) — so the `×` acknowledges
   attention even though its React `onClick` stops propagation. Clearing
   `attentionState` **is** a params-refresh trigger
   (`dockviewLayoutModel.ts:69-71`), and the refreshed render drops the
   conditionally-rendered `.workbench-tab-attention` span
   (`dockviewLayout.tsx:441-446`), removing a 7px dot
   (`styles.css:2662-2668`) plus the tab's 4px flex `gap`
   (`styles.css:1660-1670`) from a content-width tab
   (`styles.css:1648-1653` sets `padding: 0`, no width) whose `×` is 14px wide
   (`styles.css:2079-2094`). React discrete-event updates flush before
   `mouseup`, so the button can shift ~11px left under a stationary cursor
   between `mousedown` and `mouseup`. The `click` then fires on the nearest
   common ancestor — `.dv-tab` — whose React `onClick` only acknowledges
   (`dockviewLayout.tsx:424-426`). Silence, pane still running.

**Leading hypothesis after the read (to be confirmed, not assumed):** the React
`onClick` at `dockviewLayout.tsx:454` never runs, because dockview's native
`pointerdown` activation (correction 3) moves the `×` out from under the
stationary cursor before `mouseup`, so the `click` is delivered to an ancestor
instead of the button. Correction 6 is the concrete, fully source-traced chain
for that — and it is the one that also explains the "click the tab body first"
control without appeal to activation, since the tab-body click is what removes
the badge. **Second candidate chain for the same lost-`click` family:**
`TabsContainer.setActivePanel` scrolls the tab strip to reveal the newly active
tab (`.../dockview/components/titlebar/tabs.js:377-400`), which displaces the
tab horizontally whenever the strip overflows. **Retained alternative:** the
frozen-params stale closure of correction 4. Phase 1 discriminates before fixing.

## Decisions

- **D1 (resolved under goal-run posture, owner away).** Whichever mechanism the
  discriminator proves, `onRequestClosePane` is additionally routed through the
  same stable `callbacksRef` forwarder `onAcknowledgePane` already uses
  (`dockviewLayout.tsx:160-170`), because a raw callback frozen into a
  never-refreshed params snapshot is a latent hazard on this surface either way.
- **D2.** The fix must not depend on the tab carrying a pending attention badge,
  and must not require the tab to be Dockview-active. Both are the defect.
- **D3.** Frontend-only. No daemon, route, or protocol change; the close request
  already reaches the daemon correctly once confirmed.
- **Rejected: widen `shouldUpdateDockviewWorkbenchPanelParams` to always refresh
  connected-terminal params.** That gate exists to stop parameter churn from
  blurring/remounting a streaming xterm between keystrokes
  (`dockviewLayoutModel.ts:53-55`) and is required by the substrate spec's
  "pane parameter updates must be keyed by stable content revisions instead of
  React node identity". Refreshing to fix a callback would re-open that.
- **Rejected: force `setActive()` on every restored pane so no tab is ever
  un-activated.** It overrides the persisted active-pane restore
  (`App.tsx:4326-4333`) and changes user-visible focus to fix a close button.
- **Rejected: move close handling to a document-level delegated listener outside
  the Dockview adapter.** The adapter contract at `dockviewLayout.tsx:234-236`
  keeps dashboard policy outside Dockview and forbids raw Dockview handles from
  escaping; a global listener would have to re-derive pane identity from the DOM.
- **Rejected: `preventDefault()` on the button's `pointerdown` to suppress
  dockview's activation.** Dockview reads `event.defaultPrevented` at
  `.../titlebar/tabs.js:487-489` and would skip activation for the whole tab —
  a silent change to tab-activation semantics for every surface kind to work
  around one broken affordance. (F3 below is the scoped variant of the same
  idea and is *not* covered by this rejection; it is a live candidate.)

## Candidate Fixes for the Lost-`click` Hypothesis

If the discriminator lands on the frozen-closure alternative (correction 4), the
fix is D1's stable forwarder alone and this section does not apply. If it lands
on the lost-`click` family, these are the candidates; they are not
interchangeable, and Phase 1 must land the one the decision rule selects rather
than "the mechanism-specific fix" left unnamed.

- **F1 — take the attention badge out of the tab's layout flow.** Make the badge
  occupy no inline space (`position: absolute` on `.workbench-tab-attention`
  against a `position: relative` `.dockview-workbench-tab`), so acknowledging
  changes no geometry and the `×` cannot move. Smallest change; touches no
  interaction semantics; keeps the badge conditionally rendered, so the existing
  `toHaveCount(0)` assertion on `[data-workbench-tab-attention]`
  (`e2e/agent-attention-indicator.spec.ts:583-587`) stays valid. Cost: a visual
  reposition of the dot that the frontend-design pass must accept.
  - **F1' — always render the badge span** with
    `data-workbench-tab-attention="none"` and hide it visually, mirroring the
    always-present `data-attention-state` contract
    (`dockviewLayout.tsx:395-410`). Also geometry-stable, and more symmetric with
    the existing contract, but it invalidates the `toHaveCount(0)` assertion
    above and therefore edits a green attention-indicator gate. Prefer F1 unless
    the design pass wants the reserved slot visible.
  - **Scope limit (F1 and F1' alike).** They fix correction 6 only. They do
    **not** fix the tab-strip scroll chain, and do nothing for a no-attention
    reproduction.
- **F2 — keep the `×` in place regardless of what reflows.** Give the close
  button a reserved, activation-independent position within the tab (fixed
  trailing slot whose offset does not depend on sibling presence). Broader than
  F1: it neutralizes correction 6 *and* any future sibling-driven reflow, but not
  the tab-strip scroll chain. Cost: a real tab-layout change, so it needs the
  frontend-design pass and a look at every surface kind's tab content.
- **F3 — suppress dockview's activation only for gestures that start on the
  `×`.** Attach a **native** `pointerdown` listener to the button through a ref
  and `stopPropagation()` it, so dockview's `.dv-tab` listener
  (`.../tab/tab.js:145-147`) never fires for that gesture. Immune to every
  reflow chain including tab-strip scroll. Costs, all of which must be stated in
  the Result if this lands: (a) clicking `×` on an inactive tab no longer
  activates or acknowledges it, for every surface kind — a real, if defensible,
  behavior change; (b) it must be a native listener, not React's
  `onPointerDown`: React attaches at the root container, so a synthetic
  `stopPropagation()` runs *after* dockview's ancestor listener has already
  activated the panel — the exact trap the stale comment at
  `dockviewLayout.tsx:422-423` fell into for `click`.
- **F4 — fire the close request from `onPointerDown`/`onPointerUp` instead of
  `onClick`.** Immune to reflow because there is no click-target reconciliation
  left to lose. **Highest-risk candidate:** it changes close from a
  press-and-release affordance to a press (or release) affordance for every
  surface kind, removes the browser-native "press, drag off, release to cancel"
  escape, and interacts with the tab's drag handler — the same class of silent
  cross-surface semantics change this ticket rejects `preventDefault` for. Do not
  land it as an implementation detail.

**Decision rule (applies once the discriminators land).**

1. If only the with-attention configuration reproduces and the geometry
   measurement shows the `×` moving: land **F1** (F1' only if the design pass
   asks for it). This is the expected outcome.
2. If a no-attention, never-activated tab also reproduces and the geometry
   measurement shows the `×` moving: the reflow is not badge-driven — land
   **F3**, and record cost (a) above as a deliberate behavior change.
3. If the `×` does not move but the `click` still never reaches the button:
   land **F3**.
4. **F2** is a substitute for F1 only when the design pass rejects F1's dot
   placement. **F4** requires an explicit owner decision recorded in this ticket
   before it is written; an implementer may not select it from the rule.
5. D1's stable-forwarder hardening lands in every branch, including the
   no-reproduction branch in Phase 1.

## Constraints

- Keep the adapter boundary: Dockview events are reduced to dashboard
  pane/group ids before product callbacks run (`dockviewLayout.tsx:234-236`).
- Do not rename or drop the tab data attributes browser acceptance selects on
  (`data-command-id="workbench.tab.close"`,
  `data-workbench-close-popover="cursor-near"`,
  `data-workbench-tab-close-affordance`, `data-workbench-pane-id`,
  `data-attention-state`).
- Preserve the deliberate naming exception recorded at
  `dockviewLayout.tsx:401-409`; do not "tidy" it as part of this fix.

## Reproduction

A fresh session must reproduce before fixing.

Manual (fastest path to seeing it):

1. `cd ws-dashboard/frontend && npm run build`
2. `cd ws-dashboard && cargo build -p ws-dashboard-daemon`
3. `./target/debug/ws-dashboard serve --static-dir frontend/dist`, then open the
   one-time owner pairing URL it prints.
4. Open a work root. Create the terminal **without the browser ever activating
   its tab** — either `POST /api/dashboard/work-roots/{workRootId}/terminals`
   from outside the browser, or create it from the toolbar and then click the
   pinned agent tab so the terminal tab is no longer the group's active tab.
   (The pinned `main-agent` pane is spread first into group 1 —
   `ws-dashboard/frontend/src/workbench/editorGroups.ts:159-183` — and the sync
   falls back to `group.panes[0]?.id` when no active pane is restored,
   `dockviewLayout.tsx:502`, so a never-activated terminal tab is normally the
   inactive one.)
5. Put the tab in the configuration the defect was actually observed in: post a
   non-idle turn state for that terminal so the tab carries a pending attention
   badge (`data-attention-state="ready"`). The only recorded observation was on
   a badged tab, and under correction 6 the badge is load-bearing.
6. Reload the page and re-select the work root. Wait for the terminal tab to
   appear. **Do not click the tab body.**
7. Hover the tab, click `×`.

- Expected: the cursor-near `Close session?` popover with `Yes`/`No`.
- Actual: nothing happens; the pane keeps running.
- Control A (known to work): click the tab body once, then hover + `×`.
- Control B (the discriminator, see Phase 1): run the same steps with **no**
  pending attention badge on the tab at any point (skip step 5).

Automated shape: the flow above is exactly what
`e2e/agent-attention-indicator.spec.ts` already builds (spawn into a named root
through the daemon route, `page.reload()`, `selectWorkRootMinimal`), so the
assertion belongs in that file or a sibling spec reusing the same harness.

## Phases

### Phase 1: Make a never-activated, reload-restored terminal tab closable on its first `×` click

**Discriminate first (bounded, inside this phase, not a separate deliverable).**
Reproduce per the section above, then run the three cheap discriminators:

- **Attention-independence.** Reproduce twice: once with a pending attention
  badge on the tab (the configuration the defect was actually seen in), once
  with no turn-state ever posted so `attentionState` is `undefined` throughout.
  Record the primary symptom for each, not only whether the tab-body click fixes
  it. If only the badged run reproduces, correction 6 is the live mechanism. If
  the unbadged run also reproduces, correction 6 is insufficient and the reflow
  is coming from somewhere else (tab-strip scroll, or the frozen-params story).
- **Second-click.** Without ever clicking the tab body, click `×` twice. If the
  second `×` opens the popover, the first click was consumed by dockview's
  `pointerdown` activation (leading hypothesis) rather than reaching a stale
  handler.
- **Geometry.** Read the `×` button's bounding box immediately before
  `pointerdown` and again after the tab has activated. A horizontal shift of
  roughly the badge width plus the tab's flex gap confirms the lost-`click`
  family and selects among the candidate fixes; no shift at all rules F1/F2 out.

Record which mechanism the evidence supports in the phase Result. Do not fix
past the evidence.

**Then fix.** Apply the candidate fix that the decision rule in the
`## Candidate Fixes` section selects — by name, with the
selecting evidence quoted in the Result — plus D1's stable-forwarder hardening
for `onRequestClosePane` regardless of outcome. Landing a candidate the rule
does not select (in particular F4) requires an owner decision recorded in this
ticket first.

**If no configuration reproduces the symptom.** This is a live possibility:
correction 4 already argues the frozen-closure story predicts a wrong-`workRootId`
popover rather than silence, and correction 5 removes the DOM-teardown chain.
Do not stall and do not speculatively land a mechanism fix. Land D1's
stable-forwarder hardening plus the browser assertions below as a regression
guard, record the no-repro explicitly in the Result (configurations tried,
browser, what was observed instead), and close the phase on that. The spec edits
below still land: they state activation- and refresh-independence as an
invariant, which is true and asserted either way.

**Completed behavior for this phase.** All of the following, together:

1. On a reload-restored terminal tab that has never been clicked **and carries a
   pending attention badge** — the exact configuration of the only recorded
   observation (`e2e/agent-attention-indicator.spec.ts:986-991`) — a single
   hover + `×` click opens the cursor-near `Yes`/`No` popover, and confirming
   terminates the session and removes the tab.
2. The same holds on a reload-restored, never-clicked tab that carries **no**
   attention badge (D2's other half).
3. The two spec additions in `## Spec Impact` are written. The phase is not
   complete with the fix alone: the acceptance-gate spec currently makes a
   close-path coverage claim that the Background flags as overstated, and
   leaving it standing while closing this ticket re-publishes that claim as
   verified.

**Deferred scope (explicitly out).**

- Other surface kinds (`agent`, `agentChat`, `editor`, `workRootActivity`) are
  not separately re-verified here. The frozen-params gate is
  `persistentTerminal`-specific (`dockviewLayoutModel.ts:56-74`), and the
  reversible kinds close immediately with no popover, so the observed symptom
  cannot present the same way. Correction 6's chain is also terminal-only: the
  attention badge is rendered from `attentionState`, which only terminal panes
  carry (`App.tsx:6736-6747` resolves the pane out of `terminalPanes`). If the
  discriminator instead lands on a surface-kind-independent chain, or the fix
  is F3/F4 (which change behavior for every tab), note that in the Result and
  raise a follow-up ticket rather than widening this phase.
- Keyboard-driven close, command-palette close, and the drag/split paths.
- Any change to `shouldUpdateDockviewWorkbenchPanelParams`'s churn policy — see
  the rejected alternative above.
- The stale `editorGroups`/`terminalPanes` capture inside
  `performWorkbenchPaneClose` (`App.tsx:6543-6599`). If the discriminator shows
  the frozen-closure mechanism, this is the same root cause and is in scope; if
  it does not, leave it and file it separately rather than speculatively
  refactoring App state plumbing.

**Verification boundary.**

- **Binding: Playwright browser acceptance.** This ticket renders visible UI, so
  per `260516-ws-web-dashboard-browser-ui-acceptance-gate` the fix is not done
  until it is asserted in a browser test against the daemon-served production
  frontend. The assertion must be the *unclicked-tab* close: reload, select the
  root, do not click the tab body, hover, click `×` once, assert the popover is
  visible, confirm, assert the tab is gone. An assertion that clicks the tab
  first re-creates the workaround at
  `e2e/agent-attention-indicator.spec.ts:986-991` and proves nothing.
- **Binding: the badged configuration is mandatory, and it is the primary.** The
  assertion above must run at least once on a tab that **carries a pending
  attention badge at the moment `×` is pressed** — post a non-idle turn state
  before the reload, do not click the tab, then hover + `×`. This is the only
  configuration the defect was ever observed in, and under correction 6 it is the
  only one that can reproduce it. An assertion written solely against a
  no-attention tab would, if correction 6 is right, pass against unfixed code and
  close this phase green with the defect intact. The no-attention variant is
  still asserted (completed behavior 2), as the guard for D2's other half — but
  it is the secondary, not the binding one.
- **Do not pin the badge's post-click state in that assertion.** Today the `×`
  press acknowledges attention as a side effect of dockview's native activation
  (correction 6), so `data-attention-state` flips to `"none"`; under candidate
  fix F3 the press would no longer activate and the badge would survive until the
  tab is closed. Both are consistent with the acknowledgement contract recorded
  at `dockviewLayout.tsx:413-426` and asserted at
  `e2e/agent-attention-indicator.spec.ts:575-587`, which is about **tab-body**
  clicks — the `×` is not a tab-body click and no spec or test states what it
  does to the badge. Assert the popover and the close, not the badge. If F3
  lands, record the acknowledgement delta in the Result and check whether the
  attention spec sentence needs it; F1/F1'/F2 leave today's behavior unchanged.
- **Build trap (must be obeyed).** `playwright.config.ts` declares no
  `webServer` and no `globalSetup`, and `e2e/daemonHarness.ts:216` points the
  spawned daemon at the prebuilt `frontend/dist`. Only the `test:browser` npm
  script chains `npm run build` (`ws-dashboard/frontend/package.json`). Any
  `frontend/src` change used as evidence must therefore be followed by
  `npm run build` **before** the browser run, or `playwright test` silently
  exercises a stale bundle and "passes" against the unfixed code.
- **Known pre-existing failure — judge by failure SITE, not exit code.**
  `e2e/dashboard-acceptance.spec.ts:3779`
  (`expect(shortRows).toBe(terminalClearFixTallRows)`) already fails on this
  branch and is owned by `260725-bug-dashboard-fitnow-short-viewport-shrink`
  (currently `todo/`). A non-zero browser-gate exit code is acceptable evidence
  only if that is the sole failing site.
- Route/helper tests are not sufficient on their own; if the fix lands in
  `dockviewLayoutModel.ts`, add or extend a `test:workbench` case for the
  predicate change as well, but the browser gate remains the closing evidence.

## Spec Impact

Every caller-visible behavior in Phase 1 is already covered by the `spec:` stems
above — the fix restores specified behavior rather than introducing new
behavior:

- `260516-ws-web-dashboard-workroot-workbench-substrate` — "Tabs expose
  hover-only close buttons. Live terminal or agent closes use a cursor-near
  `Yes`/`No` confirmation popover", with no activation precondition.
- `260516-ws-web-dashboard-terminal-close-termination` — close terminates the
  session after inline `Yes`/`No` confirmation near the close action.
- `260516-ws-web-dashboard-browser-ui-acceptance-gate` — the workbench tab
  polish evidence paragraph already claims hover-only close affordances and
  terminal close-confirmation popover cancel/confirm coverage at browser level;
  that claim is currently overstated for the never-activated-tab case.

Two small additions land on those existing stems once the discriminator resolves
(contract-first spec: no — write them from the implemented behavior). **They are
owned by Phase 1**, listed in its completed behavior item 3; this ticket is not
complete while the acceptance-gate sentence still claims coverage the fix has
not actually produced:

1. On `260516-ws-web-dashboard-workroot-workbench-substrate`: an explicit
   invariant that a tab's lifecycle affordances (close, acknowledge) stay live
   regardless of when that panel's Dockview parameters were last refreshed and
   regardless of whether the tab has been activated — the natural companion to
   the existing "pane parameter updates must be keyed by stable content
   revisions" sentence.
2. On `260516-ws-web-dashboard-browser-ui-acceptance-gate`: extend the workbench
   tab polish evidence sentence so the covered close paths explicitly include a
   reload-restored, never-activated tab.

If the discriminator lands on the lost-`click` mechanism (the leading
hypothesis), addition 1 is reworded to state the activation-independence of tab
affordances without referencing parameter refresh — and, if correction 6 is
confirmed, to state that a tab's attention indicator must not change the tab's
close-affordance geometry when it appears or clears. Addition 2 is unchanged in
every branch, including the no-reproduction branch: the extended close-path
coverage wording is what the new assertions actually establish.
