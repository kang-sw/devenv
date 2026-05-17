---
title: ws dashboard workbench tab polish
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-epic-ws-web-dashboard-workbench-substrate: owns the dashboard workbench substrate and tab policy boundary
  260516-epic-ws-web-dashboard-workroot-io-substrate: introduced read-only file panes and live terminal panes under opened workRoots
  260516-bug-ws-web-dashboard-ui-acceptance-recovery: recovered baseline terminal/file explorer usability and browser visual gates
  260517-bug-ws-dashboard-dockview-workbench-parity: prerequisite corrective work to make Dockview the visible workbench layout substrate before further tab polish
  260517-bug-ws-dashboard-dockview-dynamic-groups: dynamic split-group behavior should land before tab polish that depends on group placement and movement semantics
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
  - 260516-ws-web-dashboard-readonly-text-pane
  - 260516-ws-web-dashboard-file-open-placement-policy
  - 260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state
  - 260516-ws-web-dashboard-terminal-close-termination
skeletons:
  phase-1: 013bb1f
  phase-2: 013bb1f
  phase-3: 013bb1f
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard workbench tab polish

## Background

The dashboard now has live workRoot file panes and terminal panes, but several
small visible behaviors still make the workbench feel less like a coherent
editor surface. These are not just cosmetic tweaks: close affordances, tab
placement, placeholder cleanup, and preview/pin behavior are workbench lifecycle
policy.

The current read-only file pane is intentionally a simple read-only text preview
rather than CodeMirror or Monaco. That choice is acceptable for this slice. The
goal is to polish the surrounding tab interaction before introducing a richer
editor library, so later editor integration can reuse stable lifecycle rules
instead of redefining them.

This ticket depends on `260517-bug-ws-dashboard-dockview-workbench-parity`.
Do not implement these polish behaviors on top of the current custom
React/CSS/HTML5-drag tab engine; first correct the substrate so Dockview owns
the visible workbench groups, tabs, and pane layout behind dashboard policy.
It also depends on `260517-bug-ws-dashboard-dockview-dynamic-groups` for split
group semantics: tab polish should not encode fixed `primary`/`support`
assumptions if Dockview dynamic groups are still missing.

## Decisions

- Do not add a configuration/settings tab in this slice.
- Do not add CodeMirror, Monaco, writable editing, dirty buffers, save flows, or
  layout-persistence redesign here.
- Keep Dockview behind the dashboard-owned workbench registry and policy layer;
  do not expose raw Dockview handles or lifecycle APIs as product behavior.
- Treat Dockview parity as a hard prerequisite. This ticket should extend the
  Dockview-backed shell, not preserve or further entrench the custom tab/split
  implementation.
- Treat dynamic group support as a prerequisite for placement-sensitive tab
  polish. Close, insertion, and preview behavior should operate on the dynamic
  group model rather than on fixed primary/support group names.
- Preserve the pinned/opened information structure inside the Dockview-owned
  tab surface. Do not silently flatten pinned and opened panes into an
  indistinguishable single-row presentation.
- Prefer Dockview-native tab groups/chips as the primary pinned/opened
  presentation. If the current Dockview version or adapter shape makes native
  tab grouping unsuitable for this slice, keep Dockview as the tab owner and
  use pinned-left ordering plus chip/badge metadata as the fallback. Do not
  revive a competing custom two-row tab shell.
- Keep pinned panes visually distinguishable from ordinary opened panes.
  Pinned agent and terminal tabs should render license-free vector icon badges
  or an equivalent in-repo icon treatment; emoji must not be the primary icon
  system because platform font rendering changes tab width and tone. If a new
  icon package is introduced, prefer a small permissively licensed set such as
  `lucide-react` over a heavier icon dependency.
- Make active and pinned tab styling clearer than the current flattened tabs:
  a bright but restrained accent line is acceptable, pinned badges may use
  higher-contrast color, and terminal/agent/editor surface kinds should be
  distinguishable without relying on text labels alone.
- Render tab close buttons as hover-only affordances so the tab strip stays
  dense when the user is scanning tabs.
- Do not use browser-native confirmation dialogs for tab close. Closing a
  terminal or agent pane should open a small cursor-near confirmation popover
  with explicit `Yes` and `No` actions because those surfaces represent live or
  daemon-backed sessions. Closing reversible panes such as read-only editor
  preview, diagnostics, or resource views should close immediately without
  confirmation.
- The close confirmation popover must be local to the tab action: cancel keeps
  the pane open and focus coherent, while confirm closes the pane and leaves
  focus on the same deterministic neighboring-tab or empty-state policy used by
  ordinary close.
- Treat preview mode as frontend workbench policy: a single-click preview may be
  replaced by the next preview open, while a double-click pins the file as a
  normal opened tab.
- Preserve daemon-owned terminal lifecycle. Terminal close still terminates the
  daemon session; file pane close only removes the frontend attachment.
- Visible behavior in this ticket must be proven through the Playwright browser
  gate. Unit/model tests and build checks are supporting evidence, not
  replacements for hover, popover, immediate-close, tab grouping/chip, and
  preview/pin browser evidence.
- After implementation and before reviewer handoff, run one delegated
  frontend-design verification and autonomous tweak pass. That pass may adjust
  CSS or component details within this ticket's policy, then must rerun the
  relevant Playwright evidence before the ordinary implementation review.

## Phases

### Phase 1: Normalize visible tab lifecycle

Remove remaining default/mock tab list behavior from opened workRoots and make
empty workbenches explicit without showing fake editor or terminal tabs. Add
hover-only close affordances to workbench tabs where the close action is safe
and well-defined.

Success means an opened workRoot starts from an honest empty or live-resource
state, every visible closeable tab exposes its close affordance on hover, and
closing a reversible pane leaves focus on a predictable neighboring tab or
empty state. The same browser evidence should confirm that pinned agent and
terminal tabs are visually distinct through Dockview tab rendering, with
readable icon badges and active accent styling.

Terminal and agent close actions must show the small cursor-near `Yes`/`No`
popover instead of closing immediately. Browser evidence must prove both cancel
and confirm paths. Reversible panes such as read-only editor preview,
diagnostics, and resource views must close immediately and must not show that
confirmation popover.

### Phase 2: Stabilize tab insertion and focus policy

Define where newly opened file and terminal tabs appear. New tabs should not
surprise the user by appearing at the far right of an unrelated row or by
stealing focus repeatedly after background resource refreshes. If the intended
policy is "new tabs open at the left edge," implement that consistently across
read-only file panes and terminal panes; otherwise record and implement the
chosen placement rule explicitly.

Within each Dockview group, pinned panes should cluster before ordinary opened
panes unless Dockview-native tab groups/chips provide a clearer equivalent.
Native tab groups/chips are the preferred route for this ticket; pinned-left
ordering plus badges is the fallback when grouping is not practical in the
current Dockview adapter. User movement must either preserve that category
ordering or deliberately record a new dashboard-owned category/order policy; it
must not accidentally erase pinned/opened semantics.

Success means tab insertion order, duplicate-open focusing, close-after-focus,
and refresh reconciliation are deterministic and covered by workbench model
tests plus browser interaction evidence. Browser verification should include a
desktop screenshot or DOM assertion that pinned tabs remain left-biased or
grouped and visually separable from opened tabs.

### Phase 3: Add preview-to-pinned file tabs

Implement read-only file preview mode for file explorer opens. A single click
on a previewable file opens or replaces one preview tab for the selected
workRoot. A double click pins that file as a stable opened tab that is not
replaced by later preview opens. Reopening an already pinned file focuses the
existing pinned tab.

Success means preview replacement, double-click pinning, duplicate pinned-file
focus, and selected-workRoot scoping are all deterministic. The browser gate
should prove the interaction with real daemon-served file content and include
visual/DOM evidence for tab state.
