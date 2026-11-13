# Hosted Claude: slash-command completion in a just-created empty session

## Problem

1d274570f (merged on develop, daemon rebuilt from it at ~10:38 local) made a
hosted Claude lead send the `initialize` control request at launch and map
its `commands` answer to the session's command list, plus `swap()` re-syncs
the slash picker when the `cmds` region lands. Owner live check 2026-11-13
(board (28) = no): in a freshly created, empty hosted session (no message
sent yet), typing `/` in the composer still shows no completion.

## Outcome

- Diagnose with live evidence where the chain breaks for a session created
  after the rebuild. Read-only evidence allowed: the newest session dirs under
  `C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\` (registration,
  agent.json, snapshot.json `commands`, ring), the holder/daemon logs, and
  GETs against `http://127.0.0.1:29417` (no POSTs, no session actions). Check
  at least: is the hosted lead actually run by the rebuilt code (holder
  process binary vs. daemon binary: does `dev.py rerun daemon` replace the
  binary the holder/lead runner uses?), was `initialize` sent and answered,
  did the answer reach the snapshot `commands`, is the `cmds` region emitted
  for this viewer before the first turn, does the picker open.
- Fix the cause; state it in `## AI Context`. If the cause is that holders
  keep running an older binary until restarted (not a code bug), report that
  instead of changing code, with what the owner must restart.
- A test that would have caught it.

## Out of scope

- Restarting the daemon or sessions (lead does the live check). You may
  create NO sessions on the live daemon. A throwaway local `claude -p`
  stream-json probe outside the dashboard is fine (cheapest model, scratch dir
  under D:/Temp/hosted-slash/, env CLAUDE*/WS_* stripped).

## Verification

- `go test` for touched packages on w5090 (clone note `remote-build-w5090`);
  node-backed dashboard JS tests run locally (node at C:/Program Files/nodejs,
  Go at C:/Program Files/Go/bin) under the local lock, confirming they ran.
