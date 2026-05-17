---
title: ws dashboard editor scroll and IME verification follow-up
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260517-feat-ws-dashboard-workbench-tab-polish: introduced Dockview tab polish and the hotfix that added an IME fallback guard
  260516-epic-ws-web-dashboard-workbench-substrate: owns workbench pane lifecycle, focus, and browser evidence policy
spec:
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
  - 260516-ws-web-dashboard-readonly-text-pane
  - 260516-ws-web-dashboard-browser-terminal-emulator-behavior
  - 260516-ws-web-dashboard-terminal-websocket-input-fidelity
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard editor scroll and IME verification follow-up

## Background

After the workbench tab polish implementation, two follow-up risks remain that
should not be buried inside the completed tab polish ticket.

First, read-only editor pane scrolling may still propagate to the top-level
browser document instead of staying inside the pane. The current implementation
intends `.readonly-text-content` to own scrolling, and the tab polish design
tweak made editor panes fill their Dockview area, but no browser evidence proves
long-file wheel containment.

Second, Korean IME input in terminal panes needs explicit verification. A
hotfix added composition guards to the terminal window keydown fallback, but
that is only a plausibility fix. IME composition behavior should be verified
against the live xterm/WebSocket path and should not silently rely on ASCII
keypress tests.

## Decisions

- Keep the already-applied hotfix in the tab polish branch: tab labels are
  icon-plus-title, and terminal fallback now ignores active composition.
- Do not fold this work into a richer editor replacement. Dedicated editor
  technology may arrive later, but current read-only pane containment still
  needs a regression check.
- Do not treat Playwright ASCII keyboard tests as IME evidence. If browser
  automation cannot synthesize a real Korean IME composition path, record a
  manual verification artifact and keep the automated guard coverage explicit.

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

### Phase 2: Verify terminal IME composition behavior

Validate terminal IME input against the live xterm path. The terminal should
allow composed Korean input to reach the shell after composition commits, and
the window keydown fallback must not send intermediate composition keystrokes as
raw input. The fallback should remain available for the limited shortcuts it was
added to preserve.

Success means the ticket records either automated browser evidence for
composition behavior or a clearly scoped manual verification artifact when
Playwright cannot drive the platform IME. Tests should include the fallback
guard behavior so future changes do not reintroduce raw composition forwarding.
