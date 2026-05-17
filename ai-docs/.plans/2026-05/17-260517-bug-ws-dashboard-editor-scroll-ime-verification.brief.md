# Brief: 260517-bug-ws-dashboard-editor-scroll-ime-verification

## Intent

Restore confidence in the dashboard workbench after tab polish by proving that
long read-only editor content scrolls inside its pane and that focused browser
terminal input preserves native shell line-editing controls plus IME fallback
guard behavior through the live xterm/WebSocket path.

## Scope Boundary

Implement Phase 1 and Phase 2 of
`260517-bug-ws-dashboard-editor-scroll-ime-verification` only. Satisfy the
existing skeleton contracts in
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` from skeleton commit
`70881e5`.

Do not implement a richer editor library, a configuration tab, a new terminal
transport, or the separate native-Windows Ctrl-C investigation ticket.

## Caller-Visible Contract

- Long read-only file content opened in the workbench scrolls inside the
  read-only text pane. Wheel interaction over the file body must not create or
  move top-level browser document scroll and must not push dashboard chrome out
  of view.
- Focused terminal panes preserve xterm's ordinary input path for native shell
  line editing. `ctrl-u` clears the current shell line and `ctrl-w` deletes the
  previous word, with shell-visible effects and WebSocket input frames.
- Browser fallback key handling must not forward IME composition-in-progress
  keystrokes as raw terminal bytes. Real Korean IME commit evidence may be
  manual if Playwright cannot synthesize platform IME composition.

## Implementation Strategy Decisions

- Treat xterm's focused helper textarea / `onData` path as the preferred input
  path. Fallback keyboard handling should be narrower than xterm, not a
  replacement for it.
- Keep scroll containment in the existing read-only pane and Dockview layout
  chain. The ticket must not introduce a new editor dependency to make the
  containment test pass.
- Preserve the current WebSocket terminal transport. Fix focus, fallback, or
  layout issues locally instead of reintroducing polling or alternate terminal
  input transport.
- Keep terminal command assertions platform-aware where command text differs.

## Rejected Alternatives

- Do not treat ASCII Playwright typing alone as IME evidence.
- Do not hide the editor scroll issue behind a future CodeMirror/editor
  replacement.
- Do not broaden this slice into native-Windows Ctrl-C behavior; that remains
  tracked by `260517-bug-ws-dashboard-windows-terminal-control-keys`.

## Approach

- Make the existing skeleton browser acceptance test pass for long read-only
  file scroll containment.
- Make the terminal acceptance test pass for `ctrl-u`, `ctrl-w`, WebSocket input
  frame evidence, and synthetic composition fallback guard behavior.
- If Playwright cannot drive real Korean IME commit behavior, produce a scoped
  manual evidence artifact in the dashboard browser evidence location and state
  exactly what remains automated versus manual.
- Keep fixes surgical and prefer existing helpers/components over new
  abstractions.

## Constraints

- Preserve Dockview as the visible workbench owner.
- Preserve tab polish behavior, including preview/pinned file tabs and terminal
  close policy.
- Do not regress terminal WebSocket behavior, ANSI/control rendering, pane fill,
  bottom-row visibility, or no-polling-while-connected evidence.
- Avoid OS-specific command strings in browser acceptance where the existing
  command plan already supplies a helper.

## Out of scope

- Full browser-native editor replacement.
- New dashboard settings/configuration UI.
- General Windows console Ctrl-C/interrupt semantics.
- Multi-server or remote bridge behavior.

## Verification Contract

- `npm run test:terminals`
- `npm run build`
- `npm run test:browser`
- Browser evidence must include read-only scroll containment and terminal
  `ctrl-u` / `ctrl-w` behavior through the daemon-served frontend.
- Record whether Korean IME commit behavior is automated or manually evidenced.

## References

- [Must] `260516-ws-web-dashboard-browser-ui-acceptance-gate` - browser-level
  acceptance gate and evidence baseline.
- [Must] `260516-ws-web-dashboard-readonly-text-pane` - existing read-only pane
  contract.
- [Must] `260517-ws-dashboard-readonly-text-scroll-containment` - exact Phase 1
  planned behavior.
- [Must] `260516-ws-web-dashboard-browser-terminal-emulator-behavior` - terminal
  emulator baseline.
- [Must] `260516-ws-web-dashboard-terminal-websocket-input-fidelity` - live
  xterm/WebSocket input contract.
- [Must] `260517-ws-dashboard-terminal-ime-and-line-editing-fidelity` - exact
  Phase 2 planned behavior.
- [Must] `260516-ws-web-dashboard-terminal-shell-selection-portability` -
  shell-profile portability boundary.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard browser-gate,
  terminal, and workbench guidance.
- [Maybe] `260516-ws-web-dashboard-terminal-platform-command-helpers` - command
  helper contract for portable terminal evidence.
