---
title: "Electron standalone daemon start and replacement without stopping session holders"
related:
  261115-feat-werkschaft-electron-shell: "attach-only first slice and planned missing-daemon start"
  261115-feat-ws-daemon-release-auto-update: "separate user-approved release download and swap; reconcile ownership"
  261115-feat-ws-dev-daemon-accepted-build-recovery: "development accepted-build recovery and rollback follow-up"
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
  (`agents-plugin-tool/internal/wsdashboard/ensure.go:45-53,86-106,163-173`).
  This monotonic core precedence prevents cross-core replace loops and avoids
  replacing a newer-minor daemon with an older one. It does **not** remove
  existing cross-minor hello refusals
  (`agents-plugin-tool/internal/wsdashboard/proxy.go:232-254`).
- **(1198)=a:** The daemon always outlives Electron; app exit and daemon
  shutdown are separate. The daemon stops only through explicit shutdown,
  including the authenticated drain for a designated replacement. On Windows,
  the current job-object breakaway fallback can leave it inside the app's job
  and kill it with the host; a phase must handle that risk, not silently claim
  the lifetime guarantee (`agents-plugin-tool/internal/wsdashboard/spawn_windows.go:12-32`).
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

## Open Decision Queue

- Does the separate user-approved ws-mcp release-download swap in
  `261115-feat-ws-daemon-release-auto-update` remain alongside shell-bundled
  updates, and which source wins when both are available?
- How should the development selector interact with accepted-build recovery and
  rollback in `261115-feat-ws-dev-daemon-accepted-build-recovery`? That ticket
  does not yet define recovery ownership or authorization.
- For sessions actually affected by existing minor/protocol refusal, should the
  shell warn and list them before a swap, and what action is offered? Newer-core
  precedence alone does not make an older session compatible.

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
questions before implementing affected paths.

### Phase 2: Standalone missing-daemon start

Start the selected binary when no daemon is available, using the existing
`daemon-start`/`Ensure` handoff and endpoint/health verification; handle races,
hung daemons and failures without killing holders
(`agents-plugin-tool/cmd/ws-mcp/proxy.go:360-420`,
`agents-plugin-tool/internal/wsdashboard/ensure.go:86-106`). Ensure app exit
does not stop the daemon, including a verified solution or explicit failure
for Windows job-object breakaway failure, rather than the current unsafe
fallback. Test first start, retry and app exit in isolation.

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
exit. Verify explicit shutdown separately and cover the agreed update and
recovery integration; document results and limitations before closing the ticket.
