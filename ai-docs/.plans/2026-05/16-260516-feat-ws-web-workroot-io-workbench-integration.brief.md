# Brief: 260516-feat-ws-web-workroot-io-workbench-integration

## Intent

Integrate the completed workRoot file explorer, read-only text panes, and
daemon-owned terminal panes into one coherent workbench workflow with restore
semantics, consistent command/placement behavior, and daemon-served dogfood
verification.

## Scope Boundary

Selected slice: all phases of `260516-feat-ws-web-workroot-io-workbench-integration`.

Implement or verify:

- Phase 1: Cross-Surface Restore Model.
- Phase 2: Placement And Command Polish.
- Phase 3: End-To-End Dogfood Verification.

Do not add new file editing, save/dirty-state behavior, terminal multiplexing,
agent presets, named-agent controls, detached terminal restore UX, or broad IDE
features.

## Caller-Visible Contract

Refreshing or re-entering the dashboard should preserve daemon-owned live
terminal sessions through daemon terminal listing, while browser arrangement
remains presentation state. Read-only file panes may remain browser-owned but
must honestly handle missing/unpreviewable files.

File-open, create-terminal, focus-existing-surface, close-terminal, and refresh
actions use consistent command ids and placement behavior. Duplicate logical
targets focus existing surfaces rather than creating redundant panes.

Dogfood verification must exercise the daemon-served frontend workflow: open or
select a workRoot, browse files, open a read-only text pane, create/use a
terminal, refresh without losing the terminal, close the terminal, and inspect
desktop and narrow layouts. If tooling prevents a step, record the exact
blocker.

## Implementation Strategy Decisions

- Keep daemon state authoritative for live terminal existence.
- Keep browser arrangement state presentation-only.
- Reuse existing workbench logical keys and placement helpers.
- Avoid a hidden detached terminal restore list; explicit terminal close still
  terminates.
- Prefer focused tests over new browser tooling unless existing setup supports
  it cleanly.

## Rejected Alternatives

- Do not make browser layout authoritative over daemon terminal lifecycle.
- Do not resurrect terminal sessions after explicit close.
- Do not add write-back editing.
- Do not hardcode agent launch presets.
- Do not invent a new global command system if the existing `data-command-id`
  and workbench helpers are enough.

## Approach

- Audit and tighten restore behavior for terminal panes from daemon live session
  listing and file panes from browser state.
- Unify or polish command ids for file open, terminal create, terminal close,
  refresh, and focus existing surfaces.
- Add tests around restore/list reconstruction, duplicate focus, placement, and
  close semantics where gaps remain.
- Run daemon-served frontend verification. Use production build plus
  `ws-dashboard serve --static-dir ws-dashboard/frontend/dist` or the local
  dev runner if that is the established path.
- Capture any verification blocker in ticket Result.

## Constraints

- Preserve owner auth and protected static serving.
- Preserve workRoot-relative file identity and opaque terminal ids.
- Preserve close-as-terminate.
- Keep visual layout compact and responsive for desktop/narrow checks.

## Out of scope

- New terminal emulator depth beyond the current substrate.
- File editing/writeback.
- Persistence of file panes across browser sessions unless already supported by
  existing arrangement state.
- Agent/harness-specific UI.

## Details

The implementation may be mostly polish/test/verification if prior child
implementations already satisfy much of the restore and placement behavior.
Do not churn source code just to create a diff; verify first, patch only the
gaps found against this brief.

## Verification Contract

- Run Rust and frontend test/build commands covering touched surfaces.
- Run daemon-served frontend dogfood verification or record exact blockers.
- Use delegated correctness/fit/test review focused on cross-surface restore,
  command placement, and dogfood coverage.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - implemented file/terminal surfaces and planned IO restore/command/dogfood contracts.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - workbench placement, static serving, and IO surface invariants.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-workroot-io-workbench-integration.md` - selected integration scope.
- [Must] `ai-docs/tickets/todo/260516-epic-ws-web-dashboard-workroot-io-substrate.md` - milestone completion criteria.
