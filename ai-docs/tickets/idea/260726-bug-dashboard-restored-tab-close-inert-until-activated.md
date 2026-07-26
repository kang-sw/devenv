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

**Leading hypothesis after the read (to be confirmed, not assumed):** the React
`onClick` at `dockviewLayout.tsx:454` never runs, because dockview's native
`pointerdown` activation (correction 3) causes the tab's rendered content to be
torn down and re-created between `mousedown` and `mouseup` — Dockview's own
`Tab.setContent` does `removeChild`/`appendChild` on the tab's content element
(`.../dockview/components/tab/tab.js:159-163`) — so no `click` event is
delivered to the button. **Retained alternative:** the frozen-params stale
closure of correction 4. Phase 1 discriminates before fixing.

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
  dockview's activation.** It would silently change tab-activation semantics for
  every surface kind to work around one broken affordance.

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
5. Reload the page and re-select the work root. Wait for the terminal tab to
   appear. **Do not click the tab body.**
6. Hover the tab, click `×`.

- Expected: the cursor-near `Close session?` popover with `Yes`/`No`.
- Actual: nothing happens; the pane keeps running.
- Control A (known to work): click the tab body once, then hover + `×`.
- Control B (the discriminator, see Phase 1): run the same steps with **no**
  pending attention badge on the tab at any point.

Automated shape: the flow above is exactly what
`e2e/agent-attention-indicator.spec.ts` already builds (spawn into a named root
through the daemon route, `page.reload()`, `selectWorkRootMinimal`), so the
assertion belongs in that file or a sibling spec reusing the same harness.

## Phases

### Phase 1: Make a never-activated, reload-restored terminal tab closable on its first `×` click

**Discriminate first (bounded, inside this phase, not a separate deliverable).**
Reproduce per the section above, then run the two cheap discriminators:

- **Attention-independence.** Reproduce with no turn-state ever posted, so
  `attentionState` is `undefined` throughout. If the tab-body click still fixes
  the close, the attention-refresh explanation (correction 2) is out and
  something about activation is genuinely load-bearing. If the tab-body click no
  longer fixes it, the frozen-params story is the live one.
- **Second-click.** Without ever clicking the tab body, click `×` twice. If the
  second `×` opens the popover, the first click was consumed by dockview's
  `pointerdown` activation (leading hypothesis) rather than reaching a stale
  handler.

Record which mechanism the evidence supports in the phase Result. Do not fix
past the evidence.

**Then fix.** Apply the mechanism-specific fix, plus D1's stable-forwarder
hardening for `onRequestClosePane` regardless of outcome.

**Completed behavior for this phase.** On a reload-restored terminal tab that
has never been clicked and carries no attention badge, a single hover + `×`
click opens the cursor-near `Yes`/`No` popover, and confirming terminates the
session and removes the tab — identical to the already-clicked-tab path.

**Deferred scope (explicitly out).**

- Other surface kinds (`agent`, `agentChat`, `editor`, `workRootActivity`) are
  not separately re-verified here. The frozen-params gate is
  `persistentTerminal`-specific (`dockviewLayoutModel.ts:56-74`), and the
  reversible kinds close immediately with no popover, so the observed symptom
  cannot present the same way. If the discriminator lands on the lost-`click`
  mechanism (which is surface-kind independent), note that in the Result and
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

Two small additions are expected on those existing stems once the discriminator
resolves (contract-first spec: no — write them from the implemented behavior):

1. On `260516-ws-web-dashboard-workroot-workbench-substrate`: an explicit
   invariant that a tab's lifecycle affordances (close, acknowledge) stay live
   regardless of when that panel's Dockview parameters were last refreshed and
   regardless of whether the tab has been activated — the natural companion to
   the existing "pane parameter updates must be keyed by stable content
   revisions" sentence.
2. On `260516-ws-web-dashboard-browser-ui-acceptance-gate`: extend the workbench
   tab polish evidence sentence so the covered close paths explicitly include a
   reload-restored, never-activated tab.

If the discriminator instead lands on the lost-`click` mechanism, addition 1 is
reworded to state the activation-independence of tab affordances without
referencing parameter refresh; addition 2 is unchanged either way.
