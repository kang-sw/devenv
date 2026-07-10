# Plan: 260707-bug-dashboard-windows-daemon-state-persistence-silently-noop — Phase 2: Daemon-level HTTP repro of the Windows fallback fix

## Relevant Ticket Contract
- Launch a real native-Windows daemon (WSL2 interop path per
  `ai-docs/_index.local.md` is explicitly sufficient) with `$env:HOME` unset
  and no `WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME`/`XDG_STATE_HOME`
  set — only `LOCALAPPDATA` present (stock Windows default).
- `POST /api/dashboard/root-picker/pins` against that daemon; confirm a real
  `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json` file appears on disk
  (not just a `200`), and a follow-up read reflects the persisted pin.
- Confirm the one-time `tracing::warn!` in `default_local()` does **not** fire
  in this run (a resolvable state file exists) — checks the warning path
  doesn't false-positive on success.
- Tear down: stop the daemon, remove any scratch worktree/state file created
  solely for this repro.
- Record outcome as an **Edition under Phase 1** (not a new Phase 2 Result)
  if it simply confirms the existing fix; open a new bug ticket instead if a
  genuine behavior gap surfaces.
- Full cross-machine reversed-topology walk (resources, work-roots/open,
  files, Git, terminals) is explicitly **out of scope** for this phase — that
  belongs to the sibling ticket
  `260707-chore-dashboard-linked-server-tunnel-dogfood-plan` and is not
  required here.

## Out of Scope
- Any change to `default_state_file()`, `DashboardStateStore`, or the
  `tracing::warn!` call — Phase 1 already implemented and unit-tested the
  fix; this phase is verification-only, no source edits.
- The sibling ticket's full forwarded-operation / linked-server / terminal
  WebSocket-relay walk.
- Editing the frozen Phase 1 `### Result` text in place — new findings go in
  an `#### Edition (<short-hash>) - YYYY-MM-DD` block appended after it, per
  `AGENTS.md` ticket conventions.

## Codebase Findings
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L485-L503` — fixed
  `default_state_file()`: order is `WS_DASHBOARD_STATE_FILE` ->
  `WS_DASHBOARD_STATE_HOME` -> `XDG_STATE_HOME` -> `HOME` ->
  `#[cfg(windows)] LOCALAPPDATA` -> `None`. Windows branch yields
  `<LOCALAPPDATA>\ws-dashboard\opened-workroots.json`.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L30-L42` —
  `default_local()` calls `default_state_file()`; on `None` it emits
  `tracing::warn!("no dashboard state file could be resolved ...")` once at
  construction and falls back to `Self::disabled()`. With `LOCALAPPDATA` set,
  this branch is not taken — repro must confirm this warn line is absent
  from the daemon's log output.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L433-L449` —
  `write_state_json` creates the parent directory via `fs::create_dir_all`
  before writing (write-to-temp + rename). No manual `mkdir` of
  `%LOCALAPPDATA%\ws-dashboard\` is needed before the `POST`.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L145-L172` —
  `pin_root_picker_directory` handler: takes `Json(RootPickerPinRequest)`,
  loads existing pins, appends + dedups, calls
  `state.dashboard_state.persist_root_picker_pins(pins)`, returns
  `RootPickerPlacesView { places }` on success or `500` on persist failure.
- `ws-dashboard/crates/daemon/src/router.rs#L230-L237` — routes are direct
  (non-server-scoped) local endpoints: `GET /api/dashboard/root-picker`,
  `POST/DELETE /api/dashboard/root-picker/pins`. No server-link indirection
  needed for this repro (the linked-server 404 bug is a separate concern
  already root-caused to the same underlying `state_file: None`).
- `RootPickerPinRequest` body shape: `{"path": "<absolute path>"}` (confirmed
  via `clean_pin_path(&request.path)` in `root_picker.rs#L149`).
- `ws-dashboard/crates/daemon/src/cli.rs#L24-L48` — `ServeArgs`: `--host`
  (default `127.0.0.1`), `--bind-mode` (default `Local`), `--no-auth` (loopback
  debug bypass), `--port` (default `0` = random ephemeral; pass an explicit
  port for a scriptable repro), `--static-dir`. `--no-auth` on a loopback bind
  is the low-friction choice for this repro (no pairing-cookie dance) and is
  explicitly permitted for `--bind-mode local` (`config.rs` only forbids
  `--no-auth` with `--bind-mode public`).
- `ai-docs/_index.local.md#L280-L328` ("Run log: 2026-07-08") — proven, reusable
  WSL2-interop recipe for driving native Windows `cargo`/binaries from this
  session without touching `D:\dbg-ws-dashboard-dev`'s primary checkout:
  1. `command git fetch /home/swkang/devenv/.worktree/ws-dashboard-dev
     ws-dashboard-dev` from inside `/mnt/d/dbg-ws-dashboard-dev` (use
     `command git`, not the `/mnt/*`-redirected `git` shell function).
  2. `command git worktree add /mnt/d/<scratch-name> FETCH_HEAD` (detached)
     off `D:\dbg-ws-dashboard-dev`.
  3. Build: `powershell.exe -NoProfile -Command "cd D:\<scratch-name>\ws-dashboard;
     cargo build -p ws-dashboard-daemon"` — binary lands at
     `target\debug\ws-dashboard.exe`.
  4. Launch via PowerShell `Start-Process ... -PassThru | Select-Object
     -ExpandProperty Id` to get a killable PID back through WSL interop (a
     bare foreground `-Command` call backgrounds silently under the `Bash`
     tool; read the PID from the task output after a short pause).
  5. `command git worktree remove /mnt/d/<scratch-name> --force` for teardown.
- `ai-docs/_index.local.md#L1-L27` — environment shape: WSL2 sandbox, Windows
  host reachable, `powershell.exe` interop path confirmed working for
  `cargo`, native checkout at `D:\dbg-ws-dashboard-dev` /
  `/mnt/d/dbg-ws-dashboard-dev`.
- `ai-docs/tickets/ready/260707-bug-dashboard-windows-daemon-state-persistence-silently-noop.md#L131-L186`
  — Phase 1 `### Result` is already frozen (has a `### Result` section); any
  new finding from this phase must go in an `#### Edition (<short-hash>) -
  2026-07-10` block appended after it, not an in-place edit, and not a new
  `### Result` under Phase 2 itself.

## Implementation Plan
1. Re-derive environment facts (do not reuse stale values): confirm
   `/mnt/d/dbg-ws-dashboard-dev` is reachable and check its `git status` for
   uncommitted state before touching it.
2. From `/mnt/d/dbg-ws-dashboard-dev`, `command git fetch
   /home/swkang/devenv/.worktree/ws-dashboard-dev ws-dashboard-dev`, then
   `command git worktree add /mnt/d/<scratch-name> FETCH_HEAD` (pick a fresh
   disposable scratch dir name).
3. Build the daemon on the real Windows target: `powershell.exe -NoProfile
   -Command "cd D:\<scratch-name>\ws-dashboard; cargo build -p
   ws-dashboard-daemon"`.
4. Launch the daemon via `powershell.exe` with a clean env for this run:
   unset `HOME`/`WS_DASHBOARD_STATE_FILE`/`WS_DASHBOARD_STATE_HOME`/
   `XDG_STATE_HOME` for the launched process (PowerShell child-process env,
   not the interop shell's ambient env — verify with `$env:HOME` /
   `$env:LOCALAPPDATA` printed just before launch to confirm only
   `LOCALAPPDATA` is present), `--no-auth --bind-mode local --host
   127.0.0.1 --port <fixed-port>`. Use `Start-Process ... -PassThru |
   Select-Object -ExpandProperty Id` to capture a killable PID; redirect
   stdout/stderr to a log file for the warn-absence check in step 6.
5. `curl -sS -X POST http://127.0.0.1:<port>/api/dashboard/root-picker/pins
   -H 'Content-Type: application/json' -d
   '{"path":"C:\\Users\\<user>\\Desktop"}'` (or any real existing directory
   on the Windows box) from WSL against the Windows-host-facing address, or
   run the `curl` itself through `powershell.exe` if WSL->Windows-loopback
   reachability is a problem — confirm `200` plus a `places` array containing
   the pinned path.
6. Verify on disk via `powershell.exe -NoProfile -Command "Get-Content
   \"$env:LOCALAPPDATA\ws-dashboard\opened-workroots.json\""` (or
   `Test-Path` first) — confirm the file exists and its `rootPickerPins`
   entry matches the pinned path.
7. `GET /api/dashboard/root-picker` (or `/pins`-equivalent read path) to
   confirm the persisted pin round-trips into a fresh read, not just the
   `POST` response.
8. Inspect the daemon's captured log output for the exact
   `"no dashboard state file could be resolved"` warn string
   (`persistent_state.rs#L34-L38`) — confirm it is **absent** (state file
   resolved successfully via `LOCALAPPDATA`).
9. Teardown: stop the daemon process (`Stop-Process -Id <pid> -Force`,
   followed by a separate `Get-Process` existence check per the documented
   caveat that a piped `Stop-Process` in the same invocation may not confirm
   termination), delete the scratch
   `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json` (and its directory if
   otherwise empty) created solely for this repro, then `command git
   worktree remove /mnt/d/<scratch-name> --force`.
10. Record the outcome: append an `#### Edition (<short-hash>) - 2026-07-10`
    block after Phase 1's `### Result` in the ticket (do not edit the frozen
    Result text), stating the daemon-level HTTP repro was executed and its
    outcome (file created, pin round-tripped on read, warn absent) — this
    closes the "Still not done" gap Phase 1's Result explicitly left open.
    Do not add a `### Result` under Phase 2 itself per the ticket's own
    instruction. If the repro instead surfaces a genuine behavior gap, stop
    and open a new bug ticket instead of forcing a positive Edition.

## Verification Plan
- The repro itself **is** the verification for this phase: `200` from the
  `POST`, on-disk file existence + correct `rootPickerPins` content at
  `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json`, a follow-up `GET` that
  reflects the pin, and absence of the `default_local()` warn string in the
  daemon's log.
- No `cargo test`/`cargo build` re-run is required in this phase (Phase 1
  already covered unit tests and native-Windows compilation); this phase is
  a live-process HTTP/filesystem check only.
- Confirm daemon process and scratch worktree are both gone after teardown
  (`Get-Process` miss on Windows side; `git worktree list` clean on the
  `/mnt/d/dbg-ws-dashboard-dev` side) before recording the outcome.

## Escalations
- None.
