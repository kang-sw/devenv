---
title: ws dashboard command dispatch spine
parent: 260518-epic-ws-dashboard-activity-console
spec:
  - 260516-ws-web-dashboard-inspectable-navigation-shell
related:
  260518-feat-ws-dashboard-activity-console-ui: first new surface that should build controls on the command spine
  260517-feat-ws-dashboard-workroot-activity: existing Activity pane entrypoint and live-refresh surface to preserve
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard command dispatch spine

## Background

The dashboard already exposes many `data-command-id` values and a lightweight
`onCommand` path, but recent analysis found this is not yet a full command
dispatch spine. Several visible controls log or expose a command identity and
then execute side effects through adjacent direct callbacks. That is acceptable
for the current substrate, but continuing that pattern through Activity Console
would create keybinding debt: a future tmux-like prefix layer would need to
reconstruct click handlers instead of invoking shared dashboard commands.

This ticket is the Phase 0 guard before Activity Console UI work. It establishes
the minimal frontend command spine needed so new controls can route through a
single command-dispatch path from the start.

## Decisions

- Implement a thin frontend command dispatch spine, not a full keybinding UI.
- Treat stable command ids as executable behavior keys, not only DOM selectors
  or log labels.
- Keep terminal raw byte input outside dashboard command dispatch. Shell input
  fidelity remains owned by xterm/WebSocket input paths.
- Use logical dashboard targets in command payloads: opaque resource ids,
  pane ids, logical surface keys, activity ids, and terminal ids where needed.
  Do not carry host paths, cache paths, stream paths, pids, or backend session
  paths as command identity.
- Activity Console controls added later must dispatch commands such as
  `activity.selectItem`, `activity.transcript.loadMore`, `activity.refresh`,
  and existing workbench open/focus commands rather than raw UI callbacks.

## Constraints

- Do not implement the tmux-like prefix keybinding table in this ticket.
- Do not add new agent control actions.
- Do not route terminal raw input through dashboard commands.
- Keep the existing command log behavior or replace it with an equivalent
  command-spine observer so browser evidence can still show recent command ids.
- Limit existing-control migration to representative paths needed to prove the
  spine and unblock Activity Console. Large workbench lifecycle cleanup can be
  split if it would dominate the phase.

## Phases

### Phase 1: Add the frontend command dispatch spine

Introduce a central frontend command dispatch path that maps command ids plus
logical payloads to dashboard side effects. Absorb the current `onCommand`
logging path under that dispatch path, then migrate representative existing
visible controls so their click behavior and dispatched command behavior are
the same path.

Minimum migrated controls:

- `dashboard.refresh`
- `workRoot.open`
- `fileExplorer.refresh`
- `fileExplorer.toggleDirectory`
- `fileExplorer.openFile`
- `fileExplorer.selectEntry`
- `workbench.openActivity`
- `terminal.create`

Also audit close/select/move workbench paths and record whether they are
migrated in this phase or intentionally left as a follow-up cleanup. The
implementation must leave Activity Console UI work with an obvious command
dispatch API for ribbon selection, transcript refresh/load-more, and pane
open/focus controls.

Verification should cover command id and dispatch parity for the migrated
controls, command log/observer behavior, no host/cache/session/path leakage in
command payloads, and a browser-level check proving at least one click path and
one programmatic dispatch path produce the same visible state change.
