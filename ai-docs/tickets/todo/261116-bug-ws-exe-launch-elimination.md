---
title: Eliminate ws executable launches outside new-daemon start
related:
  261115-feat-werkschaft-electron-daemon-override: Electron new-daemon start is one of the two permitted launch points
---

# Eliminate ws executable launches outside new-daemon start

## Background

The owner's Windows PC runs AhnLab V3, which flags repeated executable
launches. ws was moved to a daemon to avoid this, but V3 still fires dozens
of times per daemon swap and periodically ("MCP launch") while only hosted
Claude / Pi sessions are in use.

Owner contract, 2026-11-16 09:48 (verbatim, binding):

> 이 exe 실행 지점 없애는 걸 최우선으로 삼죠. 오로지 런쳐에서 새 데몬 띄울 때+
> (이미 살아있는 데몬 버전 체크도 REST API로만 하고 실행 파일 실행으로 검증 하지 않음)
> electron이 새 데몬 띄울 때 외에는 exe 실행 아예 없어야 합니다.

Meaning: a ws executable is executed only when (a) the launcher must start a
NEW daemon because none is alive, or (b) the Electron app must start a NEW
daemon. A live daemon's version / compatibility is checked only through its
REST API, never by executing a binary. Everything else talks to the daemon
over the network. Priority: top.

### Live trace evidence (2026-11-16 09:48-09:53 KST)

Sampled `Win32_Process` every ~250 ms (process-start trace was access-denied);
CSV at `D:/Temp/exe-spawn-trace/spawns-compact.csv` (machine-local, deleted
after this ticket is promoted).

- Steady state, 3.1 min window: zero daemon-start, zero new
  `ws-mcp-dashboard-*.exe`, zero new launcher / holder processes, zero
  launches from Claude hooks or the Pi mailbox/wake code.
- The V3 hits at 09:49-09:53 were a hosted Pi session (model-initiated)
  running `agents-plugin-pi/.runtime/windows-amd64/ws-mcp-0.50.1-<hash>.exe
  call tickets.create_empty ...` from its bash tool, including python loops
  that launched the exe once per call. Chain: daemon -> holder python ->
  pi node -> bash -> python -> ws-mcp exe. Cause: `tickets.create_empty` is
  not in that Pi worker's tool profile, so the model fell back to the exe CLI.
- V3 `AnalysisRpt.exe` ran at 09:44:38 against a newly staged
  `dashboard\bin\ws-mcp-dashboard-<sha16>.exe` right after the 09:43:35 swap:
  every distinct build stages a new unique path, which V3 treats as an unknown
  file. `dashboard\bin` held 4 staged exes plus 2 orphan `.tmp` copies.
  Staging timestamps (09:07 vs 09:10) suggest different sessions with
  different builds flip-flop the daemon (unverified).
- Daemon swap 09:43-09:46: each restarted hosted session costs ~6-7 process
  launches (holder python + conhost, claude.exe / pi node + conhost, launcher
  python + conhost); ~9-10 sessions restarted, ~60+ launches. Whether the
  swap or the owner's resume triggered the restarts is unresolved.

### Code audit: launch sites (paths relative to the repo root)

Launcher `agents-plugin/bin/ws-mcp-launcher.py` (byte-identical copies in
`agents-plugin-pi/bin/` and `agents-plugin-wsflow/bin/`):

1. Live-daemon probes are already network-only: `/health` (`daemon_health`
   ~:1041, `probe` ~:1311-1330), socket hello (~:1177), sidecar metadata
   (`read_runtime_metadata`, `runtime_capabilities_compatible` ~:200,
   :730-755), `inspect_runtime` (~:1803-1820). `serve --stdio`,
   `mailbox wait` and CLI calls are socket proxies (`proxy_serve` ~:1746,
   `proxy_cli` ~:1764). Keep it that way.
2. `start_daemon` ~:1357-1405 runs `subprocess.run([staged, "daemon-start",
   ...])` (~:1386) when `health is None` or `needs_start` (`connect`
   ~:1424-1426). `health is None` means "did not answer", not "no daemon".
   Reconnect storm: the reconnect loop ~:1625-1628 calls
   `link.connect(hello, grace=1.5)`; once a swap leaves the daemon silent for
   more than 1.5 s, every connected proxy independently stages the binary
   (`stage_daemon_binary` ~:941) and execs `daemon-start`, which itself
   spawns the real daemon via `SpawnDetached`
   (`agents-plugin-tool/internal/wsdashboard/spawn_windows.go:26-29`): two
   exe launches per proxy.
3. Lifecycle mode ~:1922 `subprocess.call([staged, *argv])`: only on
   health None / `needs_start` / `--replace`. Permitted when it really starts
   a new daemon; same "None means not answering" issue as item 2.
4. `go build` ~:631 (`build_local_devenv_runtime`), `scripts/dev.py:88,141`,
   `agents-plugin-pi/src/bridge.ts:626` + `local-devenv.ts:331`: dev-build
   bursts (dozens of toolchain exes per rebuild, plus a new unknown exe).
5. `git rev-parse` in `detect_project_root` ~:787 per launcher start.
6. `stage_daemon_binary` ~:941-967 writes a new `ws-mcp-dashboard-<sha16>.exe`
   per distinct build and leaks `.tmp` copies.

Daemon / tool (`agents-plugin-tool/`):

- `cmd/ws-mcp/proxy.go:194` LAN-mode self-respawn and
  `cmd/ws-mcp/dashboard.go:367-372` direct-serve stage+spawn: permitted
  (they start a new daemon).
- `internal/mcp/playbook_tools.go:342`: printed / taught commands may point
  at `os.Args[0]` (the runtime exe), inviting models to execute it.
- `internal/wsdashboard/plan_usage_probe.go:23-27` with
  `plan_usage_cmd_windows.go:13,19`: runs `codex app-server` at most once
  per 5 min while the page polls and a Codex login is detected.
- `internal/wsagent/pi.go:63` `pi --list-models`, `:1251` `pi --version`
  (once per process, `internal/mcp/pi_tools.go:86`).
- Per hosted-session children (inherent to hosting): holder python
  (`internal/wsholder/manager.go:179`, `spawn_windows.go:28`), `claude.exe`
  (`internal/wsagent/claude.go:341`), pi node (`internal/wsagent/pi.go:429,458`),
  Pi payload python (`pi_payload.go:43` via `pi_lead.go:131`), per-worker
  launcher `serve --stdio` (`cmd/ws-mcp/agent_mcp.go:12-50`), `taskkill`
  per stop (`internal/wsholder/kill_windows.go:33`).
- Periodic git (not ws exe): `internal/wsdashboard/gitbadge.go:51,147` every
  3 min per watched root.

Python-launcher spawns per session (no ws exe, but python.exe launches):

- Claude: `.mcp.json` `python3 ./bin/ws-mcp-launcher.py serve --stdio` per
  session / subagent; `claude-mod/register.ts:525,531,588` (via `:1039`)
  `mailbox wait --stream` per session plus one-shot fallback;
  `hooks/hooks.json:10` one python per compaction.
- Pi: `agents-plugin-pi/src/mcp-stdio-client.ts:372` launcher per session /
  fork / subagent; `mailbox-waiter.ts:910` long-lived `--stream` per session
  (respawn on mode change / crash), `:1125` one-shot per wake fallback;
  `local-devenv.ts:96` one inspect per lead start.

## Decisions

- Live-daemon checks are REST-only (owner contract). No executable is run to
  read a version, capability, or hello.
- "Did not answer" is not "absent": a launcher starts a daemon only after a
  network check concludes no daemon is alive (with a grace window that covers
  a swap), and starts are single-flight across processes.
- Phase 2 scope (python launcher / mailbox / holder process spawns) waits on
  the owner's scope answer on the board (exe-spawn topic); Phase 1 does not.

## Phases

### Phase 1: No ws executable launch except a real new-daemon start

Goal: after this phase, the only ws executables ever executed are the daemon
start by the launcher when no daemon is alive and the Electron new-daemon
start. Steady-state, swap, reconnect, and model-tool paths launch none.

1. Reconnect storm (launcher items 2-3): replace the 1.5 s grace with
   `/health` polling over a swap-sized window; before `start_daemon`, take a
   cross-process single-flight lock (file lock under the existing runtime
   state dir) and re-check `/health` after acquiring it; only the lock holder
   may stage + `daemon-start`. Proxies that lose the race wait for health.
2. Model-run exe CLI: add `tickets.create_empty` (and audit for any other
   ticket / ws tool the Pi worker profile lacks that workers routinely need)
   to the Pi worker tool profile; make every printed or taught command
   (`internal/mcp/playbook_tools.go:342` and any similar site) never name the
   runtime exe. Where a CLI path must be shown, show the python launcher's
   socket-proxy form, or better, the MCP tool name.
3. Staging hygiene for V3: stable staged daemon path per channel where
   possible (or reuse an existing staged copy whose hash matches), prune
   orphan `.tmp` files and superseded staged exes that no live process uses.
   Find and stop the daemon flip-flop between sessions running different
   builds (the existing "newer core wins" rule must not let two builds
   replace each other repeatedly).
4. `codex app-server` plan-usage probe: replace with a network/REST source
   (like `queryPi`), or drop the probe on Windows when no Codex session is
   active.
5. `pi --list-models` / `pi --version`: cache the result on disk keyed by the
   pi executable path + mtime so a daemon restart does not re-run them.
6. Verification: no Go tests or test binaries on the owner's Windows PC
   (owner rule: Go tests on remote w5090 only). Prove item 1 with launcher unit tests (Python, no exe) and
   a w5090 run; prove the whole phase with a short process-sampling window
   (PowerShell `Get-CimInstance Win32_Process` polling, as in the trace)
   across one daemon swap: expected ws exe launches = 1 daemon start (+1
   detached child) and nothing else.

### Phase 2: Per-session python spawns move to direct network transport

Blocked on the owner's scope answer (exe-spawn topic). Candidate scope:
Claude `.mcp.json` and Pi `mcp-stdio-client.ts` connect to the daemon over
HTTP MCP instead of spawning a python stdio proxy; Claude mod and Pi
extension mailbox waits use daemon HTTP long-poll / SSE instead of a python
`mailbox wait --stream` child. Holder processes and hosted harness children
(`claude.exe`, pi node) stay out unless the owner widens scope. Dev-only
`go build` bursts and git.exe use stay out of this ticket.
