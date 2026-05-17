---
title: ws dashboard editor scroll and terminal input fidelity follow-up
parent: 260514-epic-ws-web-dashboard-mvp
completed: 2026-05-17
skeletons:
  phase-1-2: 70881e5
related:
  260517-feat-ws-dashboard-workbench-tab-polish: introduced Dockview tab polish and the hotfix that added an IME fallback guard
  260516-epic-ws-web-dashboard-workbench-substrate: owns workbench pane lifecycle, focus, and browser evidence policy
  260517-bug-ws-dashboard-windows-terminal-control-keys: related native-Windows terminal control-key gap noted during terminal portability dogfood
spec:
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
  - 260516-ws-web-dashboard-readonly-text-pane
  - 260517-ws-dashboard-readonly-text-scroll-containment
  - 260516-ws-web-dashboard-browser-terminal-emulator-behavior
  - 260516-ws-web-dashboard-terminal-websocket-input-fidelity
  - 260517-ws-dashboard-terminal-ime-and-line-editing-fidelity
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard editor scroll and terminal input fidelity follow-up

## Background

After the workbench tab polish implementation, two follow-up risks remain that
should not be buried inside the completed tab polish ticket.

First, read-only editor pane scrolling may still propagate to the top-level
browser document instead of staying inside the pane. The current implementation
intends `.readonly-text-content` to own scrolling, and the tab polish design
tweak made editor panes fill their Dockview area, but no browser evidence proves
long-file wheel containment.

Second, terminal input fidelity still does not match a native terminal. Korean
IME input does not work in the browser terminal, and shell line-editing control
keys such as `ctrl-u` (clear line) and `ctrl-w` (delete previous word) do not
behave as they do in a native shell. A hotfix added composition guards to the
terminal window keydown fallback, but that is only a plausibility fix. IME
composition and shell control-key behavior should be verified against the live
xterm/WebSocket path and should not silently rely on ASCII keypress tests.

## Decisions

- Keep the already-applied hotfix in the tab polish branch: tab labels are
  icon-plus-title, and terminal fallback now ignores active composition.
- Do not fold this work into a richer editor replacement. Dedicated editor
  technology may arrive later, but current read-only pane containment still
  needs a regression check.
- Do not treat Playwright ASCII keyboard tests as IME or native terminal
  line-editing evidence. If browser automation cannot synthesize a real Korean
  IME composition path, record a manual verification artifact and keep the
  automated guard coverage explicit.
- Treat native terminal behavior as the oracle for ordinary shell line editing:
  `ctrl-u` clears the current command line, `ctrl-w` deletes the previous word,
  and those controls must not be swallowed by browser focus handling,
  Dockview-level shortcuts, or the window keydown fallback.

## Phases

### Phase 1: Prove read-only editor scroll containment

Add deterministic browser evidence for a long read-only file opened in the
workbench. Wheel or scroll interaction over the editor content should scroll
the internal read-only content region and should not move the top-level browser
document or push dashboard chrome out of view.

Success means the browser gate covers long read-only file scroll containment,
including the Dockview editor pane after tab polish styling. If current behavior
is broken, fix only the read-only pane containment chain and avoid introducing a
new editor library in this ticket.

### Result (1c1fa1b) - 2026-05-17

Implemented the scroll containment fix by bounding editor panes within their
Dockview panel host so `.readonly-text-content` becomes the vertical scroll
owner for long files. The browser acceptance gate now opens a long read-only
fixture, proves the internal scroll position changes, and verifies the
top-level document does not scroll.

### Phase 2: Verify terminal IME composition and shell control keys

Validate terminal input against the live xterm path. The terminal should allow
composed Korean input to reach the shell after composition commits, and the
window keydown fallback must not send intermediate composition keystrokes as raw
input. Native shell line-editing controls such as `ctrl-u` and `ctrl-w` should
reach the PTY and produce shell-visible effects. The fallback should remain
available only for the limited shortcuts it was added to preserve and must not
replace xterm's normal input/composition handling for focused terminal text.

Success means the ticket records either automated browser evidence for
composition behavior or a clearly scoped manual verification artifact when
Playwright cannot drive the platform IME. Tests should include fallback guard
coverage and shell-visible `ctrl-u` / `ctrl-w` behavior so future changes do not
reintroduce raw composition forwarding or swallowed line-editing controls.

### Result (9dadeab) - 2026-05-17

Implemented terminal input fidelity coverage and fixes for the live browser
terminal path. The terminal fallback now mirrors xterm-style raw bytes for
`ctrl-u` and `ctrl-w` when Dockview focus leaves the helper textarea, while
composition-in-progress fallback keydown events remain ignored. Browser
evidence now proves shell-visible `ctrl-u` and `ctrl-w` behavior, WebSocket
input frames for those controls, committed Hangul text reaching the shell, and
synthetic IME composition guard behavior.
