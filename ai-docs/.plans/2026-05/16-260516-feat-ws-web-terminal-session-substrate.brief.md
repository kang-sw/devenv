# Brief: 260516-feat-ws-web-terminal-session-substrate

## Intent

Implement the first usable daemon-owned shell terminal substrate for opened
workRoots: create/list live sessions, move terminal output/input through
authenticated routes, render terminal panes in the workbench, preserve sessions
across browser refresh, and terminate sessions on explicit close.

## Scope Boundary

Selected slice: all phases of `260516-feat-ws-web-terminal-session-substrate`.

Implement:

- Phase 1: Daemon Terminal Registry And PTY Spawn.
- Phase 2: Terminal I/O Transport.
- Phase 3: Frontend Terminal Pane.
- Phase 4: Close Semantics And Verification.

Do not add Codex/Claude/agent presets, named-agent controls, detached restore
UX, full terminal multiplexing, terminal search, broad scrollback persistence,
or custom keybinding UI.

## Caller-Visible Contract

Authenticated owners can create shell terminal sessions scoped to an opened
workRoot. Terminal ids are opaque browser-facing ids, not process ids or host
paths. Live terminal sessions survive browser refresh because the daemon owns
their lifecycle. Explicitly closing a terminal panel terminates the daemon
session.

The browser can list live sessions, open/focus a terminal pane, send input,
observe output, and request bounded resize. Terminal panes use workbench
placement policy and do not continuously rewrite logical PTY/TUI dimensions
during visual split drag.

## Implementation Strategy Decisions

- Reuse the opened workRoot registry for spawn working directories.
- Keep terminal state in daemon-owned process memory for this first substrate.
- Prefer a proven Rust PTY crate already compatible with the workspace if one
  exists or can be added narrowly.
- Use an authenticated route family for terminal create/list/output/input/resize
  and close. WebSocket/SSE is allowed if simple; polling output is acceptable
  only if it still supports interactive input and deterministic tests.
- Keep frontend terminal rendering simple and functional. Use xterm.js if adding
  the dependency is straightforward; otherwise implement a basic monospace
  terminal pane that can display output and send input for this substrate.

## Rejected Alternatives

- Do not hardcode agent launch presets.
- Do not treat browser layout as terminal lifecycle authority.
- Do not keep hidden detached terminal sessions after explicit close.
- Do not continuously resize PTY dimensions during visual split dragging.
- Do not expose host paths or process ids as public terminal identity.

## Approach

- Add daemon terminal registry and route handlers with tests for auth, create,
  list, output/input, resize, close, and close termination.
- Add frontend terminal API helper/types/tests.
- Add terminal pane state and UI in the workbench for selected workRoot live
  sessions.
- Add create-terminal and close-terminal command ids.
- On refresh/reload of frontend state, reconstruct terminal panes from daemon
  live session listing where practical within current component state.
- Keep close wired to terminate the daemon session, not detach.

## Constraints

- Every terminal route stays owner-authenticated.
- WorkRoot ids and terminal ids are opaque.
- Terminal session creation requires an opened online workRoot.
- Tests should avoid relying on a specific user shell prompt; use deterministic
  command/input fixtures where possible.
- Keep frontend text/layout compact and dark-token based.

## Out of scope

- Agent presets, named-agent protocol control, MCP authority.
- Full terminal emulator polish beyond first substrate.
- Detached terminal restore UX.
- Terminal search, scrollback persistence beyond first usable output buffer.
- Foreground-process close confirmation.

## Details

Route shapes may vary to fit local Axum patterns, but should be workRoot and
terminal scoped, for example:

```text
POST /api/dashboard/work-roots/{workRootId}/terminals
GET  /api/dashboard/work-roots/{workRootId}/terminals
GET  /api/dashboard/terminals/{terminalId}/output?after=<cursor>
POST /api/dashboard/terminals/{terminalId}/input
POST /api/dashboard/terminals/{terminalId}/resize
DELETE /api/dashboard/terminals/{terminalId}
```

## Verification Contract

- Run Rust formatting, daemon route tests, daemon crate tests, and cargo check.
- Run frontend terminal/helper/workbench tests and frontend build.
- Add delegated correctness/fit/test review focused on lifecycle, auth, I/O,
  close termination, refresh persistence, and scope control.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - daemon foundation, workRoot resource model, workbench substrate, instance event scaffold, and planned terminal contracts.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - auth, daemon-owned lifecycle, workbench, and terminal sizing invariants.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-terminal-session-substrate.md` - selected terminal implementation scope.
- [Must] `ai-docs/tickets/todo/260516-epic-ws-web-dashboard-workroot-io-substrate.md` - milestone terminal decisions and exclusions.
- [Maybe] `ai-docs/tickets/todo/260516-feat-ws-web-workroot-io-workbench-integration.md` - later restore/dogfood integration expectations.
