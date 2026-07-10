# Plan: 260707-chore-dashboard-linked-server-tunnel-dogfood-plan — Phase 2: Re-walk the reversed-topology leg now that the state-persistence bug has a fix

## Relevant Ticket Contract
- Launch the reversed topology from Phase 1 step 3 again: **Windows-hosted
  gateway daemon** (normal owner-auth, default `local` bind mode) linking a
  **WSL-hosted remote daemon** bound to `127.0.0.1` (default `local` bind
  mode, normal owner-auth), added via the direct-endpoint field as
  `http://localhost:<port>`.
- Confirm the link handshake still succeeds (`200 connected`), then walk the
  **full** forwarded-operation set that Phase 1 step 3 never reached because
  of the 404 bug: resources listing, root picker, work-roots/open, file
  read/write, Git status/branches, terminal create, and the terminal
  WebSocket relay (typed input round-trips).
- Confirm none of the above 404 the way the original Phase 1 attempt did —
  this is the direct end-to-end confirmation that the state-persistence fix
  resolves the originally discovered symptom, not just the unit-test/HTTP-
  repro-level verification already recorded on
  `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`.
- Record the outcome as a further dated append to
  `260525-feat-ws-dashboard-server-scoped-operation-forwarding` (same target
  as Phase 1 step 4; do not edit its frozen `### Result` text in place).
- If anything beyond the already-fixed 404 symptom surfaces, file it as its
  own ticket rather than folding a code fix into this verification-only
  ticket.
- Tear down all test artifacts per Phase 1 step 6's discipline (kill both
  daemons, remove linked-server entries/state created solely for this run).
- Explicitly out of scope for this phase: SSH connectivity and the
  SSH-tunnel leg (Phase 1 steps 1-2) — this phase covers only Phase 1 step
  3's leg.
- Depends only on the code fix being present on `ws-dashboard-dev` (already
  merged); does not depend on
  `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`'s own
  closure and can proceed in parallel with it.
- Constraint (from ticket header, still binding): never weaken
  `entrypoint_headers_allowed`/`is_allowed_host` or any Host/Origin/bind-mode
  invariant. Never echo/persist pairing tokens, link passphrases, or bearer
  tokens — capture into a shell variable and consume in the same command
  chain.

## Out of Scope
- Phase 1 steps 1-2 (SSH connectivity probe, SSH-tunnel leg) — still blocked
  on the open harness-classifier escalation; not touched by this phase.
- Any code change to `entrypoint_headers_allowed`/`is_allowed_host`,
  `default_state_file`, or `DashboardStateStore` — the fix is already merged
  and this phase is verification-only.
- The `LinkedServerModal` UI gap (no `ssh_target` field) — irrelevant here
  since this leg uses the direct-endpoint field, which the UI already
  supports, though this phase still drives it via `curl` for consistency
  with Phase 1's own methodology and to keep raw response bodies inspectable.
- Filing/implementing a fix for any new bug found — file a ticket instead
  (same rule as Phase 1 step 5).

## Codebase Findings
- `ws-dashboard/crates/daemon/src/router.rs:93` — `POST
  /api/dashboard/servers/link`, handler `link_endpoint_server`
  (`servers.rs:280-326`). Body (`EndpointLinkedServerRequest`,
  `servers.rs:1936-1942`, camelCase): `serverId`, `label`, `endpoint`,
  optional `passphrase`. Use `endpoint: "http://localhost:<wsl-remote-port>"`
  per the ticket's reversed-topology step. Persists via
  `persist_linked_server` (`servers.rs:322`) — this write path is exactly
  what `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`
  fixed for the Windows-gateway side.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L485-L503` — fixed
  `default_state_file()` order: `WS_DASHBOARD_STATE_FILE` ->
  `WS_DASHBOARD_STATE_HOME` -> `XDG_STATE_HOME` -> `HOME` ->
  `#[cfg(windows)] LOCALAPPDATA` -> `None`. The Windows gateway daemon in
  this phase must resolve a real state file (either via `LOCALAPPDATA` with
  a stock env, or an explicit `WS_DASHBOARD_STATE_HOME` scratch override) for
  the link to actually persist across the forwarded calls — this is the
  exact condition the original 404 bug violated.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L30-L42` —
  `default_local()` emits a one-time `tracing::warn!("no dashboard state
  file could be resolved ...")` when resolution fails to `None`. If this
  fires in the Windows gateway's log during this run, the state-file fix is
  not actually in effect for this build — treat as a blocking signal to
  re-check the build/env before continuing the walk.
- `ws-dashboard/crates/daemon/src/router.rs:94-215` — forwarded-operation
  routes under `/api/dashboard/servers/{server_route}/...`: `resources`
  (94-97), `root-picker` + `/directories` + `/pins` (106-117),
  `work-roots/open` (118-121), `work-roots/{id}/files` + `/files/read` +
  `/files/write` (126-137), `git/status` + `/git/branches` (170-177),
  `work-roots/{id}/terminals` (194-197, create), `terminals/{id}/socket`
  (202-205, WS relay). This is the exact "full forwarded-operation set" the
  phase must walk.
- `ws-dashboard/crates/daemon/src/servers.rs:1291-1345` /
  `1346-1416` — `connect_remote_terminal_websocket` /
  `terminal_websocket_relay`: WS relay goes through the same owner-auth +
  `entrypoint_headers_allowed` gate (special-cased for `Upgrade: websocket`
  in `router.rs:393-401`) — the WS test needs the same owner cookie as the
  HTTP calls, not a bare token.
- `ws-dashboard/crates/daemon/src/auth.rs:274-277,308-335` —
  `entrypoint_headers_allowed`/`is_allowed_host`: pass only
  `localhost`/loopback on `Host` (and `Origin` if present). Confirms
  `http://localhost:<port>` is exactly the case this check is designed to
  allow for the WSL remote (bound `127.0.0.1`) — unchanged by this ticket's
  scope; do not modify.
- `ws-dashboard/crates/daemon/src/cli.rs:23-49,51-60` — `ServeArgs`: both
  daemons use default `local` bind mode + normal owner-auth (no
  `--bind-mode public`, no `--no-auth`) per the ticket's explicit
  instruction; `--host 127.0.0.1 --port <fixed-or-0>` for both.
- `ai-docs/tickets/ready/260707-chore-dashboard-linked-server-tunnel-dogfood-plan.md`
  Phase 1 `### Result` — records the exact prior failure this phase re-walks
  past: link handshake `200 connected`, then every forwarded call 404'd
  ("unknown server") due to the now-fixed state-persistence bug. This
  phase's success criterion is precisely the absence of that same 404
  pattern.
- `ai-docs/tickets/.done/260707-bug-dashboard-windows-daemon-state-persistence-silently-noop.md`
  Phase 1 Result + `#### Edition (55dfb0a6) - 2026-07-10` — confirms the fix
  is merged and already verified end-to-end at the single-daemon
  root-picker-pin level on a real native Windows daemon (build via
  `D:\dbg-ws-dashboard-dev` + WSL2 interop). This phase is the next step up:
  the same fix, exercised through the linked-server forwarding path with two
  daemons instead of one.
- `ai-docs/_index.local.md#L280-L328` ("Run log: 2026-07-08") — reusable
  WSL2-interop recipe for building/running the Windows-native daemon without
  touching `D:\dbg-ws-dashboard-dev`'s primary checkout:
  1. From `/mnt/d/dbg-ws-dashboard-dev`: `command git fetch
     /home/swkang/devenv/.worktree/ws-dashboard-dev ws-dashboard-dev` (use
     `command git`, not the `/mnt/*`-redirected `git` shell function, to
     avoid the UNC-path `origin` problem).
  2. `command git worktree add /mnt/d/<scratch-name> FETCH_HEAD` (detached).
  3. `powershell.exe -NoProfile -Command "cd D:\<scratch-name>\ws-dashboard;
     cargo build -p ws-dashboard-daemon"` — binary at
     `target\debug\ws-dashboard.exe`.
  4. Launch via `Start-Process ... -PassThru | Select-Object
     -ExpandProperty Id` to capture a killable PID (a bare `-Command` call
     backgrounds silently under the Bash tool; read the PID from the task
     output after a short pause).
  5. `command git worktree remove /mnt/d/<scratch-name> --force` for
     teardown.
- `ai-docs/_index.local.md#L116-L198` ("Run log: 2026-07-07 execution") —
  documents the credential-capture pattern
  (`RESP=$(curl ...); TOKEN=$(...); curl ... -H "Authorization: Bearer
  $TOKEN"; unset TOKEN RESP`), the pairing-cookie flow (`GET
  /pair?token=<token>` -> `Set-Cookie`), and that this is the *first* time
  the reversed-topology leg is walked past the link step — no prior full
  forwarded-op-set run to diff against, only the 404 failure this phase must
  now not reproduce.
- **Risk signal — no linked-server removal API**: no `DELETE` route targets
  `/api/dashboard/servers/{server_route}` (checked `router.rs`'s `DELETE`
  routes). Teardown of the linked-server entry created on the Windows
  gateway's persisted state requires either (a) pointing the gateway at a
  scratch `WS_DASHBOARD_STATE_HOME`/`WS_DASHBOARD_STATE_FILE` for this run so
  teardown is just deleting that scratch path, or (b) editing the real state
  file's `linkedServers` array afterward. Recommend (a) for both daemons in
  this run, consistent with Phase 1's own plan.

## Implementation Plan
1. Pick a scratch state override for both daemons this run (e.g.
   `WS_DASHBOARD_STATE_HOME=/tmp/dogfood-reversed-rewalk` on WSL,
   `$env:WS_DASHBOARD_STATE_HOME="D:\dogfood-reversed-rewalk"` for the
   Windows process) so teardown is a plain directory delete and no
   pre-existing persisted state is touched.
2. Launch the WSL remote daemon: `ws-dashboard serve --host 127.0.0.1
   --bind-mode local --port <fixed-or-0>` with the scratch state env set;
   capture its printed pairing URL / link passphrase into shell variables
   without echoing.
3. Bring `/mnt/d/dbg-ws-dashboard-dev` current and build the Windows-native
   gateway daemon per the `ai-docs/_index.local.md#L280-L328` recipe above
   (fetch via Linux path, disposable detached worktree, `cargo build -p
   ws-dashboard-daemon` through `powershell.exe`).
4. Launch the Windows gateway daemon via `powershell.exe`
   (`Start-Process ... -PassThru`) with the scratch state env set, `--host
   127.0.0.1 --bind-mode local --port <fixed-or-0>`, normal owner-auth.
   Capture the PID.
5. Pair with the Windows gateway: `GET /pair?token=<gateway pairing token>`,
   capture `Set-Cookie` into `$COOKIE`.
6. `POST /api/dashboard/servers/link` on the Windows gateway with `Cookie:
   $COOKIE`, body `{"serverId": "...", "label": "...", "endpoint":
   "http://localhost:<wsl-remote-port>", "passphrase": "<remote's link
   passphrase>"}`. Confirm `200` with `"status":"connected"` — this repeats
   Phase 1 step 3's already-successful handshake; it is not the new ground
   this phase covers.
7. Immediately check the Windows gateway's daemon log for the
   `"no dashboard state file could be resolved"` warn string
   (`persistent_state.rs#L34-L38`); it must be **absent**. If present, stop
   and re-check the scratch state env before proceeding — the rest of the
   walk would be meaningless without a working state file.
8. Walk the full forwarded-operation set against the linked server, all with
   `Cookie: $COOKIE` against the Windows gateway (`router.rs:94-215`):
   `GET .../resources`, `GET .../root-picker`, `POST .../work-roots/open`,
   `GET`/`POST .../files` + `/files/read` + `/files/write`,
   `GET .../git/status` + `/git/branches`, `POST .../terminals` (create),
   then the WS relay at `GET .../terminals/{id}/socket` with the same
   `Cookie` header — send typed input, confirm output round-trips. For each
   call, confirm the response is a normal success/data response, not a `404
   "unknown server"` (the exact symptom this phase re-walks past).
9. If any call still 404s the same way, or a different failure appears,
   treat as a genuine new finding — do not attempt a code fix; file a new
   ticket per step 5 of Phase 1's own rule.
10. Record the outcome as a further dated append to
    `ai-docs/tickets/ready/260525-feat-ws-dashboard-server-scoped-operation-forwarding.md`
    (mirroring the existing dated-append style already used for Phase 1's
    finding; do not edit any frozen `### Result` text in place) — state
    explicitly that the reversed-topology leg's full forwarded-operation set
    was walked, that none of it reproduced the original 404, and reference
    the fixed bug ticket by stem.
11. Teardown: stop both daemon processes (`kill <pid>` on WSL,
    `Stop-Process -Id <pid> -Force` via `powershell.exe`, followed by a
    separate `Get-Process` existence check — a piped `Stop-Process` result
    in the same invocation is not reliable per prior run notes), remove both
    scratch state directories, and `command git worktree remove
    /mnt/d/<scratch-name> --force`.

## Verification Plan
- Manual-only verification: this phase's success criteria are the curl/WS
  responses observed during the walk — `200 connected` on link, `200`/normal
  data on every forwarded call (resources, root-picker, work-roots/open,
  files read/write, git status/branches, terminal create), and a working
  input/output round-trip on the terminal WebSocket. No automated test suite
  exists or should be added for this phase.
- Explicitly confirm the absence of both known failure modes before
  declaring success: the original `404 "unknown server"` symptom (state-file
  bug) and the already-settled `403` Host-check non-bug (would only appear
  if `--bind-mode public` were mistakenly used — this plan does not use it).
- Credential-handling check throughout: never let a pairing token, link
  passphrase, or session cookie value appear in a file, this session's final
  report, or an echoed shell command — capture-and-consume in the same
  command chain per `ai-docs/_index.local.md`'s documented pattern.
- Confirm both daemon processes and the scratch worktree are gone after
  teardown before recording the outcome as final.

## Escalations
- None.
