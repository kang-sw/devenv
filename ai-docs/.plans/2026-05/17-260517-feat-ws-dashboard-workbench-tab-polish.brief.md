# Brief: 260517-feat-ws-dashboard-workbench-tab-polish

## Intent

Make the ws-dashboard workbench tab strip feel like a coherent editor surface
without leaving the Dockview-owned layout model. This slice removes fake/default
tab behavior, adds tab close affordances and session-safe close confirmation,
stabilizes tab insertion/focus, and adds preview-to-pinned read-only file tabs.

## Scope Boundary

Implement the whole ready ticket: Phase 1 through Phase 3. The existing
skeleton commit `013bb1f` is binding for this slice. Do not widen into writable
editing, CodeMirror/Monaco, settings/configuration UI, layout persistence
redesign, or non-Dockview custom tab shells.

## Caller-Visible Contract

- Dockview remains the visible tab, split, and pane layout owner.
- Pinned/opened hierarchy must be visible through Dockview-native tab
  groups/chips where practical; fallback is pinned-left ordering plus
  badge/chip metadata and a clear active accent.
- Tab close affordances are hover-only.
- Terminal and agent tab close opens a small cursor-near `Yes`/`No` popover.
  `No` keeps the pane open and focus coherent. `Yes` proceeds with the existing
  daemon-backed close/terminate behavior.
- Reversible panes such as read-only editor preview, diagnostics, viewer,
  inspector, or resource projections close immediately and do not show the
  confirmation popover.
- Opened workRoots do not show mock/default tabs. Empty workbenches show honest
  empty or live-resource state.
- New file/editor panes prefer group 2, creating it when only group 1 exists;
  terminals prefer group 1; user-created groups 3+ are preserved but not
  automatic placement targets.
- Single-clicking a previewable file opens or replaces one preview tab for the
  selected workRoot. Double-clicking pins the file as a stable opened tab.
  Reopening an already pinned file focuses the existing pinned tab.

## Implementation Strategy Decisions

- Keep raw Dockview handles inside `dockviewLayout.tsx`; App and policy code
  should work with dashboard pane/group ids and logical surface keys.
- Use the skeleton helpers and metadata from `013bb1f` instead of inventing a
  second close or preview identity model.
- Use dashboard semantic CSS tokens for tab accents, badges, hover-only close
  controls, and popovers.
- Prefer license-free vector/icon treatment already available in the project.
  If adding a dependency is necessary, keep it small and permissively licensed.
  Emoji must not be the primary tab icon system.
- Implement the required post-implementation frontend-design verification and
  autonomous tweak pass before ordinary review handoff.

## Rejected Alternatives

- Browser-native `confirm()` / native modal close confirmation is rejected.
- Recreating the retired custom two-row pinned/opened tab shell is rejected.
- Making terminal tab close detach without daemon lifecycle termination is
  rejected; confirmation gates the existing terminate behavior.
- Treating unit/build checks as sufficient for visible UI behavior is rejected.

## Approach

- Wire Dockview tab rendering to expose hover-only close controls, visible
  pinned/opened grouping/chip or fallback metadata, and stable selectors for
  Playwright.
- Add App-level close request state that positions the confirmation popover near
  the close action and routes confirm/cancel through existing pane close logic.
- Extend close policy so session surfaces request confirmation and reversible
  surfaces close immediately.
- Replace fake/default tab initialization with honest empty/live state.
- Wire file explorer single-click to preview mode and double-click to pinned
  mode using the read-only pane mode skeleton.
- Preserve dynamic group placement and selected-workRoot scoping for preview,
  pinned, and terminal surfaces.

## Constraints

- Do not expose Dockview lifecycle APIs or handles as product behavior.
- Do not auto-target group 3+ for ordinary file/editor placement.
- Do not break terminal WebSocket input/output behavior, xterm focus, or
  close-as-terminate cleanup.
- Do not use private paths, pairing tokens, hostnames, or local tunnel details
  in committed evidence.

## Out of scope

- Writable files, dirty buffers, save flows, LSP/editor replacement, settings
  tab, persistent user layout storage, remote multi-server bridge work, and
  native-Windows-only terminal behavior changes.

## Details

- Skeleton final commit: `013bb1f`.
- Draft skeleton commit: `f603118`.
- The close presentation contract lives in workbench policy helpers.
- Dockview tab metadata and close request shape live in the Dockview adapter.
- Preview/pinned read-only file identity lives in `workRootFiles.ts`.
- Browser acceptance evidence lives in the Playwright dashboard acceptance gate.

## Verification Contract

Required implementation verification:

- `cd ws-dashboard/frontend && npm run test:workbench`
- `cd ws-dashboard/frontend && npm run test:work-root-files`
- `cd ws-dashboard/frontend && npm run test:terminals` when terminal close code
  or terminal pane wiring changes
- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`

Playwright evidence must cover hover-only close affordance, terminal/agent
confirmation popover cancel and confirm paths, immediate close for reversible
panes, pinned/opened grouping/chip or pinned-left fallback presentation, and
preview-to-pinned file behavior against the daemon-served production frontend.

Before ordinary review, run one delegated frontend-design verification and
autonomous tweak pass. That pass may adjust CSS/component details within this
brief and must rerun the relevant Playwright evidence before handoff.

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard workbench,
  Dockview adapter, terminal, file explorer, semantic CSS, and Playwright rules.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-workroot-workbench-substrate`,
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`,
  `260516-ws-web-dashboard-workroot-file-explorer`,
  `260516-ws-web-dashboard-readonly-text-pane`,
  `260516-ws-web-dashboard-file-open-placement-policy`,
  `260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state`, and
  `260516-ws-web-dashboard-terminal-close-termination`.
- [Must] `ai-docs/tickets/ready/260517-feat-ws-dashboard-workbench-tab-polish.md`
  - target ticket and phase success criteria. Implementer should not read this
  directly; fit reviewer may read it.
- [Maybe] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-dark-visual-system` for visual token guidance.
- [Maybe] `ai-docs/tickets/todo/260514-epic-ws-web-dashboard-mvp.md` - parent
  board context.
