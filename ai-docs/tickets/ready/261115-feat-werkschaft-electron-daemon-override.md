---
title: "Electron standalone daemon start and replacement without stopping session holders"
related:
  261115-feat-werkschaft-electron-shell: "attach-only first slice and planned missing-daemon start"
  261115-feat-ws-daemon-release-auto-update: "separate user-approved release download and swap; reconcile ownership"
  261115-feat-ws-dev-daemon-accepted-build-recovery: "development accepted-build recovery and rollback follow-up"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 4bf35c6c91796d9c
sage-review-design-reviewed: 4bf35c6c91796d9c
---

# Electron standalone daemon start and replacement

## Background

The owner requires Electron itself to start **and** replace the daemon
standalone, without a plugin session or `scripts/dev.py` starting it first
(2026-11-16). The existing shell only discovers an endpoint and connects; on
failure it asks the user to start the dashboard normally
(`agents-plugin-tool/desktop-electron/endpoint.js:8-75`,
`agents-plugin-tool/desktop-electron/connection.html:14-19`). The earlier
(1115) override decision reverses the shell's attach-only rule after v0.50.0;
(1124)=b schedules it through the Electron auto-updater.

## Decisions

- **(1196)=c:** Electron launches the daemon binary bundled with the app by
  default. A setting can select a development build for dogfooding. Neither
  path depends on an installed plugin runtime.
- **(1197)=b:** The newer numeric core wins between Electron and plugin
  daemons. If the bundled core is older, Electron attaches rather than
  replacing it. For equal cores retain the existing rule: ordinary connection
  keeps the daemon; explicit `--replace` requests one swap, regardless of build
  time or hash. A newer core is never downgraded
  (`agents-plugin-tool/internal/wsdashboard/ensure.go:40-53,89-107,163-174`).
  This monotonic core precedence prevents cross-core replace loops and avoids
  replacing a newer-minor daemon with an older one. It does **not** remove
  existing cross-minor hello refusals
  (`agents-plugin-tool/internal/wsdashboard/proxy.go:232-254`).
- **(1198)=a:** The daemon always outlives Electron; app exit and daemon
  shutdown are separate. The daemon stops only through explicit shutdown,
  including the authenticated drain for a designated replacement. On Windows,
  the current job-object breakaway fallback can leave it inside the app's job
  and kill it with the host; a phase must handle that risk, not silently claim
  the lifetime guarantee (`agents-plugin-tool/internal/wsdashboard/spawn_windows.go:12-34`).
- **(1124):** Replacement never stops, signals or restarts session holders;
  their PIDs remain the same and sessions detach and reconnect. Holders exit
  only when their sessions end. This applies to standalone replacement too.
- **Compatibility target T0** for this feature: retain existing protocol,
  endpoint, lock and store meanings and transparent old-client behavior. The
  replacement itself has no independent tier; grade each affected call and
  store path directionally, including recovery, before release. Existing
  cross-minor hello refusal is a **T3 boundary**, not evidence of transparent
  cross-minor compatibility. Any new T3 boundary requires a minor release and
  explicit owner approval; T4 must not ship
  (`ai-docs/manuals/release-compatibility.md:27-51,80-91,108-119`).

- **Development selection (technical, delegated by the coordinating lead):**
  select an existing hash-bound binary and its matching runtime contract,
  resources and skills. Selection never builds, downloads, or promotes a build
  to accepted status. Explicitly selecting a candidate authorizes its launch,
  not background accepted-build recovery. The accepted-build ticket continues
  to own promotion, retained accepted state, observation and rollback. Rejected:
  inventing a second accepted-build state machine in the shell while that
  ticket's ownership and authorization remain unsettled.

- **Updater coexistence (announced default, owner board 2026-11-16):** retain
  the separate user-approved release-download daemon updater alongside
  shell-bundled updates, derived from (867)/(868). Numeric-newer core wins;
  equal-core replacement requires an explicit swap. This ticket implements no
  downloader. Rejected: implicitly retiring the already-confirmed updater.
- **Cross-boundary replacement (announced default, owner board 2026-11-16):**
  reuse (868): one confirmation lists affected sessions and plugin-update steps
  before a bundled/dev swap across a minor/protocol boundary. Cancel attaches
  to the existing daemon. If attached hello versions cannot be discovered,
  fail closed for the automatic swap and show the reason; add no new endpoint.
  Rejected: automatic replacement silently locking out existing sessions.
- **Preserve running LAN mode (review-raised technical decision):** Electron
  ordinary connection must not reconfigure LAN. Existing Ensure treats an
  equal-core LAN-mode change as a replacement request (`ensure.go:163-174`).
  No inherited unrelated dashboard manifest may convert attach into a drain.
  Merely omitting that manifest is insufficient: daemonStartLAN can fall back
  to a different global LAN value after a failed probe (`cmd/ws-mcp/proxy.go:341-349`).
  The Phase 2/3 handoff must preserve a concurrently discovered equal-core
  daemon's LAN mode even after transient health failure; pin this race in a
  focused test before connecting the shell to a replacement-capable starter.
  If satisfying it needs another cross-module interface, obtain approval first.
  Rejected: claiming an early health probe alone guarantees mode preservation.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- All work, ticket promotion included, stays in the leased full worktree on
  `impl/develop/cape-barn-hence`. The coordinating lead owns integration; never
  change the shared root checkout's branch, commit there, merge or push.
- Go builds/tests run only through sys_lock on w5090. No local Windows Go
  test/build or ad-hoc test executables: corporate security flagged a local
  test-built fake codex.exe. Node checks may run locally without spawning such
  executables. Before any local Windows binary acceptance, ask the coordinating
  lead with the exact command; cross-compile on w5090 and fetch only under
  `D:/Temp/exec3-daemon/`, never the default Windows TEMP directory.
  Use scoped checks only. Remove temporary artifacts after use; stop when D:
  has less than 20 GB free.
- **Deferred owner confirmation:** additive Windows CLI opt-in
  `daemon-start --require-independent-lifetime` is recommended but not yet
  authorized (coordinating lead, 2026-11-16). Readiness and independent phases
  may proceed. Isolate that implementation step; do not introduce the flag,
  change plugin fallback, or claim verified Windows independent lifetime until
  the owner confirms its interface. Alternatives on the owner board: environment
  seam or no change. Strict-launch failure must be detected before draining a
  healthy predecessor, not discovered only by Ensure's post-drain Spawn call.
  Until that mechanism is authorized and implemented, Windows start/replacement
  must fail closed with an actionable lifetime-unavailable reason; read-only
  attach remains available. This is an explicit unimplemented acceptance gate,
  not permission to use unsafe fallback or to close the ticket. Phase 1 and
  portable independent work remain executable without resolving the gate.
- Lifecycle tests use an isolated absolute WS_CACHE_HOME and non-shared port;
  never start, replace, stop or shut down the shared daemon on port 29417.
- No new release downloader, accepted-build registry, recovery promotion,
  rollback authorization, MCP tool, hello field or shared store schema in this
  ticket. Do not claim reconnect repairs an existing protocol/minor refusal.
- Packaged Windows/macOS acceptance is a real completion gate. Missing access to
  a platform is reported as an unverified boundary, not a passing simulation.

## Compatibility Evidence

This is a source-backed T0 target, to be rechecked against the implementation;
no new T3 boundary is authorized.

| Direction / surface | Existing reader, writer and recovery evidence | Required result |
|---|---|---|
| New shell starts a daemon; old launcher reconnects | Existing daemon writes the same endpoint fields atomically (`internal/wsdashboard/endpoint.go:48-94`); both Go endpoint readers and the shell consume identity without rewriting it. Shutdown is authenticated with the retained token (`client.go:53-69`). | Preserve endpoint, lock, health and authenticated-drain meanings. No endpoint migration or new field required. |
| New starter / old starter | Existing `daemon-start` parses flags before constructing the server or invoking Ensure (`cmd/ws-mcp/proxy.go:365-374`). New optional strict lifetime behavior must leave the absent-flag path unchanged; an old selected binary must reject the unknown flag before any drain. | T0 additive CLI behavior for existing callers; unsupported strict-lifetime candidates fail explicitly, never fall back to unsafe launch. |
| Old compatible session calls new daemon after a swap | Hello still checks proxy protocol and same major.minor (`internal/wsdashboard/proxy.go:232-254`). Launcher replays initialization and unsent calls, not already-sent requests (`agents-plugin/bin/ws-mcp-launcher.py:1636-1662`). | No MCP tool or argument removal, protocol change or handler-store change. Report uncertain in-flight outcomes; do not call reconnect a rollback. |
| Old-minor session meets new-minor daemon | Existing hello rejects before handing the session to the handler (`internal/wsdashboard/proxy.go:232-254`); no handler/store write occurs and reopening cannot repair the refusal. | Existing T3 boundary remains; warning/confirmation or refusal must precede a shell-triggered cross-boundary swap. |
| Old daemon / new daemon shared holder records | Shutdown detaches only (`internal/wsdashboard/daemon.go:653-695`); Manager.Close closes attach connections without killing holders, and Scan reattaches from the same descriptors (`internal/wsholder/manager.go:235-264`). | No descriptor/store format change; preserve holder and child PIDs across compatible replacement. Reopening the new daemon reads the same retained records. |
| New shell selection and staged provenance / old shell or launcher | New shell-only files must use distinct app-user-data names; existing launchers do not read or write that namespace. Binary metadata remains schema 1 with a SHA256 binding (`agents-plugin/bin/ws-mcp-launcher.py:196-240`); optional protocol/inventories follow the existing gate (`:730-752`). | No shared accepted-build writer or config-manifest field. Old readers neither overwrite nor lose these private files; new reopening validates retained bytes again. |

Go paths above are relative to `agents-plugin-tool/`.

## Prior Decisions

- 481f0f765 (2026-11-16, commit): "Record owner decisions for bundled daemon selection, core precedence, and independent lifetime." — bearing: supports
- b820cd188 (2026-11-15, commit): "Record Electron daemon override decision for after v0.50.0." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Electron shell, launcher, daemon and compatibility paths named in the phases: `agents-plugin-tool/desktop-electron/`, `agents-plugin/bin/ws-mcp-launcher.py`, `agents-plugin-tool/cmd/ws-mcp/proxy.go`, `agents-plugin-tool/internal/wsdashboard/` |
| scope.surface | cross-module | Electron package, plugin launcher and Go daemon handoff are coordinated; no single-module change |
| scope.new_public_symbol | yes | Proposed opt-in `daemon-start --require-independent-lifetime` is a separately gated step awaiting owner confirmation; existing callers retain their defaults. |
| scope.new_type_contract | yes | Shell-local development selection records an existing binary and matching resource/skills/contract root; no shared ws manifest or protocol field. |
| scope.test_surface | existing | Existing Electron tests include `agents-plugin-tool/desktop-electron/test/endpoint.test.js` and `main.test.js`; daemon Ensure/daemon-start tests exist in their Go packages. |
| complexity.reuse_points | confirmed | Existing `daemon-start`/`wsdashboard.Ensure` supports absent-daemon startup, authenticated drain, core precedence and bounded successor verification (`agents-plugin-tool/cmd/ws-mcp/proxy.go:354-420`, `agents-plugin-tool/internal/wsdashboard/ensure.go:89-160`). |
| complexity.side_effect_risk | high | Starts and drains processes, changes the daemon answering a shared cache root, and must preserve holder lifetimes. |
| risk.correctness | high | Incorrect precedence or handoff can cause replacement loops, stale endpoint use, or loss of live daemon availability. |
| risk.fit | moderate | The feature extends the Electron shell and existing daemon lifecycle, but spans distinct package/module responsibilities. |
| risk.test | high | Required coverage includes packaged Windows/macOS behavior, concurrent starts, reconnect, held-process continuity and app-exit survival. |
| risk.security_or_contract | high | Binary provenance, resource/skills environment, authenticated shutdown, compatibility boundaries and cross-minor refusal all constrain the contract. |

## Phases

All lifecycle tests use isolated `WS_CACHE_HOME` and isolated daemons, never
replace the shared daemon.

### Phase 1: Provenance and compatibility specification

Specify bundled and selected-dev binary provenance, selection and validation;
port the relevant Python launcher compatibility gate (hash-bound metadata,
version, protocol and capabilities) without requiring Python or an installed
plugin. Specify how the packaged app supplies `WS_RSRC_ROOT` and
`WS_SKILLS_ROOT`, including dev selection and failure reporting
(`agents-plugin/bin/ws-mcp-launcher.py:134-150,730-755,841-863`). Trace
old/new call and store directions and recovery against the T0 target; stop for
approval if a higher tier is needed. Reconcile the open updater and recovery
questions before implementing affected paths. Deliver the validation module and
focused fixture tests without executing a binary; do not wire a pending Windows
CLI interface in this phase. Specify package input layout and the shell-local
setting so the next phase consumes a concrete contract.

### Phase 2: Standalone missing-daemon start

Start the selected binary when no daemon is available, using the existing
`daemon-start`/`Ensure` handoff and endpoint/health verification; handle races,
hung daemons and failures without killing holders
(`agents-plugin-tool/cmd/ws-mcp/proxy.go:354-420`,
`agents-plugin-tool/internal/wsdashboard/ensure.go:89-160`). Ensure app exit
does not stop the daemon, including a verified solution or explicit failure
for Windows job-object breakaway failure, rather than the current unsafe
fallback. Test first start, retry and app exit in isolation. The Windows
strict-lifetime step is separately gated by owner confirmation above; before
it lands, verify the Windows refusal path without spawning any executable,
and report Windows start/survival acceptance as pending. Do not claim the
Windows slice or this ticket complete on portable/mock evidence.

### Phase 3: Controlled replacement and reconnect

Apply newer-core precedence and the existing explicit equal-core rule; verify
which daemon actually answers after handoff. Test older, equal and newer cores,
concurrent plugin starts, failed drain and reconnect. Prove every holder retains
its PID and each compatible session reconnects; audit that no shell path kills,
signals or restarts holders. Surface incompatibility rather than claiming
reconnect overrides the existing hello gate.

### Phase 4: Packaged acceptance

On packaged Windows and macOS builds, verify clean-machine standalone start,
bundled and selected-dev provenance, replacement and newer-core attach,
resource/skills availability, holder continuity, and daemon survival after app
exit. Verify explicit shutdown separately. Update/recovery integration here
means bundled-vs-dev selection authorization, newer-core coexistence and
isolation from accepted-build state; do not implement or require either
separate ticket's downloader, promotion or rollback operation. Windows real
start/survival acceptance awaits the separately approved strict-lifetime step;
missing Windows or macOS evidence leaves Phase 4 and ticket closure pending.
Document exact results and limitations before closing the ticket.
