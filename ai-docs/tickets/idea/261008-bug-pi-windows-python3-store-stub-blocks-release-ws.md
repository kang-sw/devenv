---
title: Release ws-on-Pi fails on Windows when python3 is the Microsoft Store stub
related:
  261008-bug-windows-mailbox-wait-outlives-pi-host: discovered during its native-Windows probe
---

# Release ws-on-Pi fails on Windows when python3 is the Microsoft Store stub

## Background

During the 2026-10-08 native-Windows probe for
261008-bug-windows-mailbox-wait-outlives-pi-host (host DESKTOP-3SUVUA9, Pi
1.0.4, `pi install git:github.com/kang-sw/devenv@v0.46.34`), Pi with the
release ws package failed at startup with "session bootstrap failed ...
code=9009".

- On that host `python3` resolves to
  `%LOCALAPPDATA%\Microsoft\WindowsApps\python3.exe`, the Store App Execution
  Alias stub, which exits 9009 when the Store Python is not installed.
- A stock python.org install (`AppData\Local\Programs\Python\Python312\`) ships
  `python.exe` and the `py.exe` launcher but no `python3.exe`, so the stub wins
  the PATH lookup.
- The Pi adapter hard-codes `python3` for the launcher: `spawn` in
  `agents-plugin-pi/src/mailbox-waiter.ts`, `McpStdioClient("python3", ...)` in
  `agents-plugin-pi/src/mcp-stdio-client.ts`, and `execFile("python3", ...)` in
  `agents-plugin-pi/src/bridge.ts`.
- The probe worked around it with a hardlink named `python3.exe` to the real
  `python.exe` placed first on PATH; nothing on the host was changed globally.

Consequence: a downstream Windows user with only a python.org install cannot
run release ws-on-Pi at all.

## Open questions

- Interpreter resolution order on Windows (`python3`, `python`, `py -3`) and
  how to detect the Store stub (exit 9009 or the WindowsApps path).
- `py.exe` adds a process layer (`py.exe -> python.exe`); the probe infers it
  would orphan `python.exe` as well as ws-mcp under the current kill path, so
  this choice interacts with 261008-bug-windows-mailbox-wait-outlives-pi-host.
- Whether the Claude/Codex `.mcp.json` launch shape has the same exposure.
