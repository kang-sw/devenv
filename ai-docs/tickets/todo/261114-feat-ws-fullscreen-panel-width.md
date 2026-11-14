---
title: Center shared fullscreen panels with an explicit full-width extension
related:
  261114-feat-ws-whiteboard-compact-floating-toolbar: "prerequisite: implement only after its source lands; shared cards.js/dashboard.css surfaces"
  261114-feat-ws-whiteboard-unified-whole-history: "coordinate entry-panel disclosure, sticky-header and scroll behavior"
  261114-feat-ws-dashboard-shared-card-actions: "existing shared maximize ownership and controls"
---

# Center shared fullscreen panels with an explicit full-width extension

## Background

Card fullscreen toggles currently open nearly viewport-wide sheets. On very
wide displays, the owner wants a centered default width limit and a separate
explicit way to use the available full width. Preserve the existing shared
maximize lifecycle rather than introducing a second kind of fullscreen panel.

Read-only scouting traced the shared implementation to `maximize()` and the
`.bc-max` / `.d-max` / `.d-min` actions in
`agents-plugin-tool/internal/wsdashboard/assets/cards.js`, including the Files
pane's `maximize({files:true})` path. Sheet geometry lives in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`; current wide
insets are 16px and the narrow rules use 8px. Dashboard-framed whiteboard entry
panels expose fullscreen controls; standalone whiteboard pages do not.
The shared-card action implementation landed in `f227e6d9b`.

## Decisions

- **(733)=a — all shared maximize surfaces.** Apply the same width behavior to
  preview, terminal and web cards, board topics, dashboard-framed whiteboard
  entry panels, and the Files pane. Rejected: a card-only subset that leaves
  other shared sheets inconsistent. MAIN relayed the owner's exact answer
  `733,734=a` on 2026-11-14 at 12:34.
- **(734)=a — no remembered extension.** Extended width resets on close,
  minimize and reload. Rejected: session memory or browser-persisted state.
- **(735), announced default — centered cap.** Default visible sheet width is
  `min(1920px, available width)`, centered horizontally. The extension control
  explicitly changes the open sheet to the existing available full-width
  geometry. MAIN included this settled default in the 12:34 capture directive.
- **(736), announced default — narrow behavior unchanged.** Preserve existing
  narrow/mobile layout, including the existing below-640px rules. A viewport
  whose available width is no greater than the cap is unaffected. Show the
  separate extension button beside the existing fullscreen controls only on
  wide layouts where extending can provide additional width.
- **Compatibility: T0.** This is a dashboard-asset UI addition. No protocol,
  MCP argument, daemon store, shared configuration or stored-state change is
  part of this ticket. Width extension is transient and not persisted.

## Constraints

- This is accepted `todo/` capture only. Implementation must wait until
  `261114-feat-ws-whiteboard-compact-floating-toolbar` source has landed;
  overlapping files make the landed toolbar the implementation baseline.
  Coordinate with the unified-whole-history owner before touching shared
  entry-panel layout. Do not require unrelated whole-history completion.
- Preserve maximize/minimize/close ownership, focus and Escape behavior,
  panel disclosure state, scroll/anchor compensation, sticky headers and the
  Files pane lifecycle. Existing Overview exclusion while a sheet is maximized
  remains intact. Extension changes width, not maximize ownership.
- No additional fullscreen controls are added to standalone whiteboard pages.
  No remembered preference, new settings surface, daemon restart or deployment
  belongs to this change.
- Before implementation under `agents-plugin-tool/`, read
  `ai-docs/manuals/shipped-surface-boundary.md` and, for its internal paths,
  `ai-docs/manuals/release-compatibility.md` as declared by the project.
  Shipped UI behavior must remain downstream-neutral.

## Phases

### Phase 1: Cap shared maximize sheets and provide explicit wide extension

After the toolbar source lands, implement the confirmed shared width behavior
and adjacent explicit extension control across every surface listed in (733).
Keep state transient, resetting it whenever the shared sheet closes/minimizes
and on reload. Reuse the shared actions and sheet layout rather than adding
independent per-card fullscreen lifecycles. This is the complete behavior slice;
no implementation, fact population or ready promotion is authorized by capture.

Verification must cover default centering and the 1920px cap on a wider display,
explicit extension to available full width, and reset after minimize/close,
reopen and reload. Check preview, terminal, web, topic, framed entry-panel and
Files ownership paths. Check narrow/mobile and widths at or below the cap for
unchanged geometry and absence of an ineffective extension button. Retain
focus/Escape, Overview exclusion, panel disclosure, sticky-header and scroll
behavior. Use focused existing tests and representative browser evidence when
this ticket is executed; no acceptance run is part of this documentation capture.
