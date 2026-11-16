---
title: Eliminate ws executable launches outside new-daemon start
related:
  261115-feat-werkschaft-electron-daemon-override: Electron new-daemon start is one of the two permitted launch points
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: db8b9983ce535506
sage-review-completeness-reviewed: db8b9983ce535506
completed: 2026-11-16
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
   (`agents-plugin-tool/internal/wsdashboard/spawn_windows.go#L23-L34`): one
   lifecycle invocation per successful launcher start attempt, followed by
   zero to two detached daemon spawn calls inside `Ensure`, not invariably
   two exe launches per proxy (`agents-plugin-tool/internal/wsdashboard/ensure.go#L35-L38,L124-L159`).
3. Lifecycle mode ~:1922 `subprocess.call([staged, *argv])`: only on
   health None / `needs_start` / `--replace`. Permitted when it really starts
   a new daemon; same "None means not answering" issue as item 2.
4. `go build` at `agents-plugin/bin/ws-mcp-launcher.py#L622-L640`
   (`build_local_devenv_runtime`), `scripts/dev.py#L81-L96,L116-L142`,
   `agents-plugin-pi/src/bridge.ts#L614-L634` and
   `agents-plugin-pi/src/local-devenv.ts#L221-L240`: source-build entry points;
   scripts/dev.py normally selects remote w5090 but can fall back locally
   (`scripts/dev.py#L152-L158`). The claimed dozens of toolchain processes
   per rebuild is not established by these source sites.
5. `detect_project_root` conditionally calls `git_root`, which runs
   `git rev-parse`; an existing `WS_MCP_PROJECT_ROOT` skips detection and
   multiple candidate roots may be tried, not exactly one git process per
   launcher start (`agents-plugin/bin/ws-mcp-launcher.py#L780-L822`).
6. `stage_daemon_binary` already reuses a hash-matching
   `ws-mcp-dashboard-<sha16>.exe`; its copy/replace lacks failure cleanup
   (`agents-plugin/bin/ws-mcp-launcher.py#L941-L968`). Go pruning already
   removes superseded staged entries after a two-minute grace, but always
   retains `.tmp` files (`agents-plugin-tool/internal/wsdashboard/spawn.go#L76-L107`).

Daemon / tool (`agents-plugin-tool/`):

- `cmd/ws-mcp/proxy.go:194` LAN-mode self-respawn and
  `cmd/ws-mcp/dashboard.go#L371-L380` direct-serve stage+spawn: permitted
  (they start a new daemon).
- `internal/mcp/playbook_tools.go#L330-L354`: printed commands prefer
  `WS_MCP_CLI_NAME`, then `WS_MCP_RUNTIME_BINARY`, then `os.Args[0]`.
  Normal launcher startup already sets the first to the Python launcher
  (`agents-plugin/bin/ws-mcp-launcher.py#L1848-L1852`); the executable
  fallbacks and generic `ws-mcp mailbox wait` text remain audit targets
  (`agents-plugin-tool/internal/mcp/playbook_tools.go#L313-L321`).
- `internal/wsdashboard/plan_usage_probe.go#L19-L43` with
  `plan_usage_cmd_windows.go#L10-L19`: runs `codex app-server` on viewer
  queries when Codex login presence is detected, ordinarily at five-minute
  intervals but at five-second intervals for forced refresh, not at most
  once per five minutes (`agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go#L19-L27`).
- `internal/wsagent/pi.go#L57-L69` runs `pi --offline --no-approve --list-models`;
  support choices already cache this in memory for five minutes
  (`agents-plugin-tool/internal/wsagent/support.go#L48-L49,L278-L319`).
  `pi --version` is separately cached once per process, including failures
  (`agents-plugin-tool/internal/wsagent/pi.go#L1244-L1267`;
  `agents-plugin-tool/internal/mcp/pi_tools.go#L72-L94`).
- Per hosted-session children (inherent to hosting): holder python
  (`internal/wsholder/manager.go#L601-L648`, `spawn_windows.go#L23-L31`), `claude.exe`
  (`internal/wsagent/claude.go:341`), pi node (`internal/wsagent/pi.go:429,458`),
  Pi payload python (`pi_payload.go:43` via `pi_lead.go:131`), per-worker
  launcher `serve --stdio` (`cmd/ws-mcp/agent_mcp.go:12-50`), `taskkill`
  per stop (`internal/wsholder/kill_windows.go:33`).
- Periodic git (not ws exe): `internal/wsdashboard/gitbadge.go:51,147` every
  3 min per watched root.

Python-launcher spawns per session (outer commands use Python; cold CLI
startup can still reach a ws executable through `main`/`connect`,
`agents-plugin/bin/ws-mcp-launcher.py#L1861-L1911,L1422-L1428`):

- Claude: `.mcp.json` `python3 ./bin/ws-mcp-launcher.py serve --stdio` per
  session / subagent; `claude-mod/register.ts:525,531,588` (via `:1039`)
  `mailbox wait --stream` per session plus one-shot fallback;
  `hooks/hooks.json:10` one python per compaction.
- Pi: `agents-plugin-pi/src/mcp-stdio-client.ts:372` launcher per session /
  fork / subagent; `mailbox-waiter.ts:910` long-lived `--stream` per session
  (respawn on mode change / crash), `:1125` one-shot per wake fallback;
  `local-devenv.ts#L87-L109` inspects at lead/fork startup only when a valid
  local-devenv marker enables that path; it is not unconditional per lead start.

## Decisions

- Fresh item 4 REST settlement, coordinator 2026-11-16 04:23:21Z: replace
  Codex usage acquisition with read-only HTTPS, no subprocess on any OS.
  Preserve the existing file-only `detect()` predicate and CODEX_HOME
  selection. Selected Codex failure never falls through to Pi. Explicit
  ChatGPT mode uses its OAuth access token, never API-key material; legacy
  mixed API-key/token documents and unsupported selected credentials return
  unavailable. No env/keyring/external/PAT/config resolver or auth refresh,
  login, credential write, cookie or CLI fallback is added.
- Token expiry: reject a decodable expired access-token exp locally. Otherwise
  eligible nonempty tokens with unknown/missing/undecodable exp may make the
  bounded GET; the server remains authoritative. Reject malformed credential
  JSON, empty access and invalid header values locally. Optional upstream
  expiry parsing supports this over requiring a JWT expiry for every token;
  decoding is not signature validation. Keep all secrets request-local and
  all errors/view/cache/history sanitized; never persist account/token/raw
  response/URL-error material.
- Keep the exact `via Codex CLI` wire/history identifier as legacy
  CLI-owned-credential provenance, not evidence of a subprocess. Record this
  interpretation at the identifier in code and here. Coordinator explicitly
  settled this new interpretation over a new cosmetic REST label: the old
  history validator would reject that label and stop read/append. T0 is
  preferred to a boundary under AGENTS.md. Any future display-label change
  requires its own compatibility decision; no UI change is part of item 4.
- Use the fixed `https://chatgpt.com/backend-api/wham/usage` GET and selected
  OAuth access/account-id headers, with no redirects, reserve/reset requests
  or configurable upstream origin. Official Codex 0.155.0 implementation is
  evidence of this internal endpoint, not a third-party public API guarantee.
  Keep the existing detached 10-second budget, completion-based 5-minute /
  forced 5-second cadence, cached values/source/asOf on failure and history
  only on success. Missing weekly quota is failure, not empty replacement.
- Classify exactly 604800 seconds as weekly in the new Codex REST mapping.
  Preserve Pi's existing conversion and selection semantics; shared parsing
  may retain raw seconds and apply caller-specific classification. Leave
  `currentUsage`, `pushClaude`, Claude windows and all UI/assets untouched.
- Item 4 compatibility is T0 with unchanged latest/history fields and source
  identifiers. Add new REST sample -> frozen pre-change history read/append
  -> new reopen coverage preserving prior samples, source/windows/asOf;
  latest-only compatibility tests do not establish this history property.
  Sources: `plan_usage.go:18-67`, `plan_usage_history.go:44-85,128-190`;
  old and new write the same shapes/literals and new reopen retains them.

- Live-acceptance repair, coordinator assignment 2026-11-16 02:59:27Z:
  fix the proven Python dev lifecycle gaps before the Codex REST follow-up.
  `scripts/dev.py` must participate in the existing launcher starter gate
  through bounded expected-build REST confirmation; builds stay outside it.
  Reuse the imported `LauncherStartLock`, not Go's daemon singleton or a new
  gate/interface. For the specific exit-1, parsed `outcome: unavailable`
  response, use the remaining lifecycle deadline to confirm the expected
  content hash over REST, without another executable invocation. Keep the
  original diagnostic; reject malformed reports, unrelated failures, old
  hashes and deadline expiry. Technical choices follow the two source-triage
  findings; rejected: unconditional nonzero-exit success, a second starter,
  a global wait increase or an unbounded/resetting deadline.
- Acceptance evidence distinguishes process creation from readiness. The
  sampled extra starter is real, but the supplied CSV does not establish
  continuous health at its under-lock probe. Equal numeric cores already
  ignore hash/build time; do not change that policy based on process age.
  Coordinator-owned repeat sampling, not a worker runtime operation, must
  determine any remaining anomaly after repairing the dev gate bypass.
  Coordinator correction at 03:11:45Z: 02:54:29 was process creation plus
  the /health `started` field time, not independently observed health. The
  sampler first recorded healthy around 02:57:00; old retry waves stopped and
  dashboard reconnect logging appeared around 02:55:08. Reachability may have begun
  between 02:54:29 and 02:55:08, so an unpublished-endpoint window plausibly
  explains PID 3452 at 02:54:52, but its exact admission cause remains unproven.
  Do not pursue that historical anomaly or add diagnostics in this repair.
- Repair compatibility tier: T0, internal Python caller coordination and
  result handling only. Existing lock, endpoint and Go JSON outcome shapes
  remain unchanged. Old launchers ignore the persistent gate; new launchers
  reopen the same byte-zero lock. `scripts/dev.py` reads the existing outcome
  and REST hash; it writes no shared data shape. Source evidence:
  `scripts/dev.py:206-220,291-303`, launcher `:1079-1127,1540-1573`,
  `cmd/ws-mcp/proxy.go:419-422`, `internal/wsdashboard/ensure.go:178-207`.

- item 4 deferred to follow-up by owner direction 10:56
  The isolated Codex admission implementation is parked at local branch
  `parked/exe-phase1-item4-codex` and reverted from this ticket's retained
  branch. The owner-directed Codex REST follow-up is not implemented here.

- Live-daemon inspection uses REST normally and retains the existing
  network-only `/mcp` hello fallback for compatible older daemons whose
  `/health` lacks a version (coordinator approval, 2026-11-16 01:14:40Z).
  No executable is run to read a version, capability, or hello. Rejected:
  removing the legacy fallback and creating an old-client boundary.
- "Did not answer" is not "absent": absence-based starts need a swap-sized
  health grace window and cross-process single-flight. Preserve real new
  daemon starts for newer numeric cores, explicit `--replace` and LAN-mode
  handoffs (coordinator approval, 2026-11-16 01:14:40Z). Equal-core hash,
  suffix or build time never automatically replaces a healthy daemon.
- Startup coordination (coordinator approval, 2026-11-16 01:17:44Z): updated
  launchers serialize starts with persistent `dashboard/launcher-start.lock`,
  separate from Go's daemon-held `dashboard.lock`. Use a non-inheritable
  descriptor and OS advisory lock (`fcntl.flock` on POSIX; byte-zero
  `msvcrt.locking` on Windows), with finally-unlock/close; never truncate,
  unlink or TTL-steal the lock. Hold the starter lock through health recheck,
  authorized staging/start and healthy successor or bounded failure. Losers
  poll over the network/retry the lock within the existing 75-second budget.
  Never acquire/hold Go's singleton lock across invocation of the starter.
  Preserve `DaemonHung`; a live or unqueryable endpoint PID vetoes absence
  startup. No process kill or endpoint deletion to manufacture absence.
  Rejected: treating silence alone as absence or sharing Go's held lock.
- The single-flight guarantee covers updated launchers, not old binaries.
  A rare Electron-versus-launcher race may execute one losing starter that
  exits under Go's singleton; the coordinator explicitly accepts this as a
  new-daemon start event. No cross-language start-gate interface is added.
- Owner (1205)=a, 2026-11-16 10:01: scope is ws executables only. The
  Python launcher stays; `.mcp.json` intentionally routes through it. Python
  process creation is not part of the V3 issue. Rejected: direct HTTP MCP
  transport replacing the launcher; the former Phase 2 is removed. The
  coordinator clarified on 2026-11-16 01:14:40Z that original Phase 1 items
  4 (Codex probe) and 5 (Pi probe cache) remain explicitly approved; Python
  is exempt, not those probes. Preserve credential/profile semantics.
- Staging keeps hash-addressed immutable filenames. Clean failed-copy `.tmp`
  files and conservatively prune only stale launcher-owned `dashboard/bin`
  executables unused by any live process, excluding current/accepted targets.
  Never prune Electron packages or retained rollback targets (coordinator
  approval, 2026-11-16 01:14:40Z). Rejected: stable-path overwrite on Windows.
  This ticket does not rewire Electron startup to replacement-capable Ensure.
- Mailbox waits are Python + network only, including Claude mod and Pi
  stream waits, one-shot fallbacks, and launcher CLI waits. They never stage
  a binary or invoke `daemon-start`; an unreachable daemon causes network
  backoff/reconnect instead. Only a session MCP launcher (`serve --stdio`)
  or explicit lifecycle mode may initiate a new daemon start. Coordinator
  approval, 2026-11-16 01:04:06Z, extends this rule to every other CLI proxy
  path: no staging/start on initial connect or reconnect; return or back off
  with a clear "daemon not running" result when unavailable.
- Profile audit: add only `tickets.create_empty` to `PiWorkerWSTools`;
  mini-lead inherits it (`internal/wsagent/builtin.go#L95-L114`). No incident
  establishes another necessary grant. Preserve parentless delegate keys,
  bootstrap exclusions, overrides and saved launched-profile ceilings; do
  not widen an existing process snapshot to admit a new child definition.
  Rejected: blanket ticket tools or lead authority. This technical fact-gap
  settlement is lead-confirmed under the coordinator's autonomy directive.
- Historical item 4 admission design selected Windows live-session gating
  for CLI probes; that implementation was parked/reverted before landing.
  The fresh 04:23:21Z REST settlement above supersedes that unimplemented
  mechanism on every OS. REST requires no executable-admission registry gate;
  retain cache, credential precedence and no-fallback semantics instead.
- Pi derived cache: operation-specific files under
  `<cache>/dashboard/pi-probes-v1/<fingerprint>-{version,models}.json`,
  schema 1 with kind, identity, original observation time and parsed value.
  Cache only successes, never raw output, credentials, errors, permissions
  or profiles. Fingerprint operation, canonical command path, size and
  nanosecond mtime; include existing resolved npm shim/Node/CLI/package
  stamps and, for models, effective agent-directory identity plus stat-only
  `models.json`/`auth.json` stamps including absence. Reuse wrapper resolution
  without changing how the probe is executed (`internal/wsagent/backend.go#L449-L519`,
  `pi.go#L57-L64,L1251-L1257`). Version success lasts while identity matches;
  model success lasts five minutes from observation, not cache read. Preserve
  existing in-memory version once/failure behavior and model failure TTL
  (`support.go#L48-L55,L141-L157`, `internal/mcp/pi_tools.go#L72-L94`).
  Missing/corrupt/stale records use the existing bounded probe; newer schemas
  are ignored and preserved. Validate decoded values using existing parsers;
  owner-only atomic write failure must not invalidate a successful probe.
  Rejected: shared read-modify-write maps, credential-content fingerprinting,
  persistent failure caching or an unrelated mid-process version refresh.
- Compatibility tier: T0. No tool/argument/hello/endpoint/profile or existing
  usage-store shape changes; the grant exposes an existing handler, and
  legacy hello remains supported. Old endpoint/metadata/stamp readers and
  writers retain their current shapes (`internal/wsdashboard/endpoint.go#L48-L94`,
  launcher #L196-L240,L300-L349); new reopen reads them unchanged. The new
  derived Pi directory is disjoint from old `support.json` writes
  (`internal/wsagent/support.go#L183-L213`), session pruning
  (`internal/wsdashboard/registry.go#L331-L385`) and `dashboard/bin` pruning
  (`spawn.go#L89-L107`). Thus old code neither rewrites nor deletes the cache;
  new reopen validates/reuses it, or recomputes missing/invalid derived data.
  Add a directional fixture exercising those old write/prune paths before
  new reopen; no T2 recovery or new T3 boundary is claimed.

## Fresh item 4 REST fact population - 2026-11-16

This section is evidence and recommendations for the next design pass, not an
implementation plan or a replacement for the recorded Decisions/Phase 1.
Coordinator authorization 03:30:49Z reports owner (1207)=a for a fresh Codex
REST plan from `7bdf11e7430163fa4ddffdfe3319ac8f02c87adc`; the full owner
answer was not supplied. Original source and dev repair are landed here
(merges `74924b4c0`, `4fdb920c0`); live acceptance remains coordinator-owned.
The old Windows live-session CLI gate is historical, not fresh REST authority.
Do not reuse/cherry-pick `d9fd94543`. This assignment changes ticket facts only.

### Checkable current claims and reuse boundary

| Claim | Verified tree evidence |
|---|---|
| Login presence and home selection | `agents-plugin-tool/internal/wsdashboard/plan_usage.go#L134-L168`: `CODEX_HOME` overrides `<home>/.codex`; only `auth_mode == chatgpt` or absent mode plus non-null `tokens` detects Codex. It does not inspect token strings, API-key env, keyring, config.toml, external auth or profiles. An explicit daemon Home disables inherited config overrides (`daemon.go#L406-L414`; `daemon_home_test.go`). |
| Source precedence | `plan_usage_probe.go#L19-L64`: detected Codex always selects queryCodex; even failure never selects Pi. Missing/API-key-only file selects Pi when its `openai-codex` member is present. This is ws file-presence policy, not complete Codex CLI auth selection. |
| Existing executable source | `plan_usage_probe.go#L93-L165`, `plan_usage_cmd_windows.go`, `plan_usage_cmd_unix.go`: queryCodex starts app-server with an 8-second context and JSON-RPC initialize / account/rateLimits/read. There is no landed Windows session-admission gate. |
| Existing REST reuse | `plan_usage_probe.go#L169-L232`: queryPi alone reads Pi OAuth access/accountId/expires, checks expiry, performs GET, and parses snake_case usage. It never refreshes or borrows Codex credentials. `plan_usage.go#L49-L67` supplies the fixed ChatGPT URL, 10-second HTTP timeout, no redirects and private latest-cache serializer. |
| Cadence and non-destruction | `plan_usage_probe.go#L19-L64`: queryMu coalesces viewers; ordinary 5 minutes / forced 5 seconds, bounded 10-second context detached from viewer cancellation; lastQuery advances on completion. Failure changes status only, retains measurements/source/asOf and adds no history. GET/refresh admission stays `plan_usage.go#L198-L235`. |
| Weekly mapping and window removal | `plan_usage_probe.go#L66-L91`: only valid weekly windows (10080 minutes), either primary or secondary; no weekly window is failure, not an empty successful replacement. Success replaces the whole GPT entry. `plan_usage.go#L169-L183` filters expired/invalid windows for both harnesses without modifying the stored entry. The REST seconds-to-minutes division at `plan_usage_probe.go#L225-L231` truncates: a new parser should test exact 604800 seconds rather than accidentally accepting 604801–604859. |
| Existing credential/profile reuse | No tracked `credentials` or `userprofiles` file/package found (`git ls-files '*credentials*' '*userprofiles*'` returned nothing; scoped internal search for keyring/cli_auth_credentials_store/CODEX_API_KEY found no production resolver). Do not invent a reusable resolver or conflate wsagent tool profiles with authentication profiles. |

Paths abbreviated in this section's tree citations are under
`agents-plugin-tool/internal/wsdashboard/`.

### Official upstream proof, version-pinned; not a public API guarantee

Nonsecret installed metadata
`C:/Users/user/AppData/Roaming/npm/node_modules/@openai/codex/package.json`
reports `@openai/codex` **0.155.0** (including its platform dependency pins).
No CLI was run to obtain a version; the active PATH-selected executable and
its live auth/config were not inspected. Official GitHub tag
[rust-v0.155.0](https://api.github.com/repos/openai/codex/git/ref/tags/rust-v0.155.0)
is annotated object `799f378ee7f85c775dee82d9bc45cc2df8df18fb`, resolving to
commit `f0a1b8f0849d90960bc406b848f32e5a129b0457`. All links below use that
commit, not moving main or third-party Orca behavior.

- [Usage request](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/backend-client/src/client/rate_limit_resets.rs#L22-L83), URL builder at L124-L129: passive usage reads GET `/wham/usage` for ChatGPT backend-api style, `/api/codex/usage` for the alternate backend style. Reserve is explicitly opt-in, not for passive readers. [Client normalization/headers](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/backend-client/src/client.rs#L184-L265) adds `/backend-api` for ChatGPT hosts. Thus ws's existing `https://chatgpt.com/backend-api/wham/usage` is a source-backed route used by Codex with selected ChatGPT OAuth credentials, not an invented endpoint. This is official CLI implementation evidence for an internal backend, not evidence of a documented, supported third-party REST API/SLA or live success.
- [App-server mapping](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/app-server/src/request_processors/account_processor.rs#L1129-L1193) obtains auth, refuses non-Codex-backend auth and calls that same client; its default `rateLimits` selects the `codex` snapshot. [Payload mapping](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/backend-client/src/client.rs#L549-L622) maps top-level `plan_type`/`rate_limit` to `codex`; additional feature limits are separate, not replacements for weekly Codex quota. [Window conversion](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/backend-client/src/client.rs#L678-L690) maps used_percent, duration and reset_at. [Generated payload](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/codex-backend-openapi-models/src/models/rate_limit_status_payload.rs#L15-L47) and [window](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/codex-backend-openapi-models/src/models/rate_limit_window_snapshot.rs#L13-L23) confirm snake_case plan/rate_limit/primary_window/secondary_window and seconds-based duration/reset. Preserve ws's weekly-only scope, not new credits/reserve/additional-limit UI.
- [Bearer headers](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/model-provider/src/bearer_auth_provider.rs#L31-L46) are Authorization Bearer and optional ChatGPT-Account-ID, with FedRAMP routing when selected. [Auth-provider construction](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/model-provider/src/auth.rs#L306-L324) uses the selected auth's token and account id. [Token/account access](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/login/src/auth/manager.rs#L552-L610) uses `tokens.access_token` and `tokens.account_id`, not id_token as bearer. [Token shape/expiry parser](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/login/src/token_data.rs#L10-L44) (expiration parser L142-L148) distinguishes access, refresh and id tokens; `last_refresh` is not an access-token expiry. No JWT decoding proves signature validity.
- [Default HTTP headers](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/login/src/auth/default_client.rs#L334-L350), default originator at L40: User-Agent plus `originator: codex_cli_rs`, with optional residency. Existing Pi headers `OpenAI-Beta: codex-1` and `originator: Codex Desktop` are not established as required by this pinned request path. Do not claim ws must impersonate Desktop or copy browser cookies. Upstream client L273-L325 includes raw response bodies in errors; that error policy must NOT be copied into ws's sanitized viewer/cache boundary.

### Upstream auth precedence versus the narrower existing ws contract

[Auth loading](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/login/src/auth/manager.rs#L1461-L1569)
orders: nonempty `CODEX_API_KEY` only when enabled and allowed; process-local
ephemeral external auth; `CODEX_ACCESS_TOKEN` for PAT/agent identity when
allowed; then the configured persistent store (unless ephemeral-only).
`OPENAI_API_KEY` has a separate env reader at L926-L949, not an unconditional
OAuth override in this loader. Provider-specific selection is not a reason
to send an API key to wham. Exact CLI/app-server flag/provider/managed-policy
composition is not fully traced here; this is not a full auth-parity claim.

[Storage dispatch](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/login/src/auth/storage.rs#L511-L544):
file reads CODEX_HOME/auth.json; keyring reads only its selected backend;
auto tries keyring then file on absence OR error (L431-L442); ephemeral is
process memory. Direct keyring uses service `Codex Auth` and key
`cli|<first16 SHA256 of canonical CODEX_HOME path>` (L235-L251).
Encrypted-secrets keyring uses CODEX_AUTH in its local CodexAuth namespace
(L326-L399), not necessarily that direct entry.
[Config enums/defaults](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/config/src/types.rs#L106-L155):
CLI store default file; keyring backend default secrets on Windows, direct
elsewhere. MCP OAuth `.credentials.json` is a different store. No keyring,
credential file or live environment value was read in this investigation.

[Mode resolution](https://github.com/openai/codex/blob/f0a1b8f0849d90960bc406b848f32e5a129b0457/codex-rs/login/src/auth/manager.rs#L1770-L1787)
honors explicit auth_mode; legacy API-key-bearing JSON resolves to API key
before default ChatGPT. ws's legacy tokens-presence heuristic is narrower
and can disagree on a mixed legacy document. Token prefixes alone are not
safe API-key-versus-OAuth classification. Full parity would need a new,
read-only resolver/policy boundary; none exists in this checkout. The landed
`261113-feat-ws-dashboard-rail-usage-gutter` explicitly treats keyring-only
login as absent and leaves auth slots separate. Expanding that scope cannot
be smuggled in as an endpoint substitution.

### Minimum recommended contract and exact verification surface

Recommendation for lead design, not a settled decision: replace only the
Codex acquisition with a bounded, read-only GET using file-selected ChatGPT
OAuth access/accountId; factor the existing Pi HTTP/parser seam without
changing Pi selection or credentials. Use the same fixed HTTPS origin,
refuse redirects, no refresh/login/write/cookie/CLI fallback, no arbitrary
provider base URL, and no Reserve/reset-credit requests. Keep token material
request-local; never cache/log auth JSON, token/id/account, URL error objects
or response bodies. Preserve generic unavailable, cached values/asOf/source,
completion cadence and history-on-success only. No Windows live-session gate
is needed to avoid a subprocess once the source is HTTP, but its replacement
must be explicitly recorded by the lead rather than silently rewriting the
historical plan. Keep existing weekly-only validation and no-weekly failure
semantics unless separately decided.

Minimum edit candidates (not edits made): `plan_usage.go` (private dependency
seams/detection only as approved), `plan_usage_probe.go` (request-local Codex
credential read plus shared safe GET/parse), `plan_usage_cmd_windows.go` and
`plan_usage_cmd_unix.go` (obsolete helper removal if no callers remain),
`plan_usage_test.go`, `plan_usage_probe_test.go`, `plan_usage_compat_test.go`,
`plan_usage_history_test.go`. No MCP/profile/launcher/Claude/assets change is
needed for the narrow file-only candidate. `daemon_home_test.go` is an
existing isolation regression to retain; a codexBinary seam removal would
also require its focused fixture update (`daemon_home_test.go`, `codexBinary` assignment). New auth-store support would
invalidate this minimal file list and requires a separate bounded design.

Reuse/extend existing `TestPlanUsageDetection`, `TestPlanUsageWindowClassification`,
`TestPlanUsageFallbackOrderAndCadence`, `TestPlanUsagePiExpiryFailureRetentionAndNoLeaks`,
`TestPlanUsagePiRedirectDoesNotForwardToken`, `TestPlanUsageCodexMalformedAndTimeout`,
`TestPlanUsageCancelledViewerStoresSuccess`, `TestPlanUsageRefreshSameOrigin`,
`TestPlanUsageHistoryGPTAcquisitionAndOptionalAPI`,
`TestPlanUsageHistoryMixedVersionNonDestruction`, and
`TestDaemonExplicitHomeDoesNotQueryUsage`. Replace the fake app-server init,
copied-executable fixture, protocol-negative and batch-shim tests with fake
HTTP/credential cases; do not retain executable test fixtures just to prove
zero executable use (`plan_usage_probe_test.go#L43-L148`). Cover explicit and
legacy mode conflicts, malformed/missing/expired access, account-id header,
no Pi fallback after selected Codex failure, 401/403/429/5xx, timeout,
redirect, oversized/malformed bodies, null/missing windows, exact weekly
seconds, switched primary/secondary, secret-free persistence/view/errors,
no-refresh/no-write and completion-based concurrency/cadence. Future Go
gates run only on remote w5090; none ran in fact population.

Directional T0 candidate, NOT a blanket verdict: retain latest fields
(`plan_usage.go#L18-L67`); old fixed-struct load/save can round-trip unchanged
known values but loses unknown fields, with no raw reopen recovery. History's
old reader validates source strings (`plan_usage_history.go#L44-L85`), refuses
unsupported semantics before append (`#L128-L164`), and preserves supported
extensions (`#L166-L190`). A new `via Codex REST` source is therefore NOT
transparently T0: an old history reader rejects it and stops normal history
reads/writes; reopen with new code restores readability, not old usability.
Keeping the already accepted `via Codex CLI` label preserves both directions
but may misdescribe transport. Lead must settle truthful source naming versus
compatibility; do not invent a new label under the old T0 declaration.
Extend the frozen old latest fixture at `plan_usage_compat_test.go#L11-L88`
with the pinned pre-change history reader/writer: new REST sample -> old
read/append -> new reopen must retain source/windows/asOf and prior history.
Existing frozen latest-only coverage alone does not prove source-label T0.

The three design gaps from this fact population were settled in Decisions
by coordinator 04:23:21Z: file-only selection, stable legacy source identifier
and optional expiry handling. The preceding recommendations are grounding
history; the settled Decisions and Phase 1 item 4 are implementation authority.
No full owner (1207) verbatim, public third-party API guarantee, active CLI
configuration or live credential/request success is established here.

Separate coordinator-owned Claude/UI scope: currentUsage is shared by both
harnesses (`plan_usage.go#L169-L183`), while pushClaude merges accepted windows
and never removes a missing kind on a later plan change (`#L72-L116`). Codex
success instead replaces the whole entry (`plan_usage_probe.go#L55-L57`),
and failure retains it. These are the exact cache/window-drop overlap sites;
no assertion that Max lacks a five-hour limit, no Claude repair, and no
percentage/offset layout change is made or authorized here.

## Prior Decisions

- 1866275e (2026-11-13, commit): "Owner priority: AhnLab prompts on non-daemon ws execution; remove ProcessStream, DIRECT bypass, catch-all exec, and executable inspection instead of adding another fallback." — bearing: constrains
- 261113-bug-ws-only-the-dashboard-daemon-runs-an-exe (2026-11-13, Decisions): "A newer launcher meeting an otherwise compatible older daemon without a version endpoint must still work: fall back to the existing /mcp hello's returned daemon version" — bearing: resolved by retaining network-only legacy hello
- 7f40e811 (2026-11-13, commit): "pi-worker ws tools: git.status, git.diff, git.log, git.merge_base, git.commit, enumerated from the delegate-implementer floor's commit step and the reviewer playbook's git.diff" — bearing: constrains the already-landed profile scope
- fbd1cd6a (2026-11-13, commit): "The launcher sets WS_RSRC_ROOT=<plugin>/rsrc before daemon-start; dev.py set nothing, so the daemon fell back" — bearing: historical dev-root evidence; the landed bounded dev repair did not change root selection
- 261114-feat-ws-dashboard-usage-history-trend (2026-11-15, Decisions): "Keep account identifiers out of history. Do not join observations across reset or source boundaries" — bearing: constrains
- a3b11204 (2026-11-15, commit): "T0 additive contract: latest structs and serializer remain unchanged." — bearing: constrains
- d3500a7c (2026-11-14, commit): "detach usage queries from viewer cancellation with a bounded 10-second context, record lastQuery after completion" — bearing: constrains
- f9b3c792 (2026-11-13, commit): "Codex CLI owns its credentials. Only absent/API-key CLI logins use Pi's unrefreshed token, confined to a bounded usage request; redirects and upstream errors cannot forward or expose it." — bearing: constrains
- 261113-feat-ws-dashboard-rail-usage-gutter (2026-11-13, Decisions): "an API-key-only login or a missing auth.json (including credentials kept in the OS keyring) counts as no login, so the Pi fallback applies." — bearing: constrains
- 261113-bug-ws-only-the-dashboard-daemon-runs-an-exe (2026-11-13, Decisions): "Version and compatibility checks execute zero ws binaries, including cold load, cache/stamp miss, repair, and local-devenv paths." — bearing: constrains
- 261115-feat-werkschaft-electron-daemon-override (2026-11-15, Decisions): "Neither path depends on an installed plugin runtime." — bearing: constrains
- 96b27026 (2026-11-10, commit): "stage_daemon_binary no longer deletes other entries: one may be the running daemon's own executable (it respawns itself from it on a LAN change)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Fresh item 4 only: agents-plugin-tool/internal/wsdashboard/plan_usage.go, plan_usage_probe.go, plan_usage_cmd_windows.go, plan_usage_cmd_unix.go and focused plan_usage test files; earlier lifecycle/profile/Pi source is landed. |
| scope.surface | public-interface | Existing GET /rail/usage source/status/windows and persisted history source semantics; no new endpoint. |
| scope.new_public_symbol | no | Narrow REST candidate needs no exported symbol, MCP tool, endpoint or profile grant. |
| scope.new_type_contract | no | Private request-local OAuth decode and expiry handling reuse existing shapes; source identifier and on-disk contracts remain unchanged under the 04:23:21Z settlement. |
| scope.test_surface | existing | plan_usage_test.go, plan_usage_probe_test.go, plan_usage_compat_test.go, plan_usage_history_test.go and daemon_home_test.go under agents-plugin-tool/internal/wsdashboard/. |
| complexity.reuse_points | confirmed | Read query/queryPi, codexUsageEntry, usageReset, currentUsage, latest save/reopen, history validator/append and frozen latest writer; no reusable credentials/userprofiles resolver found. |
| complexity.side_effect_risk | high | New Codex credential read sends selected OAuth off-machine and writes shared latest/history caches; no process/refresh authority is needed. |
| risk.correctness | high | Auth selection, exact weekly duration, no-weekly failure, expiry and no-Pi-fallback must retain existing cached values without substituting accounts. |
| risk.fit | moderate | Existing Pi GET/parser fits official Codex wham source; full auth-store parity and truthful source labeling are unsettled boundaries. |
| risk.test | high | Current Codex tests execute copied test binaries and must become HTTP-only fixtures; remote gates and live acceptance are not run here. |
| risk.security_or_contract | high | OAuth/API-key distinction, request-local secrets, no redirects/refresh, source allowlists and old-write/new-reopen compatibility need explicit proof. |

## Phases

### Phase 1: No ws executable launch except a real new-daemon start

Goal: ws executables run only for real new-daemon starts by the session MCP
launcher, explicit lifecycle mode, or Electron. This includes legitimate
newer-core, explicit replacement and LAN handoffs. Ordinary steady-state,
mailbox/CLI proxy, reconnect-to-successor and model-tool paths launch none.

1. Reconnect storm (launcher items 2-3): replace the 1.5-second grace with
   swap-sized health polling. Before permitted staging/start, acquire the
   separate `dashboard/launcher-start.lock` defined in Decisions, recheck
   health and keep the lock until healthy successor or bounded failure.
   Losing updated launchers wait/retry over the network within the existing
   75-second budget. Preserve hung/live/unqueryable-PID distinctions rather
   than converting timeout into absence; keep legitimate replacement cases.
   Apply the gate to session-MCP startup/reconnect and lifecycle startup.
2. Model-run exe CLI: add only `tickets.create_empty` to the inherited
   builtin Pi worker tool floor, preserving scope/bootstrap/profile ceilings
   as settled in Decisions. Make every printed or taught command
   (`internal/mcp/playbook_tools.go:342` and any similar site) never name the
   runtime exe. Where a CLI path must be shown, show the python launcher's
   socket-proxy form, or better, the MCP tool name.
3. Staging hygiene for V3: retain hash-matching content-addressed reuse;
   clean failed-copy `.tmp` files and conservatively prune stale owned
   `dashboard/bin` entries not used by any live process. Preserve current,
   accepted and rollback targets and exclude Electron package staging.
   Verify existing equal-core retention rather than assuming an unproven
   build flip-flop; fix a remaining automatic replacement path if reproduced.
4. Codex REST usage, fresh settlement 04:23:21Z: remove app-server execution
   on every OS. Use file-selected ChatGPT OAuth access/account-id with the
   fixed HTTPS wham GET and bounded safe HTTP/parser reuse in Decisions.
   Preserve file-only detection, Codex-before-Pi/no-fallback semantics, exact
   new-path weekly mapping, cadence, cache retention and stable legacy source
   identifier. Remove obsolete executable-command helpers/fixtures only when
   caller-free. No credential refresh, alternative auth-store resolver,
   Claude/shared-window filter or UI change. Add sanitized HTTP/auth-negative
   fixtures and frozen old history read/append/new-reopen proof of T0.
5. `pi --list-models` / `pi --version`: implement the private success-only
   cache, identity/invalidation and five-minute model freshness settled in
   Decisions. Prove restart reuse and invalidation without profile changes.
6. Mailbox waits: cut every path from Claude mod `register.ts`, Pi
   `mailbox-waiter.ts` (stream and one-shot fallbacks), and the launcher's
   `mailbox wait` CLI/reconnect handling to binary staging or daemon start.
   These paths use Python + HTTP/socket only and back off/reconnect when
   the daemon is unavailable. Only `serve --stdio` or explicit lifecycle
   mode may initiate a new daemon start. Apply the same no-start permission
   to every other CLI proxy command, with a clear "daemon not running"
   result when unavailable. Verify initial connect and reconnect, mailbox
   stream/one-shot, and non-mailbox CLI cases with mocked launcher tests
   that forbid staging/start.
7. Verification: no Go tests or test binaries on the owner's Windows PC
   (owner rule: Go tests on remote w5090 only). Prove item 1 with launcher unit tests (Python, no exe) and
   a w5090 run; prove the whole phase with a short process-sampling window
   (PowerShell `Get-CimInstance Win32_Process` polling, as in the trace)
   across one coordinated daemon swap: expect one starter invocation and
   its necessary daemon child, with no mailbox/CLI/probe ws executions.
   Report the accepted Electron-race boundary separately. For fresh item 4,
   cover file-only explicit/legacy/mixed auth selection, expired/unknown exp,
   malformed/empty access/header rejection, no Pi fallback, forced refresh,
   401/403/429/5xx, redirects/timeouts/oversized or malformed payloads,
   primary/secondary exact weekly windows and failure cache/history retention.
   All Codex fixtures are HTTP-only; no executable sentinels. Preserve Pi
   cache restart, wrapper/PATH/config invalidation, expiry, corrupt/newer
   schemas, failed writes and directional old-writer/new-reopen behavior.

### Live-acceptance repair scope (coordinator assignment 2026-11-16 02:59:27Z)

- Implement only the two dev lifecycle fixes in Decisions, in
  `scripts/dev.py` and focused `scripts/test_dev.py` cases. If an existing
  launcher test needs a deterministic assertion, limit it to this dev-gate
  interaction; do not split or refactor the three launchers.
- Mock all executable/PID APIs and control clocks. Prove a dev-held gate
  prevents an updated MCP contender from starting beyond its 20-second
  grace; successor publication yields zero extra starts. Cover bounded
  contention, exception/failure release and unchanged explicit replacement.
- Prove unavailable -> expected REST hash recovery invokes exactly one
  starter; old hash, no health, malformed/non-unavailable failures and budget
  expiry still fail with useful diagnostics. Never accept a PID alone or
  shorten production grace. No local launcher runs, builds or live swap.
- Run one independent R1, then fixes-only R2 for substantive fixes. Return
  the reviewed code/head and evidence to the coordinator; leave this ticket
  open. Codex REST is subsequent work, not part of this repair commit. The
  parked Codex commit is CLI-admission work, not a REST implementation.
- Source-only triage at `ae86b1408` is recorded in the machine-local report
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/462376e9-01-exe-live-triage.md`.
  Swap evidence is under `D:/Temp/daemon-swap-1116/`; old in-memory launchers
  are the previously accepted single-flight boundary, not a new defect.

### Result (30e401608) - 2026-11-16

Item 4 live acceptance: **PASS**. This result records the coordinator-authorized
Codex REST check only; it does not re-certify every historical Phase 1 sampling
claim or remove the accepted old-launcher/Electron and retention boundaries.
The ticket remains open for the coordinator's final disposition.

- Integration: item 4 final source handoff `30e401608` was merged as
  `8072be902`. Coordinator 06:22:08Z authorized acceptance on the swapped
  daemon, reported as develop `529c0255e`, including that merge. Before and
  after the check, `/health` confirmed the same PID 2404, version
  `0.50.1-dev`, start time `2026-11-16T06:17:21Z`, and content SHA256
  `84a3ccf259151abf8b7cbdc967ce6bf40c49a46b09e2db69b9cd4d111901c98a`.
- Approved standard-Python HTTP requests addressed only the running localhost
  dashboard. All returned HTTP 200: `/health` at 06:25:41.418-41.423Z,
  `/rail/usage?history=1` at 06:25:41.423-41.993Z, same-origin
  `/rail/usage?history=1&refresh=1` at 06:25:47.193-47.498Z after a 5.2-second
  cadence wait, and final `/health` at 06:25:47.498-47.510Z. No shutdown token
  was used. Real credential acquisition remained inside the daemon's existing
  usage path; no credential document, token or account identifier was exposed.
- The Codex row populated successfully with plan `pro`, empty success status,
  source `via Codex CLI` (the approved legacy credential-provenance identifier)
  and a `seven_day` window. Refresh advanced AsOf from `1794810341980` to
  `1794810347487`; usage remained 44% and reset `1795332373000`. Unchanged
  percentage therefore did not conceal a stale-cache-only observation.
- History grew from 212 to 213 total samples and from 41 to 42 Codex samples.
  Exactly one new Codex sample matched the refreshed row's source, kind,
  AsOf `1794810347487`, usage 44% and reset `1795332373000`. This verifies
  fresh successful REST acquisition plus append with the kept identifier;
  the independent frozen old-history read/append/new-reopen test remains
  the separate directional T0 compatibility proof.
- Coordinator separately reported zero `codex.exe` spawns over the swap
  interval from 30 seconds before to three minutes after. That is provided
  sampler evidence, not a process measurement performed by this acceptance
  executor. It is not expanded into a blanket all-executable claim.
- Evidence is the redacted execute-worker output in this lead session, worker
  `0eceb4bd-bd0e-4419-b9c0-1bed158b1e48`; no evidence/scratch files were
  created. The D: free-space guard passed. No credential file was opened by
  the executor, no direct authenticated upstream call was made outside the
  daemon, and no local Go/ws/launcher/test executable, build, PID query,
  sampling, daemon lifecycle action or shared-root write was performed.

#### Edition (3345e3588) - 2026-11-16

##### Overall final verdict

**Accepted for closure within the approved scope and residual boundaries.**
Coordinator 06:31:41Z authorized final evidence reconciliation and closure;
06:32:27Z explicitly accepted evidence-limited reconnect/mailbox observations
without a dedicated reconnect-window measurement. No new execution, sampling,
test, implementation or residual-boundary work was performed for this edition.

- Source/review foundation: original launcher/profile/Pi-cache implementation
  merged in `74924b4c0`, bounded dev lifecycle repair in `4fdb920c0`, Codex
  REST in `8072be902`, and item 4 live Result in `3345e3588`. Their scoped
  mock/remote gates and independent reviews are recorded in the checkpoints
  below; this edition does not turn them into live measurements or claim a
  broad-suite run. Directional T0 remains the frozen old-history
  read/append/new-reopen proof, not a conclusion from the live percentage.
- Read-only artifact inspection:
  `D:/Temp/daemon-swap-1116c/summary-final.txt` records the coordinator-owned
  300-ms sampler, 06:16:16-06:21:11Z, with the selected swap-minus-30-seconds
  through swap-plus-three-minutes window 06:16:51-06:20:21Z. It records two
  starts of the selected `ws-mcp-dashboard-84a3ccf259151abf.exe`: explicit
  `daemon-start --replace` PID 13204 at 06:17:20.442Z and necessary daemon
  child PID 2404 at 06:17:21.481Z. There is no repeated ws starter storm in
  that observed window, and `codex.exe` count is zero. Python/session and
  unrelated executable starts are not conflated with forbidden ws starts.
  Polling can miss short-lived processes; this is not a kernel event trace
  or proof of zero executable activity outside the recorded window.
- The same artifact records `dev.py rerun` exit 0 after the one-shot starter
  returned `unavailable`, followed by expected-build REST confirmation and
  the recovery diagnostic. This supplies live evidence for the bounded
  dev-wrapper false-failure repair without changing its success criterion.
- Reconnect/mailbox live evidence is deliberately **limited to continued
  operation/delivery after swap**: coordinator reports lead MCP tools kept
  working, and this session's 06:31Z/06:32Z mail exchange was delivered
  through the swapped daemon. No dedicated reconnect-window, latency,
  stream/one-shot matrix, cancellation or request-replay measurement exists;
  none is claimed as a measured PASS. The coordinator explicitly states
  this limitation does not block closure. Deterministic contract tests and
  source review remain their separate evidence, not substituted live data.
- Item 4 live PASS above supplies fresh real-credential dashboard acquisition
  and matching history append with the stable legacy identifier. Earlier
  `daemon-swap-1116` / `daemon-swap-1116b` observations remain historical:
  old in-memory launchers were outside the accepted updated-gate guarantee.
  They are not presented as a clean historical all-process measurement, and
  PID 3452's exact historical admission cause remains unproven.

##### Forwards - approved residual boundaries, no new assignment

- **Item 3: retained old executable/metadata and orphan temp files.** Without
  authoritative process-use/accepted/rollback retention evidence, settled
  artifacts remain intentionally; each copy cleans only its own failed temp.
  Retention itself starts no executable. A safe general sweep would require
  separate retention/process-use design, not age-only deletion. Rough size:
  **medium, 0.5-1 engineering day**; no new ticket or implementation authorized.
- **Item 5: Pi probe on cache miss and five-minute model-cache expiry.** The
  approved success cache reduces repeats but does not eliminate the existing
  Pi/Node probe process on a miss or expiry. Recurrence on this PC was not
  separately measured by this closing review. Eliminating those probes would
  need a new non-process source/refresh design beyond the completed cache
  contract. Rough size: **medium, 0.5-1 engineering day**, source-dependent;
  no new ticket or implementation authorized.
- **Item 1: Electron concurrent-start losing starter.** Updated Python
  launchers share the starter gate; Electron does not. A rare simultaneous
  new-daemon start can still execute one losing ws starter, with Go's
  singleton preventing a second daemon. This is the explicitly accepted
  new-start boundary, not a reproduced new defect. Eliminating it requires
  separate cross-language gate coordination. Rough size: **medium,
  0.5-1 engineering day**; no new ticket or implementation authorized.

These are rough future-scope estimates, not scheduled work or expanded
acceptance requirements. Closure preserves all three approved boundaries.

## Constraints

- Python launcher, holder, and hosted harness process creation remain
  unchanged. `.mcp.json` continues to use the Python launcher.
- Dev-only `go build` bursts and git executable use are outside this ticket.
- A live daemon swap and process-sampling acceptance require the coordinating
  lead's approval before touching the shared daemon. Workers never perform
  that swap, install or replace the shared runtime without approval.
- Never run Go, ws-mcp/ws-mcp-dashboard executables, test binaries or
  executable fixtures on the owner's Windows PC, including local builds
  and runtime smoke checks. Go builds/tests run only on remote w5090 via
  `D:/sys_lock.sh remote w5090 run`, synchronizing this isolated worktree.
  Use focused `-run` families, never broad unscoped package suites. Local
  Python launcher tests must mock all executable spawns; local Node-only
  checks must not launch executable fixtures.
- Work only in the acquired implementation worktree; never commit in or
  switch the shared root's branch. No merge, push or worktree release;
  return branch/head to the coordinating lead and retain the lease.
- Temporary artifacts belong only under an owned `D:/Temp/<purpose>/`
  directory and are cleaned after use. Stop if D: free space is below
  20 GB. Pi-authored commits carry no Co-Authored-By trailer.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Source-only grounding baseline: c32629f72 on
  `impl/develop/down-frame-gooey`, 2026-11-16. No tests, Go, local ws/test
  executables, daemon operations, installs or source edits were performed.
  D: had 154 GB available before inspection. No temporary artifacts were created.
- All three launcher copies currently have SHA256
  `0e137b1d0989d3ffab6f95295957d342474e48ac42efdbf8cc94ee48205f9ddd`
  (verified with `sha256sum agents-plugin/bin/ws-mcp-launcher.py agents-plugin-pi/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py` at 7bdf11e74).
  Preserve their byte equality; ordinary `.mcp.json` Python routing is intentional
  (`agents-plugin/.mcp.json#L3-L8`; `agents-plugin-pi/src/mcp-stdio-client.ts#L362-L377`).
- Mailbox no-start cut points: both Claude paths share `waitArgv`'s Python
  command (`agents-plugin/hooks/claude-mod/register.ts#L281-L289,L525-L531,L588-L589,L1037-L1040`);
  Pi stream and one-shot both spawn Python
  (`agents-plugin-pi/src/mailbox-waiter.ts#L909-L913,L1124-L1128`).
  The hidden start authority is in the launcher, not these outer spawn commands:
  `main` makes cold CLI calls repair/install-capable before constructing a
  start-capable link (`agents-plugin/bin/ws-mcp-launcher.py#L1861-L1911`);
  `proxy_cli` calls `wait_daemon_connection`, whose FIRST attempt calls
  `link.connect` and can reach `start_daemon`
  (`agents-plugin/bin/ws-mcp-launcher.py#L1231-L1260,L1422-L1428,L1764-L1780`).
  Therefore cutting only adapter spawns or only reconnect is insufficient:
  mailbox entry must avoid repair/staging/start authority as well as connect's
  start path. Its subsequent pre-send retry loop already rereads endpoint and
  uses `daemon_upgrade` only; CLI EOF returns an error, leaving adapter backoff
  and respawn to re-enter main (`agents-plugin/bin/ws-mcp-launcher.py#L1244-L1260,L1781-L1787`;
  `agents-plugin-pi/src/mailbox-waiter.ts#L309-L310,L593-L600`).
  Keep `DaemonRequestSent` non-replay and cancellation semantics; MCP recovery
  is a different path (`McpProxy.recover`, launcher #L1612-L1634).
- Startup/absence evidence: `DaemonHung` refuses a no-grace hung probe, but
  `wait_healthy` catches it and can return no health after grace; the latter
  still authorizes start today (`agents-plugin/bin/ws-mcp-launcher.py#L1037-L1067,L1332-L1355,L1407-L1428`).
  The existing Go `dashboard.lock` is held only by the daemon; it is not a
  launcher single-flight lock (`agents-plugin-tool/internal/wsdashboard/endpoint.go#L6-L16`;
  `agents-plugin-tool/internal/wsdashboard/daemon.go#L303-L329`).
  Network silence plus a longer poll alone does not prove a daemon is absent.
  Decisions settle the separate starter lock, conservative hung/live-PID
  policy and updated-launcher-only guarantee; no Electron participation
  interface is introduced.
- Exec3 overlap, read-only branch snapshot `impl/develop/cape-barn-hence`
  at de648159179663a3877162898385b78136b1496c: its
  `agents-plugin-tool/desktop-electron/daemon-selection.js` is file-only,
  returning `{source,binary,contract,metadata,env}` from bundled/dev input.
  Its ready copy of `261115-feat-werkschaft-electron-daemon-override` records
  Phase 1 Result b927532f77 and plans missing-only detached `dashboard` startup
  through the existing singleton lock, not replacement-capable Ensure.
  This checkout still has that ticket in todo with no Result; the branch is
  not landed here (pending 261115-feat-werkschaft-electron-daemon-override).
  Shared contracts are `WS_CACHE_HOME`/dashboard lock, endpoint/REST identity,
  core precedence, metadata hashes, staged executable lifetime, and
  `WS_RSRC_ROOT`/`WS_SKILLS_ROOT`/hostable plugin package. Exec3 retains whole
  out-of-bundle staged packages with no automatic pruning and separately gates
  Windows strict lifetime. This ticket's item 3 must not silently prune that
  namespace or introduce a competing package/start interface. No ordering
  dependency or new strict-lifetime flag is settled by this evidence.
- Related startup work is not a landed prerequisite: todo
  `261116-bug-ws-shared-daemon-launcher-dependent-claude-plugin-root` owns
  Claude package identity independently of Pi resources; todo
  `261115-feat-ws-dev-daemon-accepted-build-recovery` owns accepted-build
  retention/promotion/rollback; todo `261115-feat-ws-daemon-release-auto-update`
  owns user-approved shell release download/swap. Item 3 cleanup cannot infer
  that an unused executable is an unneeded rollback target. Current equal-core
  builds do not automatically replace one another; older numeric cores do not
  replace newer ones (`agents-plugin/bin/ws-mcp-launcher.py#L1349-L1355`;
  `agents-plugin-tool/internal/wsdashboard/ensure.go#L163-L174`). A present-day
  flip-flop still needs causal evidence, not an assumed newer-build tie-break.
- Profile security: item 2 explicitly authorizes adding `tickets.create_empty`,
  not blanket ticket/lead authority. The landed builtin worker floor now includes
  it and mini-lead inherits that floor (`agents-plugin-tool/internal/wsagent/builtin.go`, `PiWorkerWSTools`, `PiMiniLeadWSTools`; commits b5b73efb5 and a995e895f); the tool
  already dispatches to `wsdoc.TicketCreate` under the resolved root
  (`agents-plugin-tool/internal/mcp/server.go#L2046-L2079`). Non-lead agent
  keys are parentless delegates (`agents-plugin-tool/internal/mcp/agent_keys.go#L12-L29`),
  not top-level leads. The explicit allowlist precedes the keyed role gate,
  which alone does not enforce a worker-specific surface
  (`agents-plugin-tool/internal/mcp/server.go#L730-L772,L5694-L5702,L5723-L5737`).
  Preserve scope, keyless/bootstrap exclusions, token depth/tier checks,
  overrides and saved launched-profile subset ceilings. A routine-tool audit
  must name and justify any additional grant; it cannot convert these keys
  into leads or rewrite their parentage. Done
  `261113-feat-ws-pi-mini-lead-profile` previously rejected widening pi-worker
  alone; ready `261115-feat-ws-worker-profile-session-spawn` is a distinct
  no-ticket worker posture; todo `261116-feat-ws-subagent-worktree-lease`
  separately owns worktree-tool widening. None authorizes blanket widening here.
- Compatibility evidence is directional and provisional, not a blanket tier:
  - Unchanged endpoint/health/hello fields and existing calls target T0:
    old/new readers still read schema-1 `endpoint.json`; the daemon atomically
    rewrites its fixed shape (`agents-plugin-tool/internal/wsdashboard/endpoint.go#L48-L94`).
    Unknown additions would be lost on a fixed-shape rewrite, so do not add
    coordination state there without a reader/writer/reopen design. Existing
    protocol/minor refusal remains T3 with no data-reopen remedy
    (`agents-plugin-tool/internal/wsdashboard/proxy.go#L232-L254`); todo
    `261112-feat-ws-mixed-version-compat-guards` does not supply a landed bypass.
    Existing network-only legacy hello discovery remains approved
    (`agents-plugin/bin/ws-mcp-launcher.py#L1311-L1330`), preserving compatible
    older daemon use without executable inspection.
  - Keep schema-1 `<binary>.metadata.json` identity and schema-2
    `.compatibility.json` meanings: old-style metadata writers copy identity
    members, then bind actual bytes; stamp writers replace a fixed payload.
    Reopen rehashes bytes and recomputes the stamp, not lost arbitrary fields
    (`agents-plugin/bin/ws-mcp-launcher.py#L196-L240,L300-L349`). T0 is grounded
    for unchanged shapes, not for extra fields or an unchosen stable-path layout.
  - Existing `plan-usage.json` fields are `claude/gpt` and typed
    harness/plan/source/status/asOf/windows entries; old-style decode/save
    loses unknown members and new reopen has no raw recovery
    (`agents-plugin-tool/internal/wsdashboard/plan_usage.go#L18-L67`). Preserve
    those shapes for a T0 source-selection repair. `plan-usage-history.json`
    schema 1 preserves unknown fields but validates source values and refuses
    newer schemas/unknown source semantics before normal use; a new source
    label is not automatically T0 (`agents-plugin-tool/internal/wsdashboard/plan_usage_history.go#L13-L85,L128-L204`).
  - Decisions settle the private Pi cache and directional T0 evidence; keep
    strict Profile (`agents-plugin-tool/internal/wsagent/profile.go#L218-L237`)
    and nested `resolved_definition` unchanged.
- Probe evidence: the coordinator explicitly retained items 4 and 5.
  `queryPi` reads Pi OAuth credentials only
  when Codex login presence is absent, never as a selected Codex-credential REST
  source (`agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go#L30-L43,L178-L222`).
  That detection alone supplies no active-session predicate
  (`agents-plugin-tool/internal/wsdashboard/plan_usage.go#L134-L165`);
  Decisions select the existing session-registry liveness definition instead.
  Persistent Pi identity must account for command wrappers/PATH, not assume
  the version and model probes resolve the same file
  (`agents-plugin-tool/internal/wsagent/pi.go#L57-L64,L1251-L1257`). Related idea
  `261114-bug-ws-pi-version-probe-never-refreshes` already records stale
  once-per-process/PATH selection; related idea
  `261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe` records real-home
  fixture exposure, not permission to use live credentials.
- Scoped verification suggestions only (none run): extend mocked launcher
  `main`/`proxy_cli` cases in `agents-plugin/tests/test_ws_mcp_launcher_proxy.py`
  for cold/missing/stale/refused/hung endpoints, swap, stream EOF and one-shot
  re-arm with staging, repair and subprocess APIs forbidden. Assert zero ws
  execution for every CLI mode, plus separate permitted session-MCP/lifecycle
  starts. Add concurrent-process fake-starter lock tests with bounded time;
  retain initialize-only replay, request IDs, UTF-8, fail-open Stop and abort.
  Existing Pi `test/mailbox-daemon.integration.test.ts` covers warm fake-daemon
  routing, not unavailable-daemon no-start; its Windows script sentinel is not
  proof of zero executable fallback. Use injected spawns there and in Claude
  mod stream/fallback tests; keep actual `.mcp.json` Python routing.
  Remote w5090 only: focused `TestStageExecutableCopiesAndPrunes`, Ensure
  concurrent-start/core/LAN cases, Pi builtin/subset/snapshot/restart fixtures,
  `TestPiToolsGate`, `TestMailboxWaitCommandRenderVariable`, and plan-usage
  cadence/expiry/redirect/non-destruction fixtures. Some plan-usage tests copy
  and execute the Go test binary as fake codex
  (`agents-plugin-tool/internal/wsdashboard/plan_usage_probe_test.go#L79-L110`):
  NEVER run these locally. Isolate HOME, CLAUDE_CONFIG_DIR, CODEX_HOME,
  WS_CACHE_HOME and WS_CONFIG_HOME; use only owned D:/Temp/<purpose> artifacts
  locally and clean them. Item 7 process sampling and any shared swap remain
  coordinator-owned acceptance, not a fact-population command.

## Implementation checkpoint (not phase acceptance) - 2026-11-16

- Item 2 landed in `b5b73efb5`; item 5 in `4cc15f9ab`. Item 4 is parked
  at `parked/exe-phase1-item4-codex` (`d9fd94543`) and fully reverted by
  `36ecc6e43`; neither the Pi cache nor launcher changes depend on it.
- Pending launcher work adds a persistent OS starter lock, a 20-second
  swap grace, conservative recorded-PID absence admission and network-only
  CLI entry/retry paths. All subprocess APIs in the local proxy family are
  mocked, including permitted starters. No local ws/Go/test executable ran.
- Coordinator-approved conservative cleanup: each failed Python copy removes
  only its own random temp path. Go's existing pruning hook now retains all
  settled artifacts: this baseline has no authoritative accepted/rollback
  retention source and singleton ownership does not prove that the previous
  draining process has exited. Old executables, metadata, orphan temps,
  directories and symlinks deliberately remain; no retention index/API or
  Electron package pruning is invented.
- Verification: new mocked starter contract family passed (10 tests, one
  remote-process fixture skipped). Existing mocked proxy family: 56 of 58
  tests pass; `test_a_daemon_that_dies_with_no_successor_is_restarted` and
  `test_recovery_repairs_missing_runtime_only_when_start_becomes_needed`
  still fail waiting for the successor hello (5/8 seconds). A first correction
  isolated fake PID admission, removed obsolete CLI-start expectations and
  made timeout/symlink fixtures independent of Windows environment. The two
  reconnect failures repeated afterward: worker stop (f), no further fix
  attempt before checkpoint. Full commands/output are retained in machine-local
  `D:/Temp/exe-phase1-launcher/proxy-failure-{1,2}.txt`. Their
  implementation-versus-fixture blame remains unresolved at this checkpoint.
- The first new-family failure was a fixture reading a Windows-locked byte;
  reading after unlock fixed it. The proxy family's first run also exposed
  restricted Windows symlinks and an environment-dependent HTTP timeout;
  those fixture issues were corrected. No passing full-proxy claim is made.
- Item 5 focused remote wsagent family passed (0.158s, full output read by
  its implementer). Item 2 focused MCP render family passed (0.195s); its
  original wsagent output was truncated and deleted before full review, so
  that older run is not verified-pass evidence. The cmd re-arm fixture is
  updated but unrun under the internal-package-only command authorization.
- Coordinator 02:14:46Z authorized bounded same-branch continuation after this
  checkpoint: first diagnose whether the changed health grace exhausts the
  fixture deadline; production health grace must not be shortened to fit tests.
  Larger changes require a new report. Remaining: diagnose the repeated
  reconnect failure, focused remote starter/staging/profile gates, independent R1 and
  substantive fixes-only R2 if needed. No reviewed HEAD or phase Result yet.
- Coordinator alone owns subsequent merge/build/install/swap and live process
  sampling. Acceptance preparation after reviewed code: sample
  `Get-CimInstance Win32_Process` at 250-ms intervals into an owned
  `D:/Temp/exe-phase1-acceptance/` CSV before/during/after one coordinated swap;
  exercise session MCP reconnect plus mailbox stream/one-shot and ordinary
  CLI proxies, and inspect startup count and command lines. Expect one
  updated-launcher starter plus necessary daemon child; record the accepted
  rare Electron losing-starter boundary separately. Polling can miss very
  short-lived processes and is not a kernel process-start trace. No such
  command, swap or install has been executed by this worker.

### Bounded continuation evidence - 2026-11-16

- Confirmed diagnosis: with fixture grace `0.02s` and retry budget `0.3s`, a
  closed Windows localhost port raised `DaemonHung` after `0.0529s`, before
  any starter ran. The 20-second production grace was already test-overridden;
  the first PID-only fixture correction did not address this handshake/read
  timeout conflation. Coordinator 02:17:32Z approved separating connect
  failures/timeouts (unreachable) from established-connection timeouts (hung),
  retaining recorded-PID vetoes. Reconnect fixtures now use a short socket
  budget; production swap grace is unchanged.
- Mocked local gates passed: starter family 12 tests (one remote-only process
  fixture skipped), proxy family all 58 tests. A final pipe-fixture close
  removed ResourceWarnings; its affected one-test gate passed without warnings.
  Virtual-clock cases cover unreachable at t0, successor at t4 inside the
  20-second grace with zero starts (initial/reconnect), and no successor with
  exactly one start only after grace expiry.
- Remote w5090 focused `^Test(LauncherStarterSingleFlight|StageExecutable|Ensure|KeepRunningCorePolicy)`
  passed (1.226s), including six competing Python processes, crash lock release,
  failed-copy cleanup, conservative retention, numeric-core and LAN cases.
- Remote w5090 focused builtin/subset/saved-snapshot/old-writer family passed
  (0.190s). Its first run correctly exposed a test-fixture mismatch: the
  directional old-pruner fixture called today's now-conservative hook. Freeze
  the pre-change prune policy inside that compatibility fixture so it still
  proves actual old-write -> new-reopen; do not restore unsafe production
  deletion to satisfy the old assertion.
- Node-only injected-spawn Pi stream/fallback/EOF family passed (6 tests).
  Outer Claude/Pi wait commands remain Python; the launcher cuts their hidden
  start authority centrally. No executable integration fixtures ran locally.
- All three launcher files are byte-identical. Item 4's five source/test files
  remain byte-identical to the planning base after the explicit revert.
- Independent R1 is next; this continuation still records no phase Result,
  ticket closure, live swap, install, merge, push or acceptance success.

### Independent R1 and remediation - 2026-11-16

- R1 reviewed `8a8e52498..236ca99dd`, reporting no Critical and two Important:
  R1-I1 (Windows fresh-file initialization before byte-lock ownership) and
  R1-I2 (native PID overflow escaping the conservative unknown-PID guard).
  Full report: machine-local
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@a80733aa/review-paths/0d710944-01-exe-phase1-r1.md`.
- R1-I1 fix: initialize byte zero only after successfully taking its advisory
  lock. A deterministic mocked Windows contender regression forbids any
  initialization write without lock ownership; the native Windows unit case
  also exercises acquisition beyond EOF. The persistent file is neither
  truncated nor unlinked and its descriptor remains non-inheritable.
- R1-I2 fix: native PID-conversion overflow means unknown, never dead. Both
  platform helper paths and the controlled MCP-unavailable/zero-start path
  have explicit overflow regression cases, with native PID APIs mocked.
- Focused fix gates passed: local mocked starter family 15 tests (one
  remote-process fixture skipped), remote w5090 `^TestLauncherStarterSingleFlight$`
  passed (0.909s). The three launcher copies remain byte-identical, SHA256
  `0e137b1d0989d3ffab6f95295957d342474e48ac42efdbf8cc94ee48205f9ddd`.
- The routed lite allocation permits no worker-owned re-review. The lead
  dispatches independent fixes-only R2 for these substantive fixes; no
  reviewed-HEAD claim is made before that closure. Live acceptance remains
  separate, coordinator-owned, and unexecuted; ticket stays open without Result.

### Reviewed source boundary and coordinator handoff - 2026-11-16

- Reviewed code HEAD: `79841d02677a075cd3b55d31df8e1fe3af0f45fd`.
  Independent lead-dispatched fixes-only R2 PASS closes R1-I1 and R1-I2;
  no substantive fix-introduced issue or new Forward observation remained.
  Full report:
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@a80733aa/review-paths/6e57526d-01-exe-phase1-r2.md`.
  R2 inspected the exact `236ca99dd..79841d026` fixes and full focused evidence;
  it did not claim a fresh broad sweep or live acceptance. This handoff commit
  changes documentation only; reviewed source remains exactly that code HEAD.
- Source/verification boundary: original items 1/2/3/5 and 6-prep are implemented
  and independently reviewed, with the focused evidence above. Full suite,
  native Windows multiprocess sampling and coordinated swap acceptance remain
  unperformed. Windows contention/PID overflow have deterministic mocked
  coverage; the actual multi-process/crash-release gate ran only on w5090.
- Preserve machine-local evidence in `D:/Temp/exe-phase1-launcher/`:
  `proxy-failure-1.txt`, `proxy-failure-2.txt`, `continuation-evidence.txt`, and
  `r1-fix-evidence.txt`. These are small full-output records, not build caches.
- item 4 deferred to follow-up by owner direction 10:56
  Its retained parked branch is `parked/exe-phase1-item4-codex` at `d9fd94543`;
  final item-4 source/test diff against planning base is empty. Do not begin
  that follow-up before coordinator live-acceptance results.
- Coordinator owns release of this leased worktree, integration, remote build,
  install and one shared-daemon swap. The worker performed none of them.
  Keep this ticket in ready, without a phase Result or closure, until the
  coordinator supplies actual live-acceptance evidence.

#### Prepared acceptance procedure (coordinator only; not executed)

1. Approve/select the integrated build and updated launcher copies, using the
   coordinator's remote build/install procedure. Record pre-swap `/health`
   identity and the endpoint PID; do not execute a binary to inspect identity.
2. In a separate PowerShell console, capture a 90-second process sample:

   ```powershell
   $out = 'D:/Temp/exe-phase1-acceptance'
   New-Item -ItemType Directory -Force $out | Out-Null
   $until = (Get-Date).AddSeconds(90)
   $rows = [System.Collections.Generic.List[object]]::new()
   do {
     $seen = (Get-Date).ToUniversalTime().ToString('o')
     Get-CimInstance Win32_Process |
       Where-Object { $_.Name -like 'ws-mcp*.exe' } |
       ForEach-Object { $rows.Add([pscustomobject]@{
         SeenAt=$seen; PID=$_.ProcessId; ParentPID=$_.ParentProcessId
         Created=$_.CreationDate; Path=$_.ExecutablePath; Command=$_.CommandLine
       }) }
     Start-Sleep -Milliseconds 250
   } while ((Get-Date) -lt $until)
   $rows | Export-Csv "$out/processes.csv" -NoTypeInformation -Encoding utf8
   ```

3. Perform exactly one coordinator-approved swap while session MCP launchers
   reconnect. If using REST shutdown, read the active endpoint privately and
   send `POST http://127.0.0.1:<port>/shutdown` with its bearer token; allow the
   selected session MCP launcher or explicit lifecycle action to start the
   successor. Never log the token. Mailbox/ordinary CLI proxies are not starters.
4. Exercise existing Claude/Pi mailbox streams, one-shot fallback/re-arm and
   ordinary CLI proxies during the window. With approved plugin/key variables,
   the safe CLI form is `python3 "$plugin/bin/ws-mcp-launcher.py" tools` and
   `python3 "$plugin/bin/ws-mcp-launcher.py" mailbox wait --session-key "$key" --timeout 1s --format json`;
   stream adds `--stream` and is cancelled after observation. Read successor
   identity through `/health` and correlate distinct PID/creation-time rows.
5. Expect one updated-launcher starter invocation and its necessary daemon
   child, zero ws executions from mailbox/ordinary CLI/probe paths. Record the
   approved rare Electron losing-starter exception separately. A 250-ms poll
   can miss short-lived processes; report that measurement limit. Preserve
   only small acceptance evidence and remove owned temporary artifacts later.

## Live-acceptance repair source checkpoint - 2026-11-16

- Reviewed code HEAD: `d286b8eaf9fffc4dbfb2ae1ab86096de8e6d7a05`.
  Only `scripts/dev.py` and `scripts/test_dev.py` changed from planning HEAD
  `04e06b7b5fc080d824fb3b0d4a835bcf8e90f188`. The existing launcher gate is
  reused without a cross-module interface change; builds, seeding and staging
  finish outside it. One 60-second lifecycle deadline covers lock contention,
  explicit `--replace`, and expected-content-hash REST confirmation. Only
  parsed exit 1 / `outcome: unavailable` permits nonzero reconciliation;
  diagnostics remain visible and no second starter is invoked.
- Focused local command (all executable/PID/network boundaries mocked):
  `TMP=D:/Temp/exe-dev-repair TEMP=D:/Temp/exe-dev-repair DEV_TEST_TMP=D:/Temp/exe-dev-repair PYTHONDONTWRITEBYTECODE=1 python -B -m unittest scripts.test_dev.DevLifecycleTest -v`
  passed all 12 tests (0.593s); full output read at
  `D:/Temp/exe-dev-repair/dev-lifecycle-1.txt`. Actual advisory-lock checks and
  virtual clocks cover a dev-held gate through MCP contention beyond the
  production 20-second grace and successor publication at t25 with zero extra
  starters, shared-budget expiry, terminal release, explicit replacement,
  unavailable recovery, malformed/unrelated failures, old/no/PID-only health,
  late confirmation and hung responses. No executable fixture files were
  created by this focused family. This is not a full-suite or live proof.
- Independent focused R1 returned CLEAN for
  `04e06b7b5..d286b8eaf`, with no Critical, Important or Minor findings.
  Full report:
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@392ca909/review-paths/61cdf1a0-01-exe-dev-repair-r1.md`.
  No R1 fixes were required, so the conditional substantive-fix R2 does not
  apply. The reviewer read source and the supplied full test log; it ran no
  tests or live operations. This documentation checkpoint changes no reviewed
  source inputs.
- No Go/ws/test executable, launcher CLI, native PID query, build, install,
  runtime probe, daemon swap, sampling, merge, push or release ran. Temporary
  test directories were cleaned; only the small full-output record remains.
  Original accepted Phase 1 source, retention policy and the old in-memory
  launcher boundary are unchanged. PID 3452's historical cause is not proved.
- Ticket remains open without a phase Result: the coordinator owns integration
  and actual live acceptance. Stop before those live actions. Item 4 REST must
  be planned afresh after this repair lands; the parked CLI-admission branch
  is not a REST implementation and was not applied.
- Coordinator direction at 03:22:51Z folds duplicate
  `261116-bug-ws-dev-rerun-daemon-false-failure` into this repair. It is closed
  as absorbed, with a Resolution naming the reviewed code, focused tests and
  live-acceptance limit, so it cannot be assigned twice. Its unexecuted
  standalone phase warning is explicitly accepted for duplicate absorption,
  not treated as independent phase completion. No package-root/env-selection
  interface changed; the existing dev package selection is retained.

## Fresh item 4 REST reviewed source checkpoint - 2026-11-16

- Reviewed code HEAD: `8f6a42a3d3d87be1e5e1926b19f2183503d94278`,
  range `f3f8a4259076fddfb900127b52638507d758d40f..8f6a42a3d`.
  Core acquisition/HTTP-only fixtures landed in `4c50ad056`; the independent
  frozen-history fixture landed in `8f6a42a3d`. Only fresh Phase 1 item 4 is
  delivered here; earlier lifecycle/profile/Pi-cache/dev-repair work is unchanged.
- Codex acquisition now uses the fixed passive HTTPS wham GET on every OS,
  with selected file-only ChatGPT OAuth access/account headers. Detection and
  CODEX_HOME precedence are unchanged. Selected Codex failure never selects
  Pi; explicit ChatGPT ignores API-key material, while legacy mixed documents,
  malformed credentials, empty access, invalid headers and known expired exp
  fail closed. Unknown expiry remains server-authoritative; JWT decoding is
  not authenticity. No refresh/login/credential write, alternate resolver,
  configurable origin, cookie or executable fallback was added. Caller-free
  command helpers and copied-executable/protocol/batch fixtures were removed.
- Shared HTTP parsing keeps raw seconds: Codex accepts exactly 604800, while
  Pi retains seconds/60 classification. Both retain bounded HTTP/parse and
  generic errors; redirects remain forbidden. Completion-based 5-minute /
  forced 5-second cadence, detached 10-second budget, query coalescing,
  measurements/source/asOf retention on failure and history-on-success remain.
  `currentUsage`, `pushClaude`, Claude windows and UI/assets were not edited;
  preserve develop's independently landed absent-kind fix during integration.
- Directional T0 source proof: new `plan_usage_probe.go:17-19,96-136` writes
  the same `via Codex CLI` identifier, explicitly documented as legacy
  CLI-owned-credential provenance rather than subprocess transport. Latest
  fields/read/write remain `plan_usage.go:18-67`; old and new write identical
  shapes. Old history source validation/read and append preserve that literal
  and supported extensions (`plan_usage_history.go:44-85,128-190`, unchanged
  from planning HEAD). The independent frozen copy in
  `plan_usage_compat_test.go:60-237` retains the old types, literal allowlist,
  validation, pruning and append implementation. Its test at `:239-327`
  stores an actual new REST sample, successfully reads/appends with the frozen
  old writer, round-trips latest through the old writer, and reopens with new
  code: prior/REST/old-appended samples, source, windows, asOf and extensions
  survive. This proves old read/write usability and new reopen, not merely
  recovery after an old-reader refusal or latest-only non-destruction.
- Final focused gate, remote w5090 only:
  `go test ./internal/wsdashboard/ -run "^Test(PlanUsage|DaemonExplicitHomeDoesNotQueryUsage)" -count=1 -v`
  passed (0.290s) at reviewed code HEAD; full output read at
  `D:/Temp/codex-rest-impl/final-focused.txt`. The isolated history pair also
  passed (0.022s), full output at `history-compat-2.log` in that directory.
  Remote HOME/CODEX_HOME/CLAUDE_CONFIG_DIR/WS_CACHE_HOME/WS_CONFIG_HOME/TMPDIR
  were owned scratch paths, cleaned after use; only fake HTTP/test homes were
  used. HTTP/auth-negative fixtures cover credential selection, optional exp,
  all header byte boundaries plus valid opaque values, exact weekly windows,
  status/timeout/redirect/size/parse failures, no-Pi-fallback, cache/history
  retention, detached viewer completion and cadence/coalescing.
- First core gate failed during dependency setup, before behavioral tests:
  importing httpguts pulled unsummed x/text dependencies. Replaced that choice
  with a private RFC field-value validator instead of expanding modules/sums;
  coordinator accepted the correction. It permits HTAB and valid opaque bytes,
  rejects CR/LF/NUL/DEL/other CTL, and returns only generic diagnostics. Later
  focused gates passed. The fixture leaf's first attempt stopped at a gofmt
  blank-line check, not a behavioral assertion; correction and focused rerun
  passed. Small full failure logs remain `core-test-1.txt` and
  `history-compat-1.log`; no repeated behavioral failure occurred.
- Independent xlarge R1 Lite returned CLEAN for the exact range above: no
  Critical, Important or Minor findings. Full source-only report:
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@a80733aa/review-paths/e47ea0f6-01-codex-rest-r1.md`.
  The reviewer independently compared the frozen old history implementation
  and read the full focused logs; it ran no tests/live operations. No R1 fix
  was needed, so substantive-fix-only R2 does not apply. The handoff commit
  changes ticket documentation only; reviewed source stays at that code HEAD.
  Ticket verification passes with the Sage freshness advisory explicitly
  accepted: this checkpoint adds implementation/review evidence only and
  changes none of the settled Decisions, route facts or phase plan.
- No local Go/ws/launcher/test executable, executable fixture, authenticated
  upstream call, credential inspection, build/install/runtime/PID query,
  daemon swap, process sampling, merge, push or release ran. D: had 152 GB
  available. Only small evidence logs remain in the owned temp directory.
  Ticket stays in ready without a full Phase Result or closure: coordinator
  integration and actual live acceptance remain pending; the lease is retained.

## Forward

- Claude overlap rechecked against current develop, not assumed from this
  branch's older baseline: `7bdf11e74..develop` changes `pushClaude` to remove
  absent kinds only from complete valid newer usage input (the separately
  owned fix reported in f3a5fdf46). Item 4 leaves that function, `currentUsage`
  and UI untouched; preserve that independent fix when integrating this
  branch's narrow Codex/store-test changes. No remaining Claude defect is
  asserted from the historical baseline observation.
- Live acceptance retained old executable/metadata pairs and pre-existing
  orphan temp files intentionally: the approved baseline has no safe
  process/accepted/rollback retention authority, so its pruning hook retains
  settled artifacts. Failed-copy cleanup owns only that copy's new temp;
  no historical sweep or new deletion policy is promised by this repair.
- PID 3452's historical extra-start admission lacks a captured decision/health
  trace. Repair the proven dev gate bypass without claiming that the CSV
  proves its exact cause; repeat acceptance remains coordinator-owned.
- Coordinator-approved boundary: old launchers and Electron do not share the new starter lock; a rare Electron race can execute one losing starter, with Go's singleton preventing a second daemon.


## Resolution (2026-11-16)

Closed under coordinator authorization at 2026-11-16 06:31:41Z, clarified 06:32:27Z. The closing Result Edition (3345e3588) combines item 4 real-credential REST/history PASS with the inspected 06:17Z swap artifact and explicitly evidence-limited continued MCP/mailbox operation. It preserves the three approved residual boundaries (retained staged artifacts, Pi cache-miss/TTL probes and Electron losing starter) with rough future-scope estimates; no new work or tickets are authorized. No dedicated reconnect-window measurement or blanket all-executable guarantee is claimed.
