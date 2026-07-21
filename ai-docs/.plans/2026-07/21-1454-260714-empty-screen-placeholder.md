# Plan: 260714-bug-dashboard-workroot-close-button-hidden-when-selected — Phase 1: empty main-screen placeholder so closing the last/selected work root has a defined UX

## Relevant Ticket Contract

- Decided Direction (2026-07-21): the close-button-hidden symptom exposed a
  deeper gap — the "empty main screen" UX is undefined for when the active
  work root is closed/deselected. This round's deliverable is to add an
  "empty screen" placeholder for the main content area when no work root is
  active, then (Phase 2, separate) wire close/deselect into it.
- Phase 1 text: "Define and render a coherent empty-main-screen placeholder
  for the main content area, shown whenever there is no active work root
  (including but not limited to the close/deselect interaction added in
  Phase 2). Success: the main content area has a defined, coherent 'nothing
  active' presentation instead of a blank/undefined state."
- Verification note attached to Phase 2 (not Phase 1) mentions a
  resource-model/render-level test for the X button and a check that
  deselect lands on the empty placeholder — that is Phase 2's verification
  boundary, not this phase's, since this phase does not touch
  `canCloseWorkRoot` or `selectedId`/`resolveWorkbenchSelection` wiring.

## Out of Scope

- Phase 2 in full: changing `canCloseWorkRoot` to drop `!selected`
  (`App.tsx:9793-9798`), wiring the selected-row X to deselect, and any
  `resolveWorkbenchSelection`/sticky-selection fallback change so a deselect
  actually reaches the empty state instead of falling back to another root.
  This phase only defines/renders the placeholder that Phase 2 will later
  land on.
- `workRoot.close`'s existing "unmount workbench, keep daemon terminal
  session alive for reattach" semantics for the non-selected case — untouched
  by this phase.
- The `260714-feat-dashboard-multi-server-workbench-keepalive` per-server
  On/Off lifecycle — explicitly a different ticket per that ticket's
  Non-Goals.
- Any new "open a work root" flow, recent-workspaces list, or other
  navigation shortcut inside the placeholder beyond what Phase 1's text
  actually specifies (see Open Design Questions below) — not to be invented
  by the executor without a decision.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:6151-6160` — **exact current
  render branch for "no active work root."** Inside `WorkbenchShell`'s
  `activeHeader` derivation:
  ```tsx
  const activeHeader =
    loading && !resources ? (
      <StatusPane title="Loading" detail="workbench resources" />
    ) : error && !resources ? (
      <StatusPane title="Workbench unavailable" detail={error} />
    ) : !resources || !workbenchModel ? (
      <StatusPane
        title="No workRoot"
        detail="select a workRoot or main instance"
      />
    ) : ( /* real workbench toolbar + notices */ )
  ```
  This is the change point: the `!resources || !workbenchModel` branch is
  today's entire "no selection" UX for the main pane.
- `ws-dashboard/frontend/src/App.tsx:4522-4544` — `workbenchModel` is
  `resources && selection ? {...} : null`; `selection` is the
  `WorkbenchSelection | null` prop (`workbenchSelection` in `App()`,
  resolved through the sticky-selection/`deriveWorkbenchView` chain in
  `workbench/openRootLookup.ts`). Confirms `workbenchModel` is null exactly
  when nothing is selected — the placeholder's trigger condition is already
  correctly identified by the existing `!resources || !workbenchModel` test;
  no new "is anything selected" predicate needs to be invented.
- `ws-dashboard/frontend/src/App.tsx:6207-6262` — `WorkbenchShell` returns
  `<div className="workbench-shell">{activeHeader}{openWorkRootInstances.map(...)}{...}</div>`.
  When there is truly nothing open, `openWorkRootInstances` is empty and
  `activeHeader`'s small `StatusPane` is the *only* content in the panel —
  everything below it is just the panel's flat background color. This is
  the literal "blank main screen" the ticket describes: not devoid of markup,
  but a small top-left notice box floating over an otherwise empty region,
  not a designed empty-state.
- `ws-dashboard/frontend/src/App.tsx:10106-10122` — `StatusPane` component:
  ```tsx
  function StatusPane({ title, detail, action }: { title: string; detail: string; action?: ReactNode }) {
    return (
      <div className="status-pane ws-state-surface">
        <div className="status-title">{title}</div>
        <div className="status-detail">{detail}</div>
        {action ? <div className="status-action">{action}</div> : null}
      </div>
    );
  }
  ```
  Already supports an optional `action` slot (`ReactNode`) — reusable if the
  eventual placeholder needs a CTA button, but no caller passes `action`
  today.
- `ws-dashboard/frontend/src/styles.css:263-268,1386-1406` — `.status-pane`
  is `padding: 16px` only; `.ws-state-surface` gives it a left border rail
  and subtle panel background, i.e. it is styled like an inline
  loading/error notice, not a centered/full-height empty-state layout.
  `.workbench-shell` (`styles.css:1463-1469`) is `display:flex; height:100%;
  flex-direction:column` — the panel itself fills its parent, but the
  `StatusPane` inside does not attempt to center or fill it.
- `ws-dashboard/frontend/src/App.tsx:1487-1576` — outer layout for context:
  `<main className="app-shell">` → `<div className="shell-grid
  shell-grid-workbench">` → nav `<aside>` (`ResourceNavigation`) +
  `<section className="shell-panel shell-panel-workbench">` containing
  `<WorkbenchShell>`. The placeholder only needs to live inside
  `WorkbenchShell`'s return; no other layout container needs touching.
- `ws-dashboard/frontend/src/App.tsx:9793-9798` — **close-button
  reachability issue** (context for Phase 2, confirms ticket's description
  still holds against current code, only line numbers shifted from the
  ticket's `9112-9115`):
  ```tsx
  const canCloseWorkRoot =
    (presentation === "workRoot" ||
      presentation === "compactWorkRoot" ||
      presentation === "workspace") &&
    isOpenWorkRoot &&
    !selected;
  ```
  The trailing `!selected` still hides the X on the row currently selected.
  Confirmed present and unchanged in scope of this survey; **not** modified
  by this phase — left for Phase 2 as the ticket specifies.
- `ws-dashboard/frontend/src/App.tsx:3473` — a second, unrelated "no
  selection" message (`<div className="file-explorer-state">No workRoot
  selected</div>`) exists inside `WorkRootFileExplorer`, a different pane
  (the file explorer sidebar-within-workbench), not the main content area.
  Confirmed out of scope — the ticket's "main content area" refers to the
  `WorkbenchShell` region, not this explorer pane.
- Test/verification reality: `grep` across `ws-dashboard/frontend/src` for
  `StatusPane`/`No workRoot`/`status-pane` returns only `App.tsx` — no
  existing test references this render branch. `package.json` test scripts
  (`test:*`) all compile-and-run plain Node scripts against pure logic
  modules (`resourceModel`, `workbenchModel` pane-group logic,
  `openRootLookup`, etc.); there is no React Testing Library / jsdom /
  vitest in `devDependencies`, so no harness exists today for asserting on
  rendered DOM output of `App.tsx` components. The only real DOM-level
  coverage path is `npm run test:browser` (Playwright, requires building the
  daemon) — heavy, and not previously used for this render branch.

## Open Design Questions

The ticket's Phase 1 text ("coherent empty-main-screen placeholder", "defined,
coherent 'nothing active' presentation instead of a blank/undefined state")
sets the *outcome bar* but does **not** fully specify the placeholder's visual
content. The following are genuine open UX choices a human should confirm
before/while implementing — this plan does not invent answers for them:

1. **Copy/messaging.** Should the placeholder keep wording close to today's
   `title="No workRoot" detail="select a workRoot or main instance"`, or use
   different copy (e.g. a friendlier "Nothing open yet" / welcome-style
   message)? Ticket does not specify exact text.
2. **Call-to-action affordance.** Should the placeholder include an actionable
   button/link (e.g. "Open a work root", opening the root picker /
   `rootPickerOpen` flow already present in the nav) via `StatusPane`'s
   existing `action` slot, or remain purely informational? Ticket does not
   decide this either way.
3. **Layout/prominence.** Should the placeholder fill/center in the full
   `.workbench-shell` panel height (a true empty-state treatment), or stay a
   small top-aligned notice like today's `StatusPane` but with better
   styling? Ticket only requires "coherent... instead of blank" — it does not
   specify vertical centering, iconography, or panel-filling layout.
4. **Iconography/illustration/branding.** Whether to add an icon (e.g. a
   `lucide-react` glyph, already a project dependency) or any illustration/
   logo element is unspecified.
5. **Recent-workspaces or quick-pick list.** Whether the placeholder should
   surface recently-closed or available work roots for one-click reselection
   is not mentioned by the ticket at all; nothing in Phase 1 or Phase 2 text
   asks for this.
6. **Component reuse vs. new component.** Whether to extend the existing
   `StatusPane` (adding the icon/action treatment inline, since it already
   renders in this exact branch) or introduce a dedicated new component
   (e.g. `EmptyWorkbenchPlaceholder`) is an implementation-style choice the
   ticket leaves open.

None of these block writing a plan, but an executor should not silently pick
an answer for #1–#5 (content/affordances) without confirming — they are
genuine content/design decisions, not just implementation style. #6 is a
lower-stakes implementation choice that can reasonably be resolved during
execution and documented in the Result section, consistent with how Phase 2's
own text explicitly defers one such decision ("Decide during implementation
whether... document the choice in the Result").

## Implementation Plan

Given the open design questions above, the concrete content of the
placeholder cannot be fully pre-specified. The structural/mechanical steps
that do not depend on the open questions:

1. Locate the single change point: the `!resources || !workbenchModel`
   branch of `activeHeader` in `WorkbenchShell`
   (`ws-dashboard/frontend/src/App.tsx:6156-6160`). This is the only render
   branch needing new content; no other file needs a new "no selection"
   check invented.
2. Once content/affordances are confirmed (Open Design Questions above),
   either:
   - extend `StatusPane` usage in place (pass a richer `detail`/`action`,
     and/or add new optional props to `StatusPane`
     (`App.tsx:10106-10122`) such as an icon slot), or
   - introduce a new presentational component near `StatusPane` and swap it
     into the `!resources || !workbenchModel` branch only.
   Either approach is a same-file (`App.tsx`) change plus a `styles.css`
   addition for any new layout (e.g. centering rules), following the
   existing `.status-pane`/`.ws-state-surface` class pattern
   (`styles.css:263-268,1386-1406`).
3. If a CTA action is added (Open Question 2) and it needs to invoke the
   root picker, reuse existing command builders already imported in
   `App.tsx` (`buildRootPickerOpenCommand` et al., `App.tsx:90-96`) rather
   than inventing a parallel path — do not build new open-work-root
   plumbing.
4. Do not touch `canCloseWorkRoot` (`App.tsx:9793-9798`) or any
   `selectedId`/selection-resolution code — those are Phase 2's exclusive
   surface.

## Verification Plan

- `npm --prefix ws-dashboard/frontend run build` (`tsc -b && vite build`) —
  type-safety and bundling check; the only automated gate that exercises
  `App.tsx` as a whole today.
- No existing unit test targets this render branch (`activeHeader`'s
  `!resources || !workbenchModel` case) and the project has no
  React-Testing-Library/jsdom harness to add one cheaply — honestly note
  this coverage gap rather than claim a render-level test is trivial. If the
  executor adds a component-level check, it would need a net-new
  jsdom/RTL-style setup, which is a larger investment than this phase's
  scope; prefer manual verification instead unless the team decides that
  investment is worthwhile.
- Manual verification: run the dashboard (`ws-dashboard` dev flow), open a
  server with no work root selected (or observe initial load before any
  selection), and visually confirm the main pane shows the new
  placeholder instead of the current small top-left notice-in-empty-space
  look.
- Re-run `npm --prefix ws-dashboard/frontend run test:workbench` (covers
  `workbenchModel`/`openRootLookup`/`deriveWorkbenchView` pure-logic tests)
  to confirm no regression to `workbenchModel` derivation, since this phase
  must not change when `workbenchModel` is null, only what renders when it
  is.

## Escalations

- None for proceeding with the *mechanical* change point (confirmed single
  branch, single file). However, the executor (or the lead) should resolve
  the Open Design Questions above — a human content/UX decision, not a
  research-agent question — before writing the actual placeholder markup/
  copy, since the ticket intentionally leaves those choices unmade.
