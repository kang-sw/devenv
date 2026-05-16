# Brief: 260516-feat-ws-web-workbench-substrate Phase 3

## Intent

Lock the first workbench placement and lifecycle semantics before live editor,
terminal, viewer, task, and PTY producers exist. This phase should make future
surface opens deterministic without turning the current visible shell into a
live IDE or terminal implementation.

## Approach

- Add dashboard-owned workbench policy/model helpers for:
  - opening editor/file-like surfaces in the second or later split group when
    such a group exists;
  - focusing an existing attachment instead of opening duplicate surfaces when
    the same logical surface key is already present;
  - opening agent and persistent-terminal surfaces in the focused group, falling
    back to the first group;
  - representing panel close as `detach` for daemon-backed resources by default;
  - reserving explicit terminate commands as separate lifecycle actions rather
    than treating close as termination;
  - preserving PTY/TUI logical dimensions separately from visual split size.
- Wire the visible Phase 2 shell to surface these semantics only as current UI
  contract/affordance where useful. Do not create live backends.
- Extend workbench tests so the policy is executable and future phases cannot
  accidentally reintroduce duplicate opens, close-as-terminate, or visual-size
  driven PTY dimensions.

## Constraints

- Scope is Phase 3 only.
- Do not add live PTY, live terminal, editor, viewer, task, diagnostics, or
  inspector backends.
- Do not add layout persistence, drag/drop editing, free docking, keyboard
  navigation, or a real file explorer.
- Do not make browser routes, serialized layout, or Dockview state authoritative
  over daemon server/workspace/workRoot/instance identity.
- Do not expose raw Dockview panel/group lifecycle APIs.
- Preserve existing visible resource fetch behavior, route normalization,
  command ids for resource actions, dark visual tokens, and the Phase 2
  workRoot navigation model.

## Out of scope

- Opening real files from a filesystem picker.
- Closing or terminating real daemon processes.
- Live PTY resize integration.
- Persisting or restoring the policy result to storage.

## Details

Treat these as product semantics even while the implementation is mostly model
and contract code:

- `editor`, `viewer`, `diff`, `diagnostics`, `eventsLog`, `taskView`, and
  `inspector` are opened/support surfaces and should prefer group 2+.
- `agent` and `persistentTerminal` are durable pinned surfaces and should prefer
  the focused group, then group 1.
- Surface dedupe is keyed by dashboard-owned logical surface keys, not by raw
  Dockview panel ids or daemon process ids alone.
- Close policy resolves from the surface registry: daemon-backed surfaces detach
  by default; explicit terminate actions are separate command reservations.
- PTY/TUI logical dimensions are stable model values. Visual split resizing may
  request a future resize decision, but it must not continuously rewrite logical
  terminal dimensions in this phase.

## Verification

- Run `cd ws-dashboard/frontend && npm run test:routes && npm run test:workbench && npm run build`.
- Delegate a focused review for policy correctness and scope fit.
- Visual screenshots are optional for this phase unless visible shell behavior
  changes materially; record if no visual change was made.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-workroot-workbench-substrate`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - workbench adapter,
  serialized layout, left-nav identity, and Dockview boundary rules.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-workbench-substrate.md` -
  Phase 3 scope and prior phase results.
- [Must] `ws-dashboard/frontend/src/workbench/` - Phase 1 workbench registry,
  layout serialization, and bridge contracts.
- [Must] `ws-dashboard/frontend/src/App.tsx` - visible Phase 2 shell if small
  contract affordances are needed.
