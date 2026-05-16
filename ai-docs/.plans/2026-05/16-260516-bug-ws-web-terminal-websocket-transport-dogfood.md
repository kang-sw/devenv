# Dogfood evidence: 260516 terminal WebSocket transport

Command: `cd ws-dashboard/frontend && npm run test:browser`

Result: pass.

Viewports:
- Default Playwright viewport: 1440x900.
- Narrow relayout viewport: 480x900.

Observations recorded by the browser gate:
- Owner pairing succeeded through the daemon-served production frontend.
- A real temporary workRoot was opened; no mock terminal surface existed before terminal creation.
- Terminal attached to `/api/dashboard/terminals/{terminal_id}/socket` with a browser WebSocket.
- HTTP `/output` polling stayed stopped while the WebSocket was connected.
- Keystroke echo completed under the asserted 2000ms responsiveness bound.
- Input fidelity covered ordinary prompt input/editing, Backspace, left/right cursor movement, shell history navigation, Ctrl-C, Ctrl-D before explicit close, Ctrl-L, and paste.
- ANSI SGR output rendered through xterm color spans rather than raw escape text.
- Resize forwarding updated the daemon-confirmed terminal columns on 480x900 relayout.
- Explicit terminal close removed the tab and preserved daemon-owned lifecycle semantics.
- Reload reconstructed the surviving daemon terminal without mock surfaces.

Generated artifacts (ignored, outside tracked source):
- `ws-dashboard/frontend/e2e/.artifacts/evidence.txt`
- `ws-dashboard/frontend/e2e/.artifacts/file-explorer.png`
- `ws-dashboard/frontend/e2e/.artifacts/terminal-emulator.png`
- `ws-dashboard/frontend/e2e/.artifacts/desktop-workbench.png`
- `ws-dashboard/frontend/e2e/.artifacts/narrow-workbench.png`

Latest observed evidence file summary:
- WebSocket connected to the protected terminal socket route.
- Output polls remained at the same count while connected.
- Input echo timing was below the merge-gated responsiveness bound.
