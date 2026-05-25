# Brief: 260521-feat-ws-dashboard-command-dispatch-spine

## Intent

Add the minimal frontend command dispatch spine required before Activity
Console controls are implemented. Existing visible controls should keep their
stable command ids, but representative controls must execute through a shared
command dispatch path rather than logging a command id and then separately
running adjacent callbacks.

## Scope Boundary

Implement Phase 1 only: add the frontend command dispatch spine and migrate the
representative controls listed in the ticket. Do not implement the tmux-like
prefix keybinding table, Activity Console UI, agent controls, or terminal raw
input routing.

## Caller-Visible Contract

User-visible dashboard controls continue to expose stable `data-command-id`
values. For migrated controls, clicking the UI and dispatching the same command
programmatically invoke the same side-effect path and produce the same visible
state change. Terminal raw byte input remains outside dashboard command
dispatch.

## Contract Instructions

- Work in `ws-dashboard/frontend/src/`.
- Add a thin command model/dispatcher that can be reused by `App.tsx` and later
  Activity Console components.
- Preserve current command log behavior, or replace it with an equivalent
  observer attached to the dispatch path.
- Command payloads must use logical dashboard targets such as resource ids,
  workRoot ids, pane ids, logical surface keys, activity ids, or terminal ids.
  Do not include host paths, cache paths, stream paths, pids, backend session
  paths, or terminal input bytes in command payloads.
- Migrate at least these commands so click behavior and programmatic dispatch
  use the same executable path:
  - `dashboard.refresh`
  - `workRoot.open`
  - `fileExplorer.refresh`
  - `fileExplorer.toggleDirectory`
  - `fileExplorer.openFile`
  - `fileExplorer.selectEntry`
  - `workbench.openActivity`
  - `terminal.create`
- Audit workbench close/select/move paths. Migrate only if it stays contained;
  otherwise leave a clear implementation note and do not block Activity Console
  on large lifecycle cleanup.
- Leave an obvious API for later Activity Console commands such as
  `activity.selectItem`, `activity.transcript.loadMore`, `activity.refresh`,
  and detail toggles.

## Integration Test Instructions

- Add or extend frontend route/model tests under `ws-dashboard/frontend/src/`
  to prove command dispatch parity for representative migrated controls.
- Extend browser coverage when practical to prove at least one click path and
  one programmatic dispatch path produce the same visible state change in the
  daemon-served frontend.
- Required commands:
  - `npm run test:work-root-files`
  - `npm run test:workbench`
  - any new targeted frontend test command added for the command spine
  - `npm run build`
- Run `npm run test:browser` if browser-level coverage changed or if the new
  parity proof is implemented there.

## Implementation Strategy Decisions

- This is a thin spine, not a keybinding feature.
- Stable command ids are executable behavior keys, not only DOM selectors or
  log labels.
- Terminal raw input remains xterm/WebSocket-owned because shell input fidelity
  must not be forked through dashboard commands.
- Representative migration is enough for Phase 1; broad workbench lifecycle
  cleanup can follow if it would dominate the slice.

## Rejected Alternatives

- Full tmux-like keybinding UI now: rejected because the current blocker is the
  shared executable command path, not the keybinding table.
- Daemon-backed command identity: rejected for this phase; command dispatch is
  frontend-side behavior over existing authenticated dashboard APIs.
- Routing terminal input bytes through dashboard commands: rejected because it
  would fork shell input semantics and risk IME/control-key regressions.

## Approach

- Extract command id/payload/log types from `App.tsx` into a small command
  module.
- Give `App.tsx` a single dispatch callback that logs and executes registered
  command handlers.
- Route migrated controls through that dispatch callback rather than calling
  `onCommand` plus a separate side-effect callback.
- Keep `data-command-id` equal to the dispatched command id.
- Add pure tests around command payload/handler behavior where possible, then
  keep existing component behavior covered by current route/workbench tests.

## Constraints

- Preserve owner-auth, resource identity, workbench placement policy, and
  selected-workRoot behavior.
- Preserve file explorer row semantics: directory rows toggle, previewable file
  rows open read-only previews, other rows select.
- Preserve WorkRoot Activity duplicate-open/focus behavior.
- Preserve terminal create behavior and terminal raw input handling.
- Do not leak private paths or backend-native identifiers into command payloads.

## Out of scope

- Activity Console UI.
- tmux-like prefix binding table or keymap customization.
- Agent start/interrupt/cancel/erase/retry controls.
- Command routing for terminal raw input.
- Large workbench close/select/move refactors unless they remain contained.

## Details

Current code has `CommandPayload`, `CommandEntry`, and `executeCommand` inside
`App.tsx`. `executeCommand` currently handles select/refresh and logs recent
commands, while controls such as `workRoot.open`, `fileExplorer.openFile`,
`workbench.openActivity`, and `terminal.create` often call `onCommand` and then
run side effects through adjacent direct callbacks. Phase 1 should make the
migrated command ids dispatch the side effects themselves.

## Verification Contract

- Tests demonstrate command-id/dispatch parity for migrated controls.
- Existing file explorer and workbench tests still pass.
- Build succeeds.
- Browser-level evidence exists when the chosen parity proof touches rendered
  dashboard behavior beyond pure command model tests.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `{#260516-ws-web-dashboard-inspectable-navigation-shell}` planned command
  spine behavior.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard domain rules
  for command identities, browser verification, terminal exceptions, and
  workbench/file/terminal contracts.
- [Must] `ws-dashboard/frontend/src/App.tsx` - current command log, visible
  controls, file explorer, activity opener, and terminal create path.
- [Must] `ws-dashboard/frontend/src/workbench/` - workbench placement, close,
  tab, and lifecycle policy.
- [Must] `ws-dashboard/frontend/src/workRootFiles.test.ts` - file explorer
  behavior coverage.
- [Must] `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts` -
  workbench placement and lifecycle coverage.
- [Maybe] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` -
  browser-level command selector and interaction coverage.
- [Maybe] `ws-dashboard/frontend/src/terminals.ts` and
  `ws-dashboard/frontend/src/terminals.test.ts` - terminal creation/input
  boundaries.
