---
title: ws dashboard Windows terminal control keys
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-bug-ws-web-terminal-cross-platform-portability: native Windows dogfood reached a real cmd.exe terminal but Ctrl-C did not interrupt the long-running command fixture
spec:
  - 260516-ws-web-dashboard-terminal-websocket-input-fidelity
  - 260516-ws-web-dashboard-browser-terminal-emulator-behavior
  - 260516-ws-web-dashboard-terminal-platform-command-helpers
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard Windows terminal control keys

## Background

Native-Windows fixed-endpoint dogfood on 2026-05-17 reached a real dashboard
terminal backed by `cmd.exe` through SSH local forwarding. Owner pairing,
daemon-host workRoot opening, basic echo, Backspace, cursor movement, and
history-visible command text all reached the browser gate, but the Ctrl-C
acceptance step failed.

The browser sent Ctrl-C after starting the `cmd.exe` long-running fixture
(`ping -n 30 127.0.0.1 > nul`). The terminal stayed in that command and the
follow-up `CTRL-C-OK` echo was not observed. Increasing the fixture startup
wait did not change the result. This leaves native-Windows terminal
control-key behavior unevidenced even though the local POSIX browser gate
passes the same acceptance step.

## Notes

- The remote Cargo/toolchain blocker from the portability ticket is resolved on
  the machine-local Windows host.
- The remaining gap is not endpoint readiness, pairing, workRoot path
  translation, or shell-profile selection; the gate reached a live `cmd.exe`
  PTY.
- Investigation should distinguish browser/xterm key mapping, WebSocket input
  framing, daemon input forwarding, `portable_pty` Windows behavior, and
  `cmd.exe`/child-process Ctrl-C semantics.
- If native Windows requires a different interrupt mechanism than raw ETX
  bytes, the terminal contract and command-plan limitations should say that
  explicitly instead of treating POSIX Ctrl-C evidence as portable.
