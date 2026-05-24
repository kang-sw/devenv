---
title: Clarify dashboard context surface hierarchy
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-ws-dashboard-visual-building-blocks-first-pass: reusable CSS primitive layer this pass should extend rather than bypass
  260524-feat-ws-dashboard-icon-chrome-refinement: most recent icon/chrome polish pass; this ticket should preserve its quiet control treatment
  260524-research-ws-dashboard-visual-design-system-refresh: broader visual-system research remains separate from this tactical hierarchy pass
  260524-feat-ws-dashboard-document-viewer-editor-substrate: future markdown/document editor surfaces should inherit the resulting pane hierarchy grammar
spec:
  - 260524-dashboard-context-surface-hierarchy
  - 260516-ws-web-dashboard-dark-visual-system
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-24
---

# Clarify dashboard context surface hierarchy

## Background

The icon-first dashboard chrome pass made controls quieter and reduced
metadata clutter, but the main shell still uses nearly identical borderline
treatment for unrelated levels of context. The user can distinguish individual
components, but larger working regions such as left navigation, the workRoot
topbar, a Dockview split group, an editor shell, an editor tabbar, an internal
ribbon, and the body content do not yet read as intentionally nested context
units.

This ticket captures the final visual-polishing pass for the current branch:
make the hierarchy between context regions visible without reintroducing heavy
boxed controls, changing dashboard behavior, or redesigning pane content.

## Decisions

- Treat this as a visual hierarchy pass over the existing dashboard shell.
  Command ids, payloads, daemon routes, resource ids, Dockview placement policy,
  Activity data, terminal behavior, file explorer data, and document/editor API
  direction remain unchanged.
- Prefer surface tone, spacing, gutter treatment, and token hierarchy over
  simply adding darker or thicker borders everywhere.
- Separate structural context boundaries from local dividers and from
  selection/focus states. A selected row, a split group boundary, a tabbar/body
  divider, and a hover-revealed icon button should not all use the same visual
  grammar.
- Treat an editor or document pane as one context island whose tabbar,
  optional internal ribbon, and body belong together. The topbar and other
  workbench groups should read as separate context units.
- Keep Dockview as the visible split/tab owner. Add dashboard-local visual
  grammar around Dockview-owned regions rather than replacing Dockview layout
  behavior.

## Phases

### Phase 1: Add context surface hierarchy to dashboard chrome

Define and apply a dashboard-local context surface hierarchy so the browser UI
distinguishes large working regions, group boundaries, and local internal
dividers.

The implementation should:

- Extend the semantic CSS token vocabulary for context surfaces and dividers.
  Suggested roles include app/background surface, region surface, context
  surface, pane surface, pane body surface, structural border, context border,
  local divider, and split gutter. Exact token names may follow the existing
  `styles.css` vocabulary.
- Update `ws-dashboard/frontend/DESIGN.md` when the token or building-block
  vocabulary changes so future UI work can reuse the hierarchy grammar.
- Make the `left nav | workRoot workbench` split read as a large application
  structure boundary without making every nested nav section look equally
  heavy.
- Clarify the relationship between workRoot topbar, Dockview tab group, and
  pane body. The topbar should read as workRoot-level chrome; a Dockview group
  should read as the owner of its tabbar and body; tabbar, optional internal
  pane ribbon, and content should read as one editor/document island.
- Give Dockview split group boundaries a distinct structural treatment, such
  as a gutter or surface contrast, that is stronger than local tab/body
  dividers but quieter than active selection states.
- Keep local dividers muted inside a context unit. A tabbar-to-body separator,
  internal ribbon separator, or file explorer section separator should not
  compete with the outer region boundary.
- Preserve the quiet glass-like icon button behavior from the icon chrome
  refinement. Do not solve hierarchy by making icon buttons permanently boxed
  again.
- Preserve readable selected/active states while keeping them separate from
  structural borders. Selected workRoot rows, active tabs, and focus-visible
  rings must remain visible.
- Keep main pane content semantics unchanged. Do not redesign Activity
  transcript body typography, terminal body rendering, markdown/document
  renderer behavior, translation overlays, or save fan-out in this phase.

Completion means a screenshot reader can tell which elements belong to the
same working context and where one large context ends: left navigation versus
workRoot workbench, topbar versus workbench groups, a Dockview split group
versus another group, and an editor/document shell versus its internal body.

Deferred scope:

- Broad redesign of the dashboard visual style, typography scale, or color
  palette beyond the hierarchy tokens needed here.
- Root picker layout or interaction changes.
- New React Aria adoption outside incidental shared styling.
- Markdown viewer/editor implementation, translation overlay work, raw-text
  editing, save fan-out, or document backend changes.
- Terminal emulator fit/resize changes or Activity Console data/model changes.

Verification should include:

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`
- Browser screenshots covering the desktop workbench, a constrained viewport,
  at least one Dockview split/group state, the left navigation with file
  explorer visible, and a file/editor-like pane if available.
- Manual screenshot review that large context boundaries are more readable
  than local dividers and that no hierarchy fix reintroduces heavy always-on
  button borders.

### Result (4afb744) - 2026-05-24

Implemented the context surface hierarchy pass over the existing dashboard
chrome. `styles.css` now exposes distinct semantic roles for region, context,
pane, pane-body, document-chrome, local divider, context divider, structural
divider, and split gutter surfaces. The left navigation, workRoot topbar,
Dockview group/tab/body stack, and read-only editor pane consume those tokens
so large working contexts read separately from local tab/body or ribbon/body
dividers.

The implementation preserved behavior and command routing: resource selection,
topbar commands, Dockview ownership, terminal behavior, file explorer data, and
read-only pane semantics remain unchanged. The browser gate now asserts that
nav/workbench regions, topbar/tabbar/body surfaces, structural/local divider
tokens, and the Dockview split gutter are visually distinct.

Verification passed:

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`
- Manual screenshot review of `desktop-workbench.png`, `file-explorer.png`,
  `narrow-workbench.png`, and `topbar-overflow.png`
